package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"strings"
)

var tagRE = regexp.MustCompile(`(?s)<[^>]*>`)
var breakRE = regexp.MustCompile(`(?i)</?(br|p|div|li|tr|h[1-6])\b[^>]*>`)

type Attachment struct {
	Filename  string
	MediaType string
	Size      int64
	Data      []byte
}

func decodeHeader(s string) string {
	if s == "" {
		return ""
	}
	decoded, err := new(mime.WordDecoder).DecodeHeader(s)
	if err != nil {
		return s
	}
	return decoded
}

func decodeMessageBody(header mail.Header, body io.Reader) (string, error) {
	text, _, err := decodeMessageContent(header, body, false)
	return text, err
}

func decodeMessageContent(header mail.Header, body io.Reader, keepAttachmentData bool) (string, []Attachment, error) {
	plain, htmlBody, attachments, err := decodeEntity(header, body, keepAttachmentData)
	if err != nil {
		return "", attachments, err
	}
	if strings.TrimSpace(plain) != "" {
		return normalizeNewlines(plain), attachments, nil
	}
	if strings.TrimSpace(htmlBody) != "" {
		return htmlToText(htmlBody), attachments, nil
	}
	return "[No displayable text body]", attachments, nil
}

func decodeEntity(header mail.Header, body io.Reader, keepAttachmentData bool) (plain string, htmlBody string, attachments []Attachment, err error) {
	mediaType, params, parseErr := mime.ParseMediaType(header.Get("Content-Type"))
	if parseErr != nil || mediaType == "" {
		mediaType = "text/plain"
		params = map[string]string{}
	}

	disposition, dispParams, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
	filename := decodeHeader(strings.TrimSpace(dispParams["filename"]))
	if filename == "" {
		filename = decodeHeader(strings.TrimSpace(params["name"]))
	}
	isAttachment := strings.EqualFold(disposition, "attachment") || filename != ""

	if isAttachment {
		decodedReader := transferDecodedReader(header.Get("Content-Transfer-Encoding"), body)
		att := Attachment{Filename: filename, MediaType: mediaType}
		if keepAttachmentData {
			data, readErr := io.ReadAll(decodedReader)
			if readErr != nil {
				return "", "", nil, readErr
			}
			att.Data = data
			att.Size = int64(len(data))
		} else {
			n, readErr := io.Copy(io.Discard, decodedReader)
			if readErr != nil {
				return "", "", nil, readErr
			}
			att.Size = n
		}
		if att.Filename == "" {
			att.Filename = "attachment"
		}
		return "", "", []Attachment{att}, nil
	}

	if strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return "", "", nil, fmt.Errorf("multipart message has no boundary")
		}
		mr := multipart.NewReader(body, boundary)
		var plainParts, htmlParts []string
		for {
			part, e := mr.NextPart()
			if e == io.EOF {
				break
			}
			if e != nil {
				return "", "", attachments, e
			}
			pHeader := mail.Header(part.Header)
			p, h, a, e := decodeEntity(pHeader, part, keepAttachmentData)
			part.Close()
			if e != nil {
				continue
			}
			if strings.TrimSpace(p) != "" {
				plainParts = append(plainParts, p)
			}
			if strings.TrimSpace(h) != "" {
				htmlParts = append(htmlParts, h)
			}
			attachments = append(attachments, a...)
		}
		return strings.Join(plainParts, "\n\n"), strings.Join(htmlParts, "\n\n"), attachments, nil
	}

	decodedReader := transferDecodedReader(header.Get("Content-Transfer-Encoding"), body)
	data, err := io.ReadAll(decodedReader)
	if err != nil {
		return "", "", nil, err
	}

	switch strings.ToLower(mediaType) {
	case "text/plain":
		return string(data), "", nil, nil
	case "text/html":
		return "", string(data), nil, nil
	case "message/rfc822":
		inner, err := mail.ReadMessage(bytes.NewReader(data))
		if err != nil {
			return "", "", nil, err
		}
		return decodeEntity(inner.Header, inner.Body, keepAttachmentData)
	default:
		return "", "", nil, nil
	}
}

func transferDecodedReader(encoding string, r io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, r)
	case "quoted-printable":
		return quotedprintable.NewReader(r)
	default:
		return r
	}
}

func htmlToText(s string) string {
	s = breakRE.ReplaceAllString(s, "\n")
	s = tagRE.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return normalizeNewlines(s)
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	var out []string
	blank := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if !blank {
				out = append(out, "")
				blank = true
			}
			continue
		}
		blank = false
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
