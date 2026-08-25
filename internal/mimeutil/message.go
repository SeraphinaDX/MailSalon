package mimeutil

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Attachment struct {
	Filename string
	MIMEType string
	Data     []byte
}

type ParsedMessage struct {
	From        string
	To          string
	Cc          string
	Subject     string
	Date        string
	MessageID   string
	References  string
	Body        string
	Attachments []Attachment
}

type Draft struct {
	From              string
	To                string
	Cc                string
	Bcc               string
	Subject           string
	Body              string
	InReplyTo         string
	References        string
	Attachments       []string
	MemoryAttachments []Attachment
}

var (
	wordDecoder = &mime.WordDecoder{}
	tagRE       = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRE     = regexp.MustCompile(`[ \t]+`)
	blankRE     = regexp.MustCompile(`\n{3,}`)
)

func ParseFile(path string) (*ParsedMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m, err := mail.ReadMessage(f)
	if err != nil {
		return nil, err
	}
	p := &ParsedMessage{
		From:       decodeHeader(m.Header.Get("From")),
		To:         decodeHeader(m.Header.Get("To")),
		Cc:         decodeHeader(m.Header.Get("Cc")),
		Subject:    decodeHeader(m.Header.Get("Subject")),
		Date:       m.Header.Get("Date"),
		MessageID:  strings.TrimSpace(m.Header.Get("Message-ID")),
		References: strings.TrimSpace(m.Header.Get("References")),
	}
	var plainParts, htmlParts []string
	if err := parseEntity(textproto.MIMEHeader(m.Header), m.Body, p, &plainParts, &htmlParts); err != nil {
		return nil, err
	}
	if len(plainParts) > 0 {
		p.Body = strings.TrimSpace(strings.Join(plainParts, "\n\n"))
	} else if len(htmlParts) > 0 {
		p.Body = stripHTML(strings.Join(htmlParts, "\n"))
	}
	return p, nil
}

func Build(d Draft) ([]byte, error) {
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

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	writeHeader(w, "From", d.From)
	writeHeader(w, "To", d.To)
	if strings.TrimSpace(d.Cc) != "" {
		writeHeader(w, "Cc", d.Cc)
	}
	// Senders such as `msmtp -t` need Bcc in the input to discover envelope
	// recipients. Such senders are expected to strip it before transmission.
	if strings.TrimSpace(d.Bcc) != "" {
		writeHeader(w, "Bcc", d.Bcc)
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
	h.Set("Content-Type", fmt.Sprintf(`%s; name=%q`, ctype, filename))
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

func parseEntity(h textproto.MIMEHeader, body io.Reader, p *ParsedMessage, plainParts, htmlParts *[]string) error {
	ctype, params, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil || ctype == "" {
		ctype = "text/plain"
	}
	if strings.HasPrefix(ctype, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return fmt.Errorf("multipart message has no boundary")
		}
		mr := multipart.NewReader(body, boundary)
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if err := parseEntity(part.Header, part, p, plainParts, htmlParts); err != nil {
				return err
			}
		}
		return nil
	}

	decoded, err := io.ReadAll(decodeTransfer(h.Get("Content-Transfer-Encoding"), body))
	if err != nil {
		return err
	}
	disp, dparams, _ := mime.ParseMediaType(h.Get("Content-Disposition"))
	filename := dparams["filename"]
	if filename == "" {
		filename = params["name"]
	}
	filename = decodeHeader(filename)
	if strings.EqualFold(disp, "attachment") || filename != "" {
		p.Attachments = append(p.Attachments, Attachment{Filename: filename, MIMEType: ctype, Data: decoded})
		return nil
	}

	switch strings.ToLower(ctype) {
	case "text/plain":
		*plainParts = append(*plainParts, string(decoded))
	case "text/html":
		*htmlParts = append(*htmlParts, string(decoded))
	}
	return nil
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

func stripHTML(s string) string {
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = strings.ReplaceAll(s, "</p>", "\n\n")
	s = tagRE.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = spaceRE.ReplaceAllString(s, " ")
	s = blankRE.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
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
