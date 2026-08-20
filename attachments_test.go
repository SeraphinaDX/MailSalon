package main

import (
	"bytes"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeAndSaveAttachment(t *testing.T) {
	raw := strings.Join([]string{
		"From: Sender <sender@example.com>",
		"To: me@example.com",
		"Subject: attachment test",
		"Date: Wed, 19 Aug 2026 10:00:00 -0400",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="demo"`,
		"",
		"--demo",
		`Content-Type: text/plain; charset="UTF-8"`,
		"",
		"hello",
		"--demo",
		`Content-Type: text/plain; name="notes.txt"`,
		`Content-Disposition: attachment; filename="notes.txt"`,
		"Content-Transfer-Encoding: base64",
		"",
		"YXR0YWNobWVudCBjb250ZW50Cg==",
		"--demo--",
		"",
	}, "\r\n")

	parsed, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	body, attachments, err := decodeMessageContent(parsed.Header, parsed.Body, false)
	if err != nil {
		t.Fatal(err)
	}
	if body != "hello" {
		t.Fatalf("body = %q", body)
	}
	if len(attachments) != 1 || attachments[0].Filename != "notes.txt" || attachments[0].Size == 0 {
		t.Fatalf("attachments = %#v", attachments)
	}
	if attachments[0].Data != nil {
		t.Fatal("metadata-only decode retained attachment data")
	}

	maildir := t.TempDir()
	for _, sub := range []string{"cur", "new", "tmp"} {
		if err := os.Mkdir(filepath.Join(maildir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(maildir, "cur", "message:2,S")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	msg, err := parseMailFile(path, false)
	if err != nil {
		t.Fatal(err)
	}
	downloads := filepath.Join(t.TempDir(), "downloads")
	saved, err := saveMessageAttachments(msg, downloads)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 {
		t.Fatalf("saved = %#v", saved)
	}
	got, err := os.ReadFile(saved[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "attachment content\n" {
		t.Fatalf("saved content = %q", got)
	}
}

func TestBuildReplyWithAttachment(t *testing.T) {
	file := filepath.Join(t.TempDir(), "hello.txt")
	wantData := []byte("hello attachment\n")
	if err := os.WriteFile(file, wantData, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{From: "Me <me@example.com>"}
	original := Message{
		ReplyTo:   "sender@example.com",
		Subject:   "Subject",
		MessageID: "<original@example.com>",
	}
	wire, err := buildReplyWithAttachments(cfg, original, "reply body", []string{file})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mail.ReadMessage(bytes.NewReader(wire))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(parsed.Header.Get("Content-Type"), "multipart/mixed") {
		t.Fatalf("Content-Type = %q", parsed.Header.Get("Content-Type"))
	}
	body, attachments, err := decodeMessageContent(parsed.Header, parsed.Body, true)
	if err != nil {
		t.Fatal(err)
	}
	if body != "reply body" {
		t.Fatalf("body = %q", body)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %#v", attachments)
	}
	if attachments[0].Filename != "hello.txt" || !bytes.Equal(attachments[0].Data, wantData) {
		t.Fatalf("attachment = %#v", attachments[0])
	}
}
