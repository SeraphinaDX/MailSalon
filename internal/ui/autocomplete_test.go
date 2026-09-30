package uiapp

import (
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/mimeutil"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/pim"
)

func completionApp(t *testing.T) *App {
	t.Helper()
	a := viewTestApp(t)
	// Add a second Alice match and an alias in the cerberus book.
	c := a.cfg.Collections[0]
	data := []byte("BEGIN:VCARD\nVERSION:4.0\nUID:alice\nFN:Alice, Example\nEMAIL:alice@example.test\nEMAIL:alias@example.test\nEND:VCARD\n")
	if err := pim.Save(c, nil, data, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := pim.EnsureContact(c, mail.Address{Name: "Alice Other", Address: "other@example.test"}); err != nil {
		t.Fatal(err)
	}
	a.startCompose(nil, false)
	return a
}

func TestRecipientSpanQuotedCommaUnicodeAndCursor(t *testing.T) {
	for _, tt := range []struct {
		text               string
		cursor, start, end int
		query              string
	}{
		{"Al", 2, 0, 2, "Al"},
		{`"Example, Bob" <bob@example.test>,  Al, last@example.test`, 38, 36, 38, "Al"},
		{"éé,  Al", 7, 5, 7, "Al"},
		{"Alison, next@example.test", 2, 0, 6, "Al"},
		{"   ", 3, 3, 3, ""},
	} {
		s, e, q := recipientSpan(tt.text, tt.cursor)
		if s != tt.start || e != tt.end || q != tt.query {
			t.Fatalf("%q: got %d %d %q", tt.text, s, e, q)
		}
	}
}

func TestAutocompleteAllRecipientsAndPreservesOtherAddresses(t *testing.T) {
	a := completionApp(t)
	for _, field := range []composeField{composeTo, composeCc, composeBcc} {
		a.compose.field = field
		in := a.activeInput()
		in.Text = `"Example, Bob" <bob@example.test>, Al, last@example.test`
		in.Cursor = 37
		a.refreshCompletion()
		if len(a.compose.completion.matches) != 3 {
			t.Fatalf("matches=%v", a.compose.completion.matches)
		}
		a.handleComposeEvent(keyboard("<Down>"))
		a.handleComposeEvent(keyboard("<Enter>"))
		parsed, err := mail.ParseAddressList(in.Text)
		if err != nil || len(parsed) != 3 || parsed[0].Address != "bob@example.test" || parsed[1].Address != "alias@example.test" || parsed[2].Address != "last@example.test" {
			t.Fatalf("replacement=%q err=%v", in.Text, err)
		}
		if a.compose.field != field {
			t.Fatal("completion moved focus")
		}
		// The completed address is followed by a comma, so Tab now moves on.
		a.handleComposeEvent(keyboard("<Tab>"))
		if a.compose.field == field {
			t.Fatal("Tab on completed address didn't move focus")
		}
	}
}

func TestAutocompleteScopeAliasMouseAndDismissal(t *testing.T) {
	a := completionApp(t)
	addresses := a.compose.completion.addresses
	for _, addr := range addresses {
		if addr.Address == "legalbeaver@example.test" {
			t.Fatal("another account leaked into suggestions")
		}
	}
	a.compose.to.Text, a.compose.to.Cursor = "alias", 5
	a.compose.to.SetRect(0, 3, 120, 6)
	popup := a.completionPopup(120, 40)
	if popup == nil || len(a.compose.completion.matches) != 1 {
		t.Fatal("secondary email not suggested")
	}
	list := a.compose.completion.list
	a.handleComposeMouse(mouse("<MouseLeft>", list.Inner.Min.X, list.Inner.Min.Y))
	if !strings.Contains(a.compose.to.Text, "alias@example.test") {
		t.Fatal("click did not accept")
	}
	a.compose.to.Text, a.compose.to.Cursor = "Al", 2
	a.handleComposeEvent(keyboard("<Escape>"))
	if a.compose == nil {
		t.Fatal("Escape cancelled compose instead of dismissing suggestions")
	}
	a.refreshCompletion()
	if len(a.compose.completion.matches) != 0 {
		t.Fatal("dismissed popup reopened")
	}
	a.handleComposeEvent(keyboard("<Escape>"))
	if a.compose != nil {
		t.Fatal("second Escape didn't cancel")
	}
	a.startCompose(nil, false)
	a.cycleComposeAccount(1)
	a.compose.field = composeTo
	a.compose.to.Text, a.compose.to.Cursor = "cerberus", 8
	a.refreshCompletion()
	if len(a.compose.completion.matches) != 0 {
		t.Fatal("account switch kept old autocomplete cache")
	}
}

func TestReplySavesToPreferredAccountAndAvoidsDuplicates(t *testing.T) {
	for _, protocol := range []string{"carddav", "jmap-contacts"} {
		t.Run(protocol, func(t *testing.T) {
			a := viewTestApp(t)
			a.cfg.Collections = nil
			for _, name := range []string{"cerberus", "legalbeaver"} {
				a.cfg.Collections = append(a.cfg.Collections, config.Collection{Name: name, Account: name, Protocol: protocol, LocalDir: filepath.Join(t.TempDir(), name)})
			}
			a.cfg.Accounts[1].From = "legalbeaver@example.test"
			source := &mimeutil.ParsedMessage{From: "New Person <new@example.test>", To: "legalbeaver@example.test", Subject: "Hello"}
			a.startCompose(source, false)
			if a.compose.account != 1 || !strings.Contains(a.status, "Saved contact in legalbeaver") {
				t.Fatalf("account=%d status=%s", a.compose.account, a.status)
			}
			items, err := pim.Load(a.cfg.Collections[1])
			if err != nil || len(items) != 1 || items[0].Email != "new@example.test" || items[0].Title != "New Person" {
				t.Fatalf("saved=%v err=%v", items, err)
			}
			a.startCompose(source, false)
			items, _ = pim.Load(a.cfg.Collections[1])
			if len(items) != 1 {
				t.Fatal("second reply duplicated contact")
			}
			other, _ := pim.Load(a.cfg.Collections[0])
			if len(other) != 0 {
				t.Fatal("saved to wrong account")
			}
			a.cfg.AutoAddReplyContacts = false
			source.From = "Disabled <disabled@example.test>"
			a.startCompose(source, false)
			items, _ = pim.Load(a.cfg.Collections[1])
			if len(items) != 1 {
				t.Fatal("disable option ignored")
			}
			a.cfg.AutoAddReplyContacts = true
			a.startCompose(source, true)
			items, _ = pim.Load(a.cfg.Collections[1])
			if len(items) != 1 {
				t.Fatal("forward added sender")
			}
		})
	}
}

func TestReplySecondaryDuplicateAndLockKeepsComposerOpen(t *testing.T) {
	a := completionApp(t)
	a.startCompose(&mimeutil.ParsedMessage{From: "ALIAS@example.test"}, false)
	items, _ := pim.Load(a.cfg.Collections[0])
	if len(items) != 3 {
		t.Fatalf("alias reply created duplicate: %d", len(items))
	}
	if err := os.WriteFile(filepath.Join(a.cfg.Collections[0].LocalDir, ".mss-lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	a.startCompose(&mimeutil.ParsedMessage{From: "new@example.test"}, false)
	if a.compose == nil || !strings.Contains(a.status, "Contact not saved") {
		t.Fatalf("locked reply: %s", a.status)
	}
	items, _ = pim.Load(a.cfg.Collections[0])
	if len(items) != 3 {
		t.Fatal("wrote while syncing")
	}
}
