package uiapp

import (
	"path/filepath"
	"strings"
	"testing"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/pim"
	"github.com/gdamore/tcell/v3"
	ui "github.com/metaspartan/gotui/v5"
)

// Use the real widget drawing path so invalid row indices cannot hide behind
// Render's no-screen shortcut in headless tests.
func emptyViewScreen(t *testing.T) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(120, 40)
	previous := ui.DefaultBackend.Screen
	ui.DefaultBackend.Screen = screen
	t.Cleanup(func() { ui.DefaultBackend.Screen = previous; screen.Fini() })
}

func TestClickPIMTabsWithoutCollections(t *testing.T) {
	emptyViewScreen(t)
	cfg := config.Default()
	cfg.Accounts[0].Maildir = filepath.Join(t.TempDir(), "mail")
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.render()
	for _, view := range []int{1, 2, 0, 2, 1, 0} {
		event := ui.Event{Type: ui.MouseEvent, ID: "<MouseLeft>", Payload: ui.Mouse{X: view*40 + 5, Y: 0}}
		if !a.handleViewTabMouse(event) {
			t.Fatal("tab click not handled")
		}
		a.render()
		if a.view != view {
			t.Fatalf("clicked view %d, got %d", view, a.view)
		}
		if view != 0 {
			if len(a.pimCollections) != 0 || !strings.Contains(a.preview.Text, "No collections configured") {
				t.Fatalf("missing empty-view guidance: %q", a.preview.Text)
			}
			a.handlePIMKey("<Down>")
			a.handlePIMKey("n")
			a.handlePIMKey("d")
			a.render()
			if a.pimEditor != nil {
				t.Fatal("opened editor without a collection")
			}
		}
	}
}

func TestPIMEmptyAndAccountScopedViewsRender(t *testing.T) {
	emptyViewScreen(t)
	root := t.TempDir()
	contacts := config.Collection{Name: "Personal contacts", Account: "personal", Protocol: "carddav", LocalDir: filepath.Join(root, "contacts")}
	calendar := config.Collection{Name: "Personal calendar", Account: "personal", Protocol: "caldav", LocalDir: filepath.Join(root, "calendar")}
	cfg := config.Default()
	cfg.Accounts = []config.Account{{Name: "personal", Maildir: filepath.Join(root, "mail")}, {Name: "work", Maildir: filepath.Join(root, "work")}}
	cfg.DefaultAccount = "personal"
	cfg.Collections = []config.Collection{contacts, calendar}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Configured but never synced: local directories do not exist yet.
	for _, view := range []int{1, 2} {
		a.setView(view)
		a.render()
	}
	data, uid, err := pim.New(contacts, []string{"Alice", "alice@example.test", ""})
	if err != nil {
		t.Fatal(err)
	}
	if err := pim.Save(contacts, nil, data, uid); err != nil {
		t.Fatal(err)
	}
	a.setView(1)
	a.render()
	a.applySearch("missing contact")
	a.render()
	if len(a.pimItems) != 0 {
		t.Fatal("search should have no matches")
	}
	a.applySearch("")
	a.render()
	a.switchAccount(1)
	a.render()
	if len(a.pimCollections) != 0 {
		t.Fatal("personal collections leaked into work account")
	}
	a.switchAccount(-1)
	a.render()
	if len(a.pimItems) != 1 {
		t.Fatal("personal contact not restored")
	}
	a.handlePIMKey("d")
	a.handlePIMKey("d")
	a.render()
	if len(a.pimItems) != 0 {
		t.Fatal("last contact not deleted")
	}
}

func TestEmptyMailFolderListDraws(t *testing.T) {
	emptyViewScreen(t)
	cfg := config.Default()
	cfg.Accounts[0].Maildir = t.TempDir() // Existing container with no Maildirs.
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.render()
	if len(a.folders) != 0 {
		t.Fatal("fixture should have no folders")
	}
}
