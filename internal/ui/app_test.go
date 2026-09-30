package uiapp

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/metaspartan/gotui/v5/widgets"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/maildir"
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
	for _, want := range []string{"c Compose", "u Sync", "m Read/unread", "/ Search"} {
		if !strings.Contains(legend, want) {
			t.Fatalf("main legend missing %q: %q", want, legend)
		}
	}
}

func TestPreviewLegendShowsReadToggleAndSearch(t *testing.T) {
	a := &App{cfg: config.Config{Accounts: []config.Account{{Name: "test"}}}}
	legend := a.messagePreviewLegend()
	for _, want := range []string{"m Read/Unread", "/ Search"} {
		if !strings.Contains(legend, want) {
			t.Fatalf("message preview legend missing %q: %q", want, legend)
		}
	}
}

func TestLegendsReflectConfiguredKeybindings(t *testing.T) {
	a := &App{
		cfg: config.Config{
			Accounts: []config.Account{{Name: "test", ArchiveFolder: "Archive"}},
			Keybindings: config.Keybindings{
				Compose:         "n",
				Sync:            "s",
				Reply:           "p",
				Forward:         "F",
				Archive:         "v",
				ToggleRead:      "t",
				Search:          "?",
				Delete:          "x",
				SaveAttachments: "z",
			},
		},
		folders: []maildir.Folder{{Name: "Archive", Path: "/tmp/archive"}},
		status:  "Ready",
	}
	mainLegend := a.footerText()
	for _, want := range []string{"n Compose", "s Sync", "p Reply", "F Forward", "v Archive", "t Read/unread", "? Search", "x Delete", "z Save attachments"} {
		if !strings.Contains(mainLegend, want) {
			t.Fatalf("custom main legend missing %q: %q", want, mainLegend)
		}
	}
	previewLegend := a.messagePreviewLegend()
	for _, want := range []string{"p Reply", "F Fwd", "v Archive", "t Read/Unread", "? Search", "z Save", "x Delete"} {
		if !strings.Contains(previewLegend, want) {
			t.Fatalf("custom preview legend missing %q: %q", want, previewLegend)
		}
	}
}

func TestBindingEventID(t *testing.T) {
	tests := map[string]string{
		"c":         "c",
		"A":         "A",
		"Ctrl+S":    "<C-s>",
		"Esc":       "<Escape>",
		"Tab":       "<Tab>",
		"Shift+Tab": "<Backtab>",
		"PgUp":      "<PageUp>",
	}
	for input, want := range tests {
		if got := bindingEventID(input); got != want {
			t.Fatalf("bindingEventID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestComposeInitialFocus(t *testing.T) {
	a := &App{
		cfg: config.Config{Accounts: []config.Account{{Name: "test", From: "test@example.com"}}},
	}
	a.startCompose(nil, false)
	if a.compose == nil || a.compose.field != composeTo {
		t.Fatalf("new compose field = %v, want To", a.compose.field)
	}

	source := &mimeutil.ParsedMessage{
		From:    "Sender <sender@example.com>",
		Subject: "Hello",
		Body:    "Original body",
	}
	a.startCompose(source, false)
	if a.compose == nil || a.compose.field != composeBody {
		t.Fatalf("reply compose field = %v, want Body", a.compose.field)
	}

	a.startCompose(source, true)
	if a.compose == nil || a.compose.field != composeTo {
		t.Fatalf("forward compose field = %v, want To", a.compose.field)
	}
}

func TestComposeShowsCursorOnlyOnActiveField(t *testing.T) {
	theme, err := resolveTheme(config.DefaultTheme())
	if err != nil {
		t.Fatal(err)
	}
	a := &App{
		cfg:   config.Config{Accounts: []config.Account{{Name: "test", From: "test@example.com"}}},
		theme: theme,
	}
	a.startCompose(nil, false)

	// New compose starts on To. Only that single-line input should retain the
	// configured cursor style; the body cursor must be hidden.
	a.highlightComposeField()
	if reflect.DeepEqual(a.compose.to.CursorStyle, a.compose.to.TextStyle) {
		t.Fatal("active To field cursor is hidden")
	}
	for name, input := range map[string]*widgets.Input{
		"Cc":      a.compose.cc,
		"Bcc":     a.compose.bcc,
		"Subject": a.compose.subject,
	} {
		if !reflect.DeepEqual(input.CursorStyle, input.TextStyle) {
			t.Fatalf("inactive %s field still shows a cursor", name)
		}
	}
	if a.compose.body.ShowCursor {
		t.Fatal("inactive body still shows a cursor")
	}

	// Moving focus to the body should hide every header cursor and show only
	// the TextArea cursor.
	a.compose.field = composeBody
	a.highlightComposeField()
	for name, input := range map[string]*widgets.Input{
		"To":      a.compose.to,
		"Cc":      a.compose.cc,
		"Bcc":     a.compose.bcc,
		"Subject": a.compose.subject,
	} {
		if !reflect.DeepEqual(input.CursorStyle, input.TextStyle) {
			t.Fatalf("inactive %s field still shows a cursor", name)
		}
	}
	if !a.compose.body.ShowCursor {
		t.Fatal("active body cursor is hidden")
	}
}

func TestPeriodicReceiveCommandsDeduplicates(t *testing.T) {
	accounts := []config.Account{
		{Name: "personal", ReceiveCommand: "MailSalonSync -plain sync"},
		{Name: "work", ReceiveCommand: " MailSalonSync -plain sync "},
		{Name: "other", ReceiveCommand: "mbsync other"},
		{Name: "disabled"},
	}
	got := periodicReceiveCommands(accounts)
	want := []string{"MailSalonSync -plain sync", "mbsync other"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("periodicReceiveCommands = %#v, want %#v", got, want)
	}
}

func TestMessageMatchesSearchIncludesBodyAndHeaders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "message.eml")
	raw := "From: Alice <alice@example.com>\r\n" +
		"To: Britney <britney@example.com>\r\n" +
		"Cc: Team <team@example.com>\r\n" +
		"Subject: Weekend plans\r\n" +
		"Date: Mon, 07 Sep 2026 10:00:00 -0400\r\n" +
		"Message-ID: <search@example.com>\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"The secret search phrase is lavender mailbox.\r\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	entry := maildir.Entry{
		Path:      path,
		From:      "Alice",
		Subject:   "Weekend plans",
		Date:      time.Date(2026, 9, 7, 10, 0, 0, 0, time.FixedZone("EDT", -4*60*60)),
		MessageID: "<search@example.com>",
	}
	for _, query := range []string{"ALICE", "weekend", "britney@example.com", "lavender mailbox"} {
		if !messageMatchesSearch(entry, query) {
			t.Fatalf("search did not match %q", query)
		}
	}
	if messageMatchesSearch(entry, "definitely absent") {
		t.Fatal("search matched absent text")
	}
}
