package uiapp

import (
	"image"
	"path/filepath"
	"strings"
	"testing"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/pim"
	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

func viewTestApp(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	cfg := config.Default()
	cfg.Accounts = []config.Account{{Name: "cerberus", Maildir: filepath.Join(root, "mail-1")}, {Name: "legalbeaver", Maildir: filepath.Join(root, "mail-2")}}
	cfg.DefaultAccount = "cerberus"
	for _, account := range []string{"cerberus", "legalbeaver", ""} {
		name := account
		if name == "" {
			name = "shared"
		}
		for _, protocol := range []string{"carddav", "caldav"} {
			c := config.Collection{Name: name + " " + protocol, Account: account, Protocol: protocol, LocalDir: filepath.Join(root, name, protocol)}
			values := []string{name, name + "@example.test", ""}
			if protocol == "caldav" {
				values = []string{name, "2026-09-30T12:00:00", "2026-09-30T13:00:00", "UTC", ""}
			}
			data, uid, err := pim.New(c, values)
			if err != nil {
				t.Fatal(err)
			}
			if err := pim.Save(c, nil, data, uid); err != nil {
				t.Fatal(err)
			}
			cfg.Collections = append(cfg.Collections, c)
		}
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestViewTabsRenderKeysHighlightAndClick(t *testing.T) {
	a := viewTestApp(t)
	for _, width := range []int{60, 120} {
		for _, view := range []int{0, 1, 2} {
			a.setView(view)
			items := a.layoutViewTabs(width)
			buf := ui.NewBuffer(image.Rect(0, 0, width, 1))
			for i, item := range items {
				item.Draw(buf)
				if a.viewTabs[i].Inner.Dy() != 1 {
					t.Fatal("one-row view tab clipped")
				}
				selected := a.viewTabs[i].TextStyle.Bg == a.theme.selectedBG
				if selected != (i == view) {
					t.Fatalf("view %d tab %d highlight", view, i)
				}
			}
			var text strings.Builder
			for x := 0; x < width; x++ {
				text.WriteRune(buf.GetCell(image.Pt(x, 0)).Rune)
			}
			for _, want := range []string{"1 Mail", "2 Contacts", "3 Calendar"} {
				if !strings.Contains(text.String(), want) {
					t.Fatalf("width %d missing %q", width, want)
				}
			}
		}
	}
	// Exercise actual mouse routing from every view, not just the tab helper.
	for _, target := range []int{1, 2, 0} {
		a.layoutViewTabs(120)
		e := mouse("<MouseLeft>", target*40+4, 0)
		account := a.account
		if a.view == 0 {
			a.handleMouse(e)
		} else {
			a.handlePIMMouse(e)
		}
		if a.view != target || a.account != account {
			t.Fatalf("click view=%d account=%d", a.view, a.account)
		}
	}
	a.cfg.Keybindings.MailView = "F1"
	a.cfg.Keybindings.ContactsView = "F2"
	a.cfg.Keybindings.CalendarView = "F3"
	a.layoutViewTabs(120)
	for i, key := range []string{"F1", "F2", "F3"} {
		if !strings.Contains(a.viewTabs[i].Text, key) {
			t.Fatal("configured shortcut missing")
		}
	}
}

func TestAccountSwitchFiltersContactsAndCalendarWithSharedScope(t *testing.T) {
	a := viewTestApp(t)
	for _, view := range []int{1, 2} {
		a.account = 0
		a.setView(view)
		for _, want := range []string{"cerberus", "legalbeaver", "cerberus"} {
			if want != a.currentAccount().Name {
				a.handleKey("A")
			}
			if a.view != view || len(a.pimCollections) != 2 {
				t.Fatalf("view=%d collections=%v", a.view, a.pimCollections)
			}
			for _, c := range a.pimCollections {
				if c.Account != "" && c.Account != want {
					t.Fatalf("leaked %s into %s", c.Name, want)
				}
			}
			if len(a.pimItems) != 1 || a.pimItems[0].Title != want {
				t.Fatalf("items did not reload for %s: %v", want, a.pimItems)
			}
			if !strings.Contains(a.pimScope(), want+" only") {
				t.Fatal("account scope not clear")
			}
		}
		a.pimCollection = 1
		if !strings.Contains(a.pimScope(), "shared across accounts") {
			t.Fatal("shared scope not clear")
		}
		// Clicking the account selector must also reload the current view.
		a.accountBar.SetRect(0, 1, 24, 4)
		a.handlePIMMouse(mouse("<MouseLeft>", 2, 2))
		if a.currentAccount().Name != "legalbeaver" || a.pimItems[0].Title != "legalbeaver" {
			t.Fatal("account click did not filter items")
		}
	}
}

func TestBorderlessFooterDrawsAllHelpRows(t *testing.T) {
	a := viewTestApp(t)
	p := widgets.NewParagraph()
	p.Border = false
	p.WrapText = false
	p.Text = a.footerText()
	setBarRect(p, 0, 0, 120, 3)
	buf := ui.NewBuffer(p.GetRect())
	p.Draw(buf)
	for y, want := range []string{"cerberus", "1 Mail", "Switch account"} {
		var row strings.Builder
		for x := 0; x < 120; x++ {
			row.WriteRune(buf.GetCell(image.Pt(x, y)).Rune)
		}
		if !strings.Contains(row.String(), want) {
			t.Fatalf("footer row %d missing %q: %q", y, want, row.String())
		}
	}
}
