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

func TestBuildMultipleRecipientLists(t *testing.T) {
	raw, err := Build(Draft{
		From:    "Sender <sender@example.com>",
		To:      `Alice <alice@example.com>, "Smith, Bob" <bob@example.com>`,
		Cc:      "carol@example.com, Dave <dave@example.com>",
		Bcc:     "hidden1@example.com, hidden2@example.com",
		Subject: "Multiple recipients",
		Body:    "Hello everyone",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"To: Alice <alice@example.com>, \"Smith, Bob\" <bob@example.com>",
		"Cc: carol@example.com, Dave <dave@example.com>",
		"Bcc: hidden1@example.com, hidden2@example.com",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("built message missing %q:\n%s", want, text)
		}
	}
}

func TestBuildRejectsInvalidRecipientList(t *testing.T) {
	_, err := Build(Draft{
		From: "Sender <sender@example.com>",
		To:   "alice@example.com, definitely-not-an-address",
		Body: "hello",
	})
	if err == nil {
		t.Fatal("invalid To address list was accepted")
	}
	if !strings.Contains(err.Error(), "To:") {
		t.Fatalf("error does not identify To field: %v", err)
	}
}

func TestParseHTMLOnlyAsPlainText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "html-only.eml")
	raw := "From: sender@example.com\r\n" +
		"To: receiver@example.com\r\n" +
		"Subject: HTML only\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		`<html><head><style>.hidden { color: red; }</style></head><body>` +
		`<h1>Hello &amp; welcome</h1><p>This is <b>HTML</b> mail.</p>` +
		`<ul><li>First item</li><li>Second item</li></ul>` +
		`<p><a href="https://example.com/path">Visit the site</a></p>` +
		`<script>alert("do not show this")</script></body></html>`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Hello & welcome",
		"This is HTML mail.",
		"* First item",
		"* Second item",
		"Visit the site (https://example.com/path)",
	} {
		if !strings.Contains(parsed.Body, want) {
			t.Fatalf("plain-text HTML rendering missing %q:\n%s", want, parsed.Body)
		}
	}
	for _, unwanted := range []string{"<html", "<p>", "color: red", "do not show this"} {
		if strings.Contains(parsed.Body, unwanted) {
			t.Fatalf("HTML rendering leaked %q:\n%s", unwanted, parsed.Body)
		}
	}
}
