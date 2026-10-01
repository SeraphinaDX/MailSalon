package uiapp

import (
	"path/filepath"
	"strings"
	"testing"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/mimeutil"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/pim"
	ui "github.com/metaspartan/gotui/v5"
)

func calendarTestMessage(t *testing.T) *mimeutil.ParsedMessage {
	t.Helper()
	p, err := mimeutil.ParseBytes([]byte("From: alice@example.com\r\nTo: me@example.com\r\nSubject: Invitation\r\nContent-Type: text/calendar; method=REQUEST\r\n\r\nBEGIN:VCALENDAR\r\nVERSION:2.0\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:meeting@example.com\r\nSUMMARY:Planning session\r\nDTSTART:20261002T100000Z\r\nDTEND:20261002T110000Z\r\nLOCATION:Room A\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCalendarPreviewPickerAndAccountScope(t *testing.T) {
	emptyViewScreen(t)
	root := t.TempDir()
	cfg := config.Default()
	cfg.Accounts = []config.Account{{Name: "personal", Maildir: filepath.Join(root, "mail")}, {Name: "work", Maildir: filepath.Join(root, "work")}}
	cfg.DefaultAccount = "personal"
	cfg.Collections = []config.Collection{
		{Name: "Work only", Account: "work", Protocol: "caldav", LocalDir: filepath.Join(root, "other")},
		{Name: "Personal contacts", Account: "personal", Protocol: "carddav", LocalDir: filepath.Join(root, "contacts")},
		{Name: "Shared calendar", Protocol: "caldav", LocalDir: filepath.Join(root, "shared")},
		{Name: "Personal calendar", Account: "personal", Protocol: "jmap-calendars", LocalDir: filepath.Join(root, "personal")},
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.parsed = calendarTestMessage(t)
	a.render()
	if !strings.Contains(a.preview.Text, "Planning session") || !strings.Contains(a.preview.Text, "2026-10-02T10:00:00") || a.calendarButton == nil {
		t.Fatal("event preview/action missing")
	}
	a.handleMouse(ui.Event{Type: ui.MouseEvent, ID: "<MouseLeft>", Payload: ui.Mouse{X: a.calendarButton.Rectangle.Min.X + 2, Y: a.calendarButton.Rectangle.Min.Y + 1}})
	if a.calendarDialog == nil {
		t.Fatal("calendar button did not open picker")
	}
	a.render()
	d := a.calendarDialog
	if len(d.calendars) != 2 || d.calendars[0].Name != "Personal calendar" || d.calendars[1].Account != "" {
		t.Fatal("wrong calendar destinations", d.calendars)
	}
	a.handleCalendarImport(ui.Event{Type: ui.KeyboardEvent, ID: "<C-s>"})
	a.render()
	items, err := pim.Load(d.calendars[0])
	if err != nil || len(items) != 1 || items[0].UID != "meeting@example.com" {
		t.Fatal(items, err)
	}
	a.handleCalendarImport(ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"})
	if !strings.Contains(a.status, "Already in") {
		t.Fatal(a.status)
	}
	a.handleCalendarImport(ui.Event{Type: ui.KeyboardEvent, ID: "<Tab>"})
	a.handleCalendarImport(ui.Event{Type: ui.KeyboardEvent, ID: "<Down>"})
	a.render()
	a.handleCalendarImport(ui.Event{Type: ui.MouseEvent, ID: "<MouseLeft>", Payload: ui.Mouse{X: d.add.Rectangle.Min.X + 2, Y: d.add.Rectangle.Min.Y + 1}})
	items, err = pim.Load(d.calendars[1])
	if err != nil || len(items) != 1 {
		t.Fatal("mouse import failed", items, err)
	}
	a.handleCalendarImport(ui.Event{Type: ui.KeyboardEvent, ID: "<Escape>"})
	a.render()
	if a.calendarDialog != nil {
		t.Fatal("picker not closed")
	}
}

func TestCalendarPickerWithoutCollectionsOrEvents(t *testing.T) {
	emptyViewScreen(t)
	cfg := config.Default()
	cfg.Accounts[0].Maildir = filepath.Join(t.TempDir(), "mail")
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.parsed = calendarTestMessage(t)
	a.handleKey("i")
	a.render()
	a.addCalendarEvent()
	a.render()
	if !strings.Contains(a.status, "No calendar configured") {
		t.Fatal(a.status)
	}
	if a.calendarDialog == nil || len(a.calendarDialog.calendars) != 0 {
		t.Fatal("unexpected destination")
	}
	a.handleCalendarImport(ui.Event{Type: ui.KeyboardEvent, ID: "<Escape>"})
	a.render()
	a.parsed = nil
	a.handleKey("i")
	if a.calendarDialog != nil {
		t.Fatal("picker without calendar events")
	}
}

func TestCalendarDetailsScroll(t *testing.T) {
	emptyViewScreen(t)
	cfg := config.Default()
	cfg.Accounts[0].Maildir = filepath.Join(t.TempDir(), "mail")
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.parsed = calendarTestMessage(t)
	a.parsed.CalendarEvents[0].Description = strings.Repeat("Long description line\n", 100)
	a.startCalendarImport()
	a.render()
	a.handleCalendarImport(ui.Event{Type: ui.KeyboardEvent, ID: "<Tab>"})
	a.handleCalendarImport(ui.Event{Type: ui.KeyboardEvent, ID: "<Tab>"})
	a.handleCalendarImport(ui.Event{Type: ui.KeyboardEvent, ID: bindingEventID(a.bindings().PageDown)})
	a.render()
	if a.calendarDialog.focus != 2 || a.calendarDialog.previewScroll == 0 {
		t.Fatal("details did not scroll")
	}
	a.handleCalendarImport(ui.Event{Type: ui.KeyboardEvent, ID: "<Backtab>"})
	if a.calendarDialog.focus != 1 {
		t.Fatal("Shift+Tab did not select previous pane")
	}
}
