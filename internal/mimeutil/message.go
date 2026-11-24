// SPDX-License-Identifier: GPL-3.0-only

package mimeutil

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SeraphinaDX/MailSalonGUI/internal/pim"
	"golang.org/x/net/html/charset"
)

type Attachment struct {
	Filename       string
	MIMEType       string
	Data           []byte
	CalendarMethod string // METHOD parameter from the incoming MIME part.
}

type ParsedMessage struct {
	From           string
	ReplyTo        string
	To             string
	Cc             string
	Subject        string
	Date           string
	MessageID      string
	References     string
	Body           string
	HTMLPreview    []BodySpan
	CalendarEvents []pim.CalendarEvent
	CalendarErrors []string
	Attachments    []Attachment
}

type Draft struct {
	CalendarReplyAddress string // RSVP identity; preserved in saved drafts.
	From                 string
	To                   string
	Cc                   string
	Bcc                  string
	Subject              string
	Body                 string
	InReplyTo            string
	References           string
	Attachments          []string
	MemoryAttachments    []Attachment
}

var wordDecoder = &mime.WordDecoder{CharsetReader: charset.NewReaderLabel}

const MaxMessageSize = 64 << 20
const maxPartSize = 32 << 20
const maxParts = 256
const maxDepth = 32

// ReadMessageFile limits input before allocating memory for an untrusted message.
func ReadMessageFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxMessageSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxMessageSize {
		return nil, fmt.Errorf("message exceeds %d bytes", MaxMessageSize)
	}
	return data, nil
}

func ParseFile(path string) (*ParsedMessage, error) {
	raw, err := ReadMessageFile(path)
	if err != nil {
		return nil, err
	}
	return ParseBytes(raw)
}

// ParseBytes parses a complete RFC 5322 message already held in memory. This is
// used after OpenPGP processing has decrypted/verified a PGP/MIME wrapper.
func ParseBytes(raw []byte) (*ParsedMessage, error) {
	if len(raw) > MaxMessageSize {
		return nil, fmt.Errorf("message exceeds %d bytes", MaxMessageSize)
	}
	return parseMessage(bytes.NewReader(raw))
}

func parseMessage(r io.Reader) (*ParsedMessage, error) {
	m, err := mail.ReadMessage(r)
	if err != nil {
		return nil, err
	}
	p := &ParsedMessage{
		From:       decodeHeader(m.Header.Get("From")),
		ReplyTo:    decodeHeader(m.Header.Get("Reply-To")),
		To:         decodeHeader(m.Header.Get("To")),
		Cc:         decodeHeader(m.Header.Get("Cc")),
		Subject:    decodeHeader(m.Header.Get("Subject")),
		Date:       m.Header.Get("Date"),
		MessageID:  strings.TrimSpace(m.Header.Get("Message-ID")),
		References: strings.TrimSpace(m.Header.Get("References")),
	}
	body, err := parseEntity(textproto.MIMEHeader(m.Header), m.Body, p, 0, &parseBudget{})
	if err != nil {
		return nil, err
	}
	p.Body = strings.TrimSpace(body.text)
	if body.html {
		p.HTMLPreview = body.spans
	}
	return p, nil
}

func NormalizeAddressList(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if _, err := mail.ParseAddressList(raw); err != nil {
		return "", fmt.Errorf("invalid address list: %w", err)
	}
	return raw, nil
}

func Build(d Draft) ([]byte, error) {
	if d.CalendarReplyAddress != "" {
		from, err := mail.ParseAddress(d.From)
		if err != nil || !strings.EqualFold(from.Address, d.CalendarReplyAddress) {
			return nil, fmt.Errorf("calendar reply must be sent from %s", d.CalendarReplyAddress)
		}
		found := false
		for _, att := range d.MemoryAttachments {
			ctype, params, _ := mime.ParseMediaType(att.MIMEType)
			if ctype == "text/calendar" && strings.EqualFold(params["method"], "REPLY") && len(att.Data) > 0 {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("calendar response attachment is missing; reopen the invitation response")
		}
	}
	if strings.TrimSpace(d.From) == "" {
		return nil, fmt.Errorf("from address is empty")
	}
	if strings.TrimSpace(d.To) == "" && strings.TrimSpace(d.Cc) == "" && strings.TrimSpace(d.Bcc) == "" {
		return nil, fmt.Errorf("no recipients")
	}
	for _, v := range []string{d.From, d.To, d.Cc, d.Bcc, d.Subject, d.InReplyTo, d.References} {
		if strings.ContainsAny(v, "\r\n") {
			return nil, fmt.Errorf("header contains a newline")
		}
	}

	to, err := NormalizeAddressList(d.To)
	if err != nil {
		return nil, fmt.Errorf("To: %w", err)
	}
	cc, err := NormalizeAddressList(d.Cc)
	if err != nil {
		return nil, fmt.Errorf("Cc: %w", err)
	}
	bcc, err := NormalizeAddressList(d.Bcc)
	if err != nil {
		return nil, fmt.Errorf("Bcc: %w", err)
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	writeHeader(w, "From", d.From)
	writeHeader(w, "To", to)
	if strings.TrimSpace(cc) != "" {
		writeHeader(w, "Cc", cc)
	}
	// Senders such as `msmtp -t` need Bcc in the input to discover envelope
	// recipients. Such senders are expected to strip it before transmission.
	if strings.TrimSpace(bcc) != "" {
		writeHeader(w, "Bcc", bcc)
	}
	writeHeader(w, "Subject", encodeHeader(d.Subject))
	writeHeader(w, "Date", time.Now().Format(time.RFC1123Z))
	writeHeader(w, "Message-ID", newMessageID())
	if d.InReplyTo != "" {
		writeHeader(w, "In-Reply-To", d.InReplyTo)
	}
	if d.References != "" {
		writeHeader(w, "References", d.References)
	}
	writeHeader(w, "MIME-Version", "1.0")

	if len(d.Attachments) == 0 && len(d.MemoryAttachments) == 0 {
		writeHeader(w, "Content-Type", `text/plain; charset="utf-8"`)
		writeHeader(w, "Content-Transfer-Encoding", "quoted-printable")
		_, _ = w.WriteString("\r\n")
		if err := w.Flush(); err != nil {
			return nil, err
		}
		qw := quotedprintable.NewWriter(&buf)
		_, err := io.WriteString(qw, normalizeCRLF(d.Body))
		if closeErr := qw.Close(); err == nil {
			err = closeErr
		}
		return buf.Bytes(), err
	}

	mw := multipart.NewWriter(&buf)
	boundary := mw.Boundary()
	writeHeader(w, "Content-Type", fmt.Sprintf(`multipart/mixed; boundary="%s"`, boundary))
	_, _ = w.WriteString("\r\n")
	if err := w.Flush(); err != nil {
		return nil, err
	}

	bodyHeader := make(textproto.MIMEHeader)
	bodyHeader.Set("Content-Type", `text/plain; charset="utf-8"`)
	bodyHeader.Set("Content-Transfer-Encoding", "quoted-printable")
	part, err := mw.CreatePart(bodyHeader)
	if err != nil {
		return nil, err
	}
	qw := quotedprintable.NewWriter(part)
	if _, err := io.WriteString(qw, normalizeCRLF(d.Body)); err != nil {
		return nil, err
	}
	if err := qw.Close(); err != nil {
		return nil, err
	}

	for _, path := range d.Attachments {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("attachment %s: %w", path, err)
		}
		filename := filepath.Base(path)
		ctype := mime.TypeByExtension(filepath.Ext(filename))
		if ctype == "" {
			ctype = "application/octet-stream"
		}
		if err := writeAttachment(mw, filename, ctype, data); err != nil {
			return nil, err
		}
	}
	for _, a := range d.MemoryAttachments {
		ctype := a.MIMEType
		if ctype == "" {
			ctype = mime.TypeByExtension(filepath.Ext(a.Filename))
		}
		if ctype == "" {
			ctype = "application/octet-stream"
		}
		if a.CalendarMethod != "" {
			mediaType, params, err := mime.ParseMediaType(ctype)
			if err == nil && mediaType == "text/calendar" {
				params["method"] = a.CalendarMethod
				ctype = mime.FormatMediaType(mediaType, params)
			}
		}
		if err := writeAttachment(mw, safeFilename(a.Filename), ctype, a.Data); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeAttachment(mw *multipart.Writer, filename, ctype string, data []byte) error {
	if filename == "" {
		filename = "attachment"
	}
	h := make(textproto.MIMEHeader)
	mediaType, params, err := mime.ParseMediaType(ctype)
	if err != nil {
		return fmt.Errorf("invalid attachment content type: %w", err)
	}
	params["name"] = filename
	h.Set("Content-Type", mime.FormatMediaType(mediaType, params))
	h.Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
	h.Set("Content-Transfer-Encoding", "base64")
	part, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	lw := &base64LineWriter{w: part}
	enc := base64.NewEncoder(base64.StdEncoding, lw)
	if _, err := enc.Write(data); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return lw.Close()
}

func SaveAttachments(p *ParsedMessage, dir string) ([]string, error) {
	if p == nil || len(p.Attachments) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var saved []string
	for _, a := range p.Attachments {
		name := safeFilename(a.Filename)
		if name == "" {
			name = "attachment"
		}
		path := uniquePath(filepath.Join(dir, name))
		if err := os.WriteFile(path, a.Data, 0o600); err != nil {
			return saved, err
		}
		saved = append(saved, path)
	}
	return saved, nil
}

// Alternatives describe the same content; mixed/related body sections are
// combined in source order. Keep those rules separate to avoid dropped text
// sections or duplicate plain/HTML copies in a formatted preview.
type mimeBody struct {
	text        string
	spans       []BodySpan
	html, plain bool
}

type parseBudget struct {
	parts   int
	decoded int64
}

func parseEntity(h textproto.MIMEHeader, body io.Reader, p *ParsedMessage, depth int, budget *parseBudget) (mimeBody, error) {
	if depth > maxDepth {
		return mimeBody{}, fmt.Errorf("MIME nesting exceeds %d levels", maxDepth)
	}
	budget.parts++
	if budget.parts > maxParts {
		return mimeBody{}, fmt.Errorf("MIME message exceeds %d parts", maxParts)
	}
	ctype, params, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil || ctype == "" {
		ctype = "text/plain"
	}
	if strings.HasPrefix(ctype, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return mimeBody{}, fmt.Errorf("multipart message has no boundary")
		}
		mr := multipart.NewReader(body, boundary)
		var parts []mimeBody
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return mimeBody{}, err
			}
			content, err := parseEntity(part.Header, part, p, depth+1, budget)
			if err != nil {
				return mimeBody{}, err
			}
			if strings.TrimSpace(content.text) != "" {
				parts = append(parts, content)
			}
		}
		if len(parts) == 0 {
			return mimeBody{}, nil
		}
		if ctype == "multipart/alternative" {
			display := parts[len(parts)-1]
			for _, part := range parts {
				if part.html {
					display = part
				}
			}
			quote := display
			for _, part := range parts {
				if part.plain {
					quote = part
					break
				}
			}
			return mimeBody{text: quote.text, spans: display.spans, html: display.html, plain: quote.plain}, nil
		}
		var combined mimeBody
		var texts []string
		for _, part := range parts {
			texts = append(texts, part.text)
			if len(combined.spans) > 0 {
				combined.spans = append(combined.spans, BodySpan{Text: "\n\n"})
			}
			combined.spans = append(combined.spans, part.spans...)
			combined.html = combined.html || part.html
			combined.plain = combined.plain || part.plain
		}
		combined.text = strings.Join(texts, "\n\n")
		return combined, nil
	}

	decoded, err := io.ReadAll(io.LimitReader(decodeTransfer(h.Get("Content-Transfer-Encoding"), body), maxPartSize+1))
	if err != nil {
		return mimeBody{}, err
	}
	if len(decoded) > maxPartSize {
		return mimeBody{}, fmt.Errorf("MIME part exceeds %d bytes", maxPartSize)
	}
	budget.decoded += int64(len(decoded))
	if budget.decoded > MaxMessageSize {
		return mimeBody{}, fmt.Errorf("decoded MIME content exceeds %d bytes", MaxMessageSize)
	}
	disp, dparams, _ := mime.ParseMediaType(h.Get("Content-Disposition"))
	filename := dparams["filename"]
	if filename == "" {
		filename = params["name"]
	}
	filename = decodeHeader(filename)
	calendar := strings.EqualFold(ctype, "text/calendar") || strings.EqualFold(ctype, "application/ics") || strings.EqualFold(ctype, "application/icalendar") || strings.EqualFold(filepath.Ext(filename), ".ics")
	if calendar || strings.EqualFold(disp, "attachment") || filename != "" {
		if calendar && filename == "" {
			filename = "invitation.ics"
		}
		if calendar && params["charset"] != "" {
			if r, err := charset.NewReaderLabel(params["charset"], bytes.NewReader(decoded)); err == nil {
				if converted, err := io.ReadAll(io.LimitReader(r, maxPartSize+1)); err == nil && len(converted) <= maxPartSize {
					decoded = converted
				}
			}
		}
		if calendar {
			events, err := pim.ParseCalendar(decoded, params["method"])
			if err != nil {
				p.CalendarErrors = append(p.CalendarErrors, filename+": "+err.Error())
			} else {
				p.CalendarEvents = append(p.CalendarEvents, events...)
			}
		}
		p.Attachments = append(p.Attachments, Attachment{Filename: filename, MIMEType: ctype, Data: decoded, CalendarMethod: params["method"]})
		return mimeBody{}, nil
	}

	switch strings.ToLower(ctype) {
	case "text/plain", "text/html":
		textReader := io.Reader(bytes.NewReader(decoded))
		if label := params["charset"]; label != "" {
			textReader, err = charset.NewReaderLabel(label, textReader)
		} else if strings.EqualFold(ctype, "text/html") {
			// Also recognize HTML meta charset declarations when MIME omitted one.
			textReader, err = charset.NewReader(textReader, ctype)
		}
		if err != nil {
			// An unrecognized label must not hide an otherwise readable message.
			textReader = bytes.NewReader(decoded)
		}
		text, err := io.ReadAll(io.LimitReader(textReader, maxPartSize+1))
		if err != nil {
			return mimeBody{}, err
		}
		if len(text) > maxPartSize {
			return mimeBody{}, fmt.Errorf("decoded text exceeds %d bytes", maxPartSize)
		}
		readable := strings.ToValidUTF8(string(text), "\uFFFD")
		if strings.EqualFold(ctype, "text/html") {
			spans := parseHTML(readable)
			display := (&ParsedMessage{HTMLPreview: spans}).DisplayText()
			return mimeBody{text: display, spans: spans, html: len(spans) > 0}, nil
		}
		return mimeBody{text: readable, spans: []BodySpan{{Text: readable}}, plain: true}, nil
	}
	return mimeBody{}, nil
}

func decodeTransfer(enc string, r io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(enc)) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, r)
	case "quoted-printable":
		return quotedprintable.NewReader(r)
	default:
		return r
	}
}

func decodeHeader(s string) string {
	if v, err := wordDecoder.DecodeHeader(s); err == nil {
		return v
	}
	return s
}

func encodeHeader(s string) string {
	for _, r := range s {
		if r > 127 {
			return mime.QEncoding.Encode("utf-8", s)
		}
	}
	return s
}

func writeHeader(w *bufio.Writer, key, value string) {
	if strings.TrimSpace(value) != "" {
		_, _ = fmt.Fprintf(w, "%s: %s\r\n", key, value)
	}
}

func newMessageID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	host, _ := os.Hostname()
	if host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("<%x.%d@%s>", b, time.Now().UnixNano(), host)
}

func normalizeCRLF(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}

func safeFilename(s string) string {
	s = filepath.Base(strings.TrimSpace(s))
	if s == "." || s == string(filepath.Separator) {
		return ""
	}
	return strings.ReplaceAll(s, "\x00", "")
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

type base64LineWriter struct {
	w   io.Writer
	col int
}

func (l *base64LineWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		space := 76 - l.col
		if space == 0 {
			if _, err := io.WriteString(l.w, "\r\n"); err != nil {
				return written, err
			}
			l.col = 0
			space = 76
		}
		n := len(p)
		if n > space {
			n = space
		}
		m, err := l.w.Write(p[:n])
		written += m
		l.col += m
		p = p[m:]
		if err != nil {
			return written, err
		}
		if m != n {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

func (l *base64LineWriter) Close() error {
	if l.col != 0 {
		_, err := io.WriteString(l.w, "\r\n")
		l.col = 0
		return err
	}
	return nil
}
