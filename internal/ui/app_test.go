package uiapp

import (
	"strings"
	"testing"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/mimeutil"
)

func TestApplySignatureToReply(t *testing.T) {
	body := "\n\nOn Tue, Example wrote:\n> hello\n"
	got := applySignature(body, "Britney\nhttps://example.com")
	wantPrefix := "-- \nBritney\nhttps://example.com\n\nOn Tue, Example wrote:"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("signature was not inserted before quote:\n%q", got)
	}
}

func TestApplySignaturePreservesSeparator(t *testing.T) {
	got := applySignature("Hello", "-- \nExisting signature")
	if strings.Count(got, "-- ") != 1 {
		t.Fatalf("signature separator duplicated: %q", got)
	}
}

func TestPreferredReplyAccount(t *testing.T) {
	a := &App{
		cfg: config.Config{Accounts: []config.Account{
			{Name: "personal", From: "Person <person@example.com>"},
			{Name: "work", From: "Person <person@work.example>"},
		}},
		account: 0,
	}
	p := &mimeutil.ParsedMessage{To: "Person <person@work.example>"}
	if got := a.preferredReplyAccount(p); got != 1 {
		t.Fatalf("preferredReplyAccount = %d, want 1", got)
	}
}

func TestParseThemeColor(t *testing.T) {
	if _, err := parseColor("#ff6f91"); err != nil {
		t.Fatalf("hex color rejected: %v", err)
	}
	if _, err := parseColor("hotpink"); err != nil {
		t.Fatalf("named color rejected: %v", err)
	}
	if _, err := parseColor("not-a-color"); err == nil {
		t.Fatal("invalid color was accepted")
	}
}

func TestMainLegendShowsComposeAndUpdate(t *testing.T) {
	a := &App{
		cfg:    config.Config{Accounts: []config.Account{{Name: "test"}}},
		status: "Ready",
	}
	legend := a.footerText()
	for _, want := range []string{"c Compose", "u Update mail"} {
		if !strings.Contains(legend, want) {
			t.Fatalf("main legend missing %q: %q", want, legend)
		}
	}
}
