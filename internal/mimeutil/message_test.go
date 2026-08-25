package mimeutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildAndParseMultipart(t *testing.T) {
	dir := t.TempDir()
	attachment := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(attachment, []byte("hello from disk"), 0o600); err != nil {
		t.Fatal(err)
	}

	raw, err := Build(Draft{
		From:    "Sender <sender@example.com>",
		To:      "receiver@example.com",
		Bcc:     "hidden@example.com",
		Subject: "Test message",
		Body:    "Hello world\nSecond line",
		Attachments: []string{
			attachment,
		},
		MemoryAttachments: []Attachment{{
			Filename: "forwarded.bin",
			MIMEType: "application/octet-stream",
			Data:     []byte{1, 2, 3, 4},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Bcc: hidden@example.com") {
		t.Fatal("Bcc header missing; msmtp -t would not see that recipient")
	}

	path := filepath.Join(dir, "message.eml")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Subject != "Test message" {
		t.Fatalf("subject = %q", parsed.Subject)
	}
	if !strings.Contains(parsed.Body, "Second line") {
		t.Fatalf("body = %q", parsed.Body)
	}
	if len(parsed.Attachments) != 2 {
		t.Fatalf("attachments = %d, want 2", len(parsed.Attachments))
	}
}
