package uiapp

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/pim"
	"github.com/gdamore/tcell/v3"
	ui "github.com/metaspartan/gotui/v5"
)

func calendarViewsApp(t *testing.T) *App {
	t.Helper()
	emptyViewScreen(t)
	root := t.TempDir()
	cfg := config.Default()
	cfg.Accounts = []config.Account{{Name: "personal", Maildir: filepath.Join(root, "mail")}, {Name: "work", Maildir: filepath.Join(root, "workmail")}}
	cfg.DefaultAccount = "personal"
	cfg.Collections = []config.Collection{{Name: "Personal", Account: "personal", Protocol: "caldav", LocalDir: filepath.Join(root, "cal")}, {Name: "Work", Account: "work", Protocol: "jmap-calendars", LocalDir: filepath.Join(root, "workcal")}}
	for i, c := range cfg.Collections {
		data, uid, err := pim.New(c, []string{[]string{"Planning", "Private work"}[i], "2026-10-02T10:00:00", "2026-10-02T11:00:00", "", "Room A"})
		if err != nil {
			t.Fatal(err)
		}
		if err := pim.Save(c, nil, data, uid); err != nil {
			t.Fatal(err)
		}
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.setView(2)
	a.changeCalendarDate(time.Date(2026, 10, 2, 0, 0, 0, 0, time.Local))
	return a
}

func TestCalendarViewsKeyboardMouseAndSelectedDate(t *testing.T) {
	a := calendarViewsApp(t)
	a.render()
	c := a.calendarViews
	if c.mode != "month" || len(c.rows) != 1 || c.rows[0].Title != "Planning" {
		t.Fatal(c.mode, c.rows)
	}
	for i, mode := range []string{"month", "week", "day", "agenda"} {
		p := c.tabs[i]
		a.handlePIMMouse(mouse("<MouseLeft>", p.Rectangle.Min.X+2, 1))
		a.render()
		if c.mode != mode || !sameDate(c.date, time.Date(2026, 10, 2, 0, 0, 0, 0, time.Local)) {
			t.Fatal("mouse mode/date", c.mode, c.date)
		}
	}
	for _, tc := range []struct{ key, mode string }{{"M", "month"}, {"W", "week"}, {"D", "day"}, {"G", "agenda"}} {
		a.handlePIMKey(tc.key)
		a.render()
		if c.mode != tc.mode {
			t.Fatal(tc)
		}
	}
	a.handlePIMKey("M")
	a.render()
	a.handlePIMKey("<Right>")
	a.render()
	if c.date.Day() != 3 || len(c.rows) != 0 {
		t.Fatal(c.date, c.rows)
	}
	a.handlePIMKey("<Left>")
	a.handlePIMKey("<Enter>")
	a.handlePIMKey("e")
	if a.pimEditor == nil || a.pimEditor.original.Title != "Planning" {
		t.Fatal("wrong source for edit")
	}
	a.pimEditor = nil
	a.changeCalendarDate(time.Date(2027, 1, 31, 0, 0, 0, 0, time.Local))
	a.handlePIMKey("]")
	if c.date.Day() != 28 || c.date.Month() != time.February {
		t.Fatal("month overflow", c.date)
	}
	a.render()
	p := c.nav[0]
	a.handlePIMMouse(mouse("<MouseLeft>", p.Rectangle.Min.X+2, 2))
	if c.date.Month() != time.January {
		t.Fatal(c.date)
	}
	p = c.nav[1]
	a.handlePIMMouse(mouse("<MouseLeft>", p.Rectangle.Min.X+2, 2))
	if !sameDate(c.date, time.Now()) {
		t.Fatal("today button", c.date)
	}
	a.changeCalendarDate(time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local))
	a.handlePIMKey("n")
	if !strings.HasPrefix(a.pimEditor.fields[1].Text, "2026-10-20T") {
		t.Fatal("creation date", a.pimEditor.fields[1].Text)
	}
}

func TestCalendarGridDatesAndAccountIsolation(t *testing.T) {
	a := calendarViewsApp(t)
	a.render()
	c := a.calendarViews
	if c.from.Weekday() != time.Monday {
		t.Fatal(c.from)
	}
	for i := 0; i < 42; i++ {
		r := c.grid.cellRect(i)
		if r.Empty() {
			t.Fatal("empty cell")
		}
		d, ok := c.grid.dateAt(r.Min)
		if !ok || !sameDate(d, c.from.AddDate(0, 0, i)) {
			t.Fatal(i, d)
		}
	}
	index := int(time.Date(2026, 10, 10, 0, 0, 0, 0, time.Local).Sub(c.from).Hours() / 24)
	r := c.grid.cellRect(index)
	a.handlePIMMouse(mouse("<MouseLeft>", r.Min.X, r.Min.Y))
	a.render()
	if c.date.Day() != 10 {
		t.Fatal("day click", c.date)
	}
	a.changeCalendarMode("week")
	a.render()
	a.switchAccount(1)
	a.changeCalendarDate(time.Date(2026, 10, 2, 0, 0, 0, 0, time.Local))
	a.render()
	if c.mode != "week" || len(c.rows) != 1 || c.rows[0].Title != "Private work" || len(a.pimCollections) != 1 {
		t.Fatal("account/view leakage", c.mode, c.rows, a.pimCollections)
	}
	a.pimQuery = "unmatched"
	a.filterPIM()
	a.render()
	if len(c.rows) != 0 || len(c.occurrences) != 0 {
		t.Fatal("search did not filter grid")
	}
}

func TestCalendarSeriesDeletionAndSourceFallback(t *testing.T) {
	a := calendarViewsApp(t)
	collection := a.pimCollections[0]
	data := []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:series\r\nSUMMARY:Weekly\r\nDTSTART:20261002T120000\r\nRRULE:FREQ=WEEKLY;COUNT=5\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	if err := pim.Save(collection, nil, data, "series"); err != nil {
		t.Fatal(err)
	}
	if err := a.loadPIM(); err != nil {
		t.Fatal(err)
	}
	a.changeCalendarDate(time.Date(2026, 10, 9, 0, 0, 0, 0, time.Local))
	a.render()
	if len(a.calendarViews.rows) != 1 || a.calendarViews.rows[0].Title != "Weekly" {
		t.Fatal("occurrence missing")
	}
	a.handlePIMKey("d")
	if !strings.Contains(a.status, "entire recurring series") {
		t.Fatal(a.status)
	}
	a.handlePIMKey("d")
	a.render()
	if len(a.calendarViews.rows) != 0 {
		t.Fatal("series not deleted")
	}
	bad := strings.ReplaceAll(string(data), "FREQ=WEEKLY;COUNT=5", "FREQ=HOURLY")
	if err := os.WriteFile(filepath.Join(collection.LocalDir, "bad.ics"), []byte(bad), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.loadPIM(); err != nil {
		t.Fatal(err)
	}
	a.render()
	if len(a.calendarViews.problems) != 1 {
		t.Fatal("missing projection warning")
	}
	a.handlePIMKey("G")
	a.render()
	rows := a.calendarViews.rows
	if len(rows) != 1 || rows[0].problem == "" {
		t.Fatal("source fallback missing", rows)
	}
	a.handlePIMKey("e")
	if a.pimEditor == nil || !strings.Contains(a.pimEditor.raw.Text, "FREQ=HOURLY") {
		t.Fatal("source fallback cannot be edited")
	}
}

func TestCalendarViewsRenderSmallEmptyAndCrowded(t *testing.T) {
	a := calendarViewsApp(t)
	for i := 0; i < 12; i++ {
		c := a.pimCollections[0]
		data, uid, err := pim.New(c, []string{"Another meeting", "2026-10-02T14:00:00", "2026-10-02T15:00:00", "", ""})
		if err != nil {
			t.Fatal(err)
		}
		if err := pim.Save(c, nil, data, uid); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.loadPIM(); err != nil {
		t.Fatal(err)
	}

	screen := ui.DefaultBackend.Screen.(tcell.SimulationScreen)
	for _, size := range []image.Point{{60, 24}, {80, 24}, {120, 40}, {180, 55}} {
		screen.SetSize(size.X, size.Y)
		for _, mode := range []string{"month", "week", "day", "agenda"} {
			a.changeCalendarMode(mode)
			a.render()
			a.pimQuery = "missing"
			a.filterPIM()
			a.render()
			a.pimQuery = ""
			a.filterPIM()
		}
	}
	a.changeCalendarMode("month")
	a.render()
	c := a.calendarViews
	buf := ui.NewBuffer(c.grid.GetRect())
	c.grid.Draw(buf)
	// Selection uses a real cell background rather than styling the day label only.
	for i := 0; i < 42; i++ {
		if sameDate(c.from.AddDate(0, 0, i), c.date) {
			r := c.grid.cellRect(i)
			if buf.GetCell(r.Min).Style.Bg != a.theme.selectedBG {
				t.Fatal("selected day not highlighted")
			}
		}
	}
}

func TestCalendarSundayAndLeapMonth(t *testing.T) {
	a := calendarViewsApp(t)
	c := a.calendarViews
	c.weekStart = time.Sunday
	a.changeCalendarDate(time.Date(2028, 1, 31, 0, 0, 0, 0, time.Local))
	a.changeCalendarPeriod(1)
	a.render()
	if c.date.Day() != 29 || c.from.Weekday() != time.Sunday {
		t.Fatal(c.date, c.from)
	}
	a.handlePIMKey("<Home>")
	if c.date.Day() != 1 || c.date.Month() != time.February {
		t.Fatal("Home left month", c.date)
	}
	a.handlePIMKey("<End>")
	if c.date.Day() != 29 || c.date.Month() != time.February {
		t.Fatal("End left month", c.date)
	}
	a.changeCalendarMode("week")
	a.render()
	if c.from.Weekday() != time.Sunday {
		t.Fatal(c.from)
	}
	a.handlePIMKey("<Tab>")
	a.render()
	if c.gridFocus {
		t.Fatal("Tab did not focus events")
	}
	a.handlePIMKey("<Tab>")
	if a.focus != focusPreview {
		t.Fatal("Tab did not focus details")
	}
}
