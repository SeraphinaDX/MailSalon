package uiapp

import (
	"path/filepath"
	"strings"
	"testing"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/pim"
	ui "github.com/metaspartan/gotui/v5"
)

func TestPIMAccountViewsSearchAndCompose(t *testing.T) {
	root := t.TempDir()
	contacts := config.Collection{Name: "Personal", Account: "personal", Protocol: "carddav", LocalDir: filepath.Join(root, "contacts")}
	data, uid, _ := pim.New(contacts, []string{"Alice Example", "alice@example.test", ""})
	if err := pim.Save(contacts, nil, data, uid); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Accounts = []config.Account{{Name: "personal", Maildir: filepath.Join(root, "mail"), From: "Me <me@example.test>"}, {Name: "work", Maildir: filepath.Join(root, "work")}}
	cfg.DefaultAccount = "personal"
	cfg.Collections = []config.Collection{contacts, {Name: "Work calendar", Account: "work", Protocol: "caldav", LocalDir: filepath.Join(root, "calendar")}}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.setView(1)
	if len(a.pimCollections) != 1 || len(a.pimItems) != 1 {
		t.Fatal("contacts not loaded")
	}
	a.applySearch("missing")
	if len(a.pimItems) != 0 {
		t.Fatal("search didn't filter")
	}
	a.applySearch("alice")
	a.composeContact()
	if a.compose == nil || !strings.Contains(a.compose.to.Text, "alice@example.test") || a.compose.field != composeSubject {
		t.Fatal("compose recipient not populated")
	}
	a.compose = nil
	a.account = 1
	a.setView(1)
	if len(a.pimCollections) != 0 {
		t.Fatal("leaked personal collection into work account")
	}
	a.setView(2)
	if len(a.pimCollections) != 1 {
		t.Fatal("work calendar missing")
	}
}
func TestPIMNewEditorAndCancellation(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	cfg.Accounts[0].Maildir = filepath.Join(root, "mail")
	cfg.Collections = []config.Collection{{Name: "Contacts", Protocol: "carddav", LocalDir: filepath.Join(root, "contacts")}}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.setView(1)
	a.startPIMEditor(false)
	a.pimEditor.fields[0].Text = "Alice"
	a.pimEditor.fields[1].Text = "alice@example.test"
	a.handlePIMEditor(ui.Event{Type: ui.KeyboardEvent, ID: "<C-s>"})
	if a.pimEditor != nil || len(a.pimItems) != 1 {
		t.Fatalf("new item not saved: %s", a.status)
	}
	a.startPIMEditor(true)
	a.handlePIMEditor(ui.Event{Type: ui.KeyboardEvent, ID: "<Escape>"})
	if a.pimEditor != nil {
		t.Fatal("editor not cancelled")
	}
}
