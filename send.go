package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func buildReply(cfg Config, original Message, body string) ([]byte, error) {
	return buildReplyWithAttachments(cfg, original, body, nil)
}

func buildReplyWithAttachments(cfg Config, original Message, body string, attachmentPaths []string) ([]byte, error) {
	if strings.TrimSpace(cfg.From) == "" {
		return nil, fmt.Errorf("no From address configured; use -from or MAILTUI_FROM")
	}
	if original.ReplyTo == "" {
		return nil, fmt.Errorf("message has no usable Reply-To/From address")
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return nil, fmt.Errorf("invalid From address %q: %w", cfg.From, err)
	}
	if _, err := mail.ParseAddress(original.ReplyTo); err != nil {
		return nil, fmt.Errorf("invalid reply address %q: %w", original.ReplyTo, err)
	}

	subject := original.Subject
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(subject)), "re:") {
		subject = "Re: " + subject
	}

	var b bytes.Buffer
	writeHeader(&b, "From", cfg.From)
	writeHeader(&b, "To", original.ReplyTo)
	writeHeader(&b, "Subject", subject)
	writeHeader(&b, "Date", time.Now().Format(time.RFC1123Z))
	writeHeader(&b, "Message-ID", makeMessageID())
	if original.MessageID != "" {
		writeHeader(&b, "In-Reply-To", original.MessageID)
		refs := strings.TrimSpace(original.References + " " + original.MessageID)
		writeHeader(&b, "References", refs)
	}
	writeHeader(&b, "MIME-Version", "1.0")

	body = normalizeNewlines(body)
	if len(attachmentPaths) == 0 {
		writeHeader(&b, "Content-Type", `text/plain; charset="UTF-8"`)
		writeHeader(&b, "Content-Transfer-Encoding", "8bit")
		b.WriteString("\r\n")
		writeTextBody(&b, body)
		return b.Bytes(), nil
	}

	var multipartBody bytes.Buffer
	mw := multipart.NewWriter(&multipartBody)
	writeHeader(&b, "Content-Type", fmt.Sprintf(`multipart/mixed; boundary="%s"`, mw.Boundary()))
	b.WriteString("\r\n")

	textHeader := make(textproto.MIMEHeader)
	textHeader.Set("Content-Type", `text/plain; charset="UTF-8"`)
	textHeader.Set("Content-Transfer-Encoding", "8bit")
	textPart, err := mw.CreatePart(textHeader)
	if err != nil {
		return nil, err
	}
	writeTextBody(textPart, body)

	for _, path := range attachmentPaths {
		if err := addAttachmentPart(mw, path); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	b.Write(multipartBody.Bytes())
	return b.Bytes(), nil
}

func writeTextBody(w interface{ Write([]byte) (int, error) }, body string) {
	text := strings.ReplaceAll(body, "\n", "\r\n")
	_, _ = w.Write([]byte(text))
	if !strings.HasSuffix(text, "\r\n") {
		_, _ = w.Write([]byte("\r\n"))
	}
}

func addAttachmentPart(mw *multipart.Writer, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read attachment %q: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat attachment %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("attachment %q is not a regular file", path)
	}

	filename := filepath.Base(path)
	mediaType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if mediaType == "" {
		mediaType = http.DetectContentType(data)
	}
	contentType := mime.FormatMediaType(mediaType, map[string]string{"name": filename})
	contentDisposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})

	header := make(textproto.MIMEHeader)
	header.Set("Content-Type", contentType)
	header.Set("Content-Disposition", contentDisposition)
	header.Set("Content-Transfer-Encoding", "base64")
	part, err := mw.CreatePart(header)
	if err != nil {
		return err
	}
	return writeBase64MIME(part, data)
}

func writeBase64MIME(w interface{ Write([]byte) (int, error) }, data []byte) error {
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
	base64.StdEncoding.Encode(encoded, data)
	for len(encoded) > 0 {
		n := 76
		if len(encoded) < n {
			n = len(encoded)
		}
		if _, err := w.Write(encoded[:n]); err != nil {
			return err
		}
		if _, err := w.Write([]byte("\r\n")); err != nil {
			return err
		}
		encoded = encoded[n:]
	}
	return nil
}

func writeHeader(b *bytes.Buffer, name, value string) {
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", " ")
	fmt.Fprintf(b, "%s: %s\r\n", name, value)
}

func makeMessageID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("<%d.%d@%s>", time.Now().UnixNano(), os.Getpid(), host)
}

func runMSMTP(cfg Config, message []byte) error {
	cmd := exec.Command(cfg.MSMTP, cfg.MSMTPArgs...)
	cmd.Stdin = bytes.NewReader(message)
	output, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			return fmt.Errorf("%s failed: %w", cfg.MSMTP, err)
		}
		return fmt.Errorf("%s failed: %w: %s", cfg.MSMTP, err, text)
	}
	return nil
}
