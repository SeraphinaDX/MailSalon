package uiapp

import (
	"fmt"
	"image"
	"sort"
	"strings"
	"time"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/calendar"
	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

type calendarRow struct {
	calendar.Occurrence
	source  int
	problem string
}

type calendarViews struct {
	mode                        string
	date                        time.Time
	weekStart                   time.Weekday
	dirty, gridFocus            bool
	from, to                    time.Time
	occurrences, problems, rows []calendarRow
	selected, offset            int
	tabs                        [4]*widgets.Paragraph
	nav                         [3]*widgets.Paragraph
	period                      *widgets.Paragraph
	grid                        *calendarGrid
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
func sameDate(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}
func weekBeginning(t time.Time, first time.Weekday) time.Time {
	t = dateOnly(t)
	return t.AddDate(0, 0, -(int(t.Weekday())-int(first)+7)%7)
}
func shiftMonth(t time.Time, n int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(n), 1, 0, 0, 0, 0, t.Location())
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(t.Day(), last)-1)
}

func (a *App) ensureCalendarViews() {
	if a.calendarViews != nil {
		return
	}
	c := &calendarViews{mode: a.cfg.CalendarDefaultView, date: dateOnly(time.Now()), weekStart: time.Monday, dirty: true, gridFocus: true, period: widgets.NewParagraph(), grid: newCalendarGrid()}
	if c.mode == "" {
		c.mode = "month"
	}
	if a.cfg.CalendarWeekStart == "sunday" {
		c.weekStart = time.Sunday
	}
	for i := range c.tabs {
		c.tabs[i] = widgets.NewParagraph()
	}
	for i := range c.nav {
		c.nav[i] = widgets.NewParagraph()
	}
	if c.mode == "day" || c.mode == "agenda" {
		c.gridFocus = false
	}
	a.calendarViews = c
}

func (c *calendarViews) window() (time.Time, time.Time) {
	from := c.date
	switch c.mode {
	case "month":
		from = weekBeginning(time.Date(c.date.Year(), c.date.Month(), 1, 0, 0, 0, 0, c.date.Location()), c.weekStart)
		return from, from.AddDate(0, 0, 42)
	case "week":
		from = weekBeginning(c.date, c.weekStart)
		return from, from.AddDate(0, 0, 7)
	case "agenda":
		return from, from.AddDate(0, 0, 30)
	default:
		return from, from.AddDate(0, 0, 1)
	}
}

func (a *App) calendarRows() {
	a.ensureCalendarViews()
	c := a.calendarViews
	from, to := c.window()
	if c.dirty || !from.Equal(c.from) || !to.Equal(c.to) {
		c.from, c.to = from, to
		c.dirty = false
		c.occurrences = nil
		c.problems = nil
		for source, item := range a.pimItems {
			events, err := calendar.Occurrences(item.Data, len(a.pimCollections) > 0 && a.pimCollections[a.pimCollection].Protocol == "jmap-calendars", from, to, time.Local)
			if err != nil {
				c.problems = append(c.problems, calendarRow{source: source, problem: err.Error(), Occurrence: calendar.Occurrence{Title: item.Title}})
				continue
			}
			for _, event := range events {
				c.occurrences = append(c.occurrences, calendarRow{source: source, Occurrence: event})
			}
		}
		sort.SliceStable(c.occurrences, func(i, j int) bool {
			x, y := c.occurrences[i], c.occurrences[j]
			if x.Start.Equal(y.Start) {
				return x.Title < y.Title
			}
			return x.Start.Before(y.Start)
		})
	}
	c.rows = nil
	for _, row := range c.occurrences {
		if c.mode == "agenda" || row.Intersects(c.date, c.date.AddDate(0, 0, 1)) {
			c.rows = append(c.rows, row)
		}
	}
	if c.mode == "agenda" {
		c.rows = append(c.rows, c.problems...)
	}
	c.selected = clamp(c.selected, 0, max(0, len(c.rows)-1))
}

func (a *App) selectCalendarSource() {
	a.calendarRows()
	c := a.calendarViews
	a.pimSelected = -1
	if len(c.rows) > 0 {
		a.pimSelected = c.rows[c.selected].source
	}
}

func (a *App) changeCalendarDate(t time.Time) {
	c := a.calendarViews
	c.date = dateOnly(t)
	c.selected = 0
	c.offset = 0
	a.previewScroll = 0
	a.deleteArmed = false
}
func (a *App) changeCalendarPeriod(n int) {
	c := a.calendarViews
	switch c.mode {
	case "month":
		a.changeCalendarDate(shiftMonth(c.date, n))
	case "week":
		a.changeCalendarDate(c.date.AddDate(0, 0, 7*n))
	case "agenda":
		a.changeCalendarDate(c.date.AddDate(0, 0, 30*n))
	default:
		a.changeCalendarDate(c.date.AddDate(0, 0, n))
	}
}
func (a *App) changeCalendarMode(mode string) {
	c := a.calendarViews
	c.mode = mode
	c.gridFocus = mode == "month" || mode == "week"
	c.selected = 0
	c.offset = 0
	a.previewScroll = 0
	a.focus = focusMessages
	a.deleteArmed = false
}
func (a *App) moveCalendarRow(n int) {
	a.calendarRows()
	c := a.calendarViews
	c.selected = clamp(c.selected+n, 0, max(0, len(c.rows)-1))
	a.previewScroll = 0
}

func (a *App) handleCalendarViewKey(id string) bool {
	a.ensureCalendarViews()
	c := a.calendarViews
	k := a.bindings()
	switch {
	case bindingMatches(id, k.CalendarMonth):
		a.changeCalendarMode("month")
	case bindingMatches(id, k.CalendarWeek):
		a.changeCalendarMode("week")
	case bindingMatches(id, k.CalendarDay):
		a.changeCalendarMode("day")
	case bindingMatches(id, k.CalendarAgenda):
		a.changeCalendarMode("agenda")
	case bindingMatches(id, k.CalendarToday):
		a.changeCalendarDate(time.Now())
	case bindingMatches(id, k.CalendarPrevious):
		a.changeCalendarPeriod(-1)
	case bindingMatches(id, k.CalendarNext):
		a.changeCalendarPeriod(1)
	case bindingMatches(id, k.FocusNext):
		if a.focus == focusFolders {
			a.focus = focusMessages
			c.gridFocus = c.mode == "month" || c.mode == "week"
		} else if a.focus == focusMessages && c.gridFocus {
			c.gridFocus = false
		} else {
			a.focus = (a.focus + 1) % 3
		}
	case id == "<Left>" || id == "<Right>":
		if a.focus != focusMessages || !c.gridFocus {
			return false
		}
		n := 1
		if id == "<Left>" {
			n = -1
		}
		a.changeCalendarDate(c.date.AddDate(0, 0, n))
	case id == "<Up>" || id == "<Down>" || bindingMatches(id, k.MoveUp) || bindingMatches(id, k.MoveDown):
		if a.focus != focusMessages {
			return false
		}
		n := 1
		if id == "<Up>" || bindingMatches(id, k.MoveUp) {
			n = -1
		}
		if c.gridFocus {
			a.changeCalendarDate(c.date.AddDate(0, 0, n*7))
		} else {
			a.moveCalendarRow(n)
		}
	case bindingMatches(id, k.PageUp) || bindingMatches(id, k.PageDown):
		if a.focus != focusMessages {
			return false
		}
		n := 1
		if bindingMatches(id, k.PageUp) {
			n = -1
		}
		a.changeCalendarPeriod(n)
	case bindingMatches(id, k.Home) || bindingMatches(id, k.End):
		if a.focus != focusMessages {
			return false
		}
		if c.gridFocus {
			from, to := c.window()
			if c.mode == "month" {
				from = time.Date(c.date.Year(), c.date.Month(), 1, 0, 0, 0, 0, c.date.Location())
				to = from.AddDate(0, 1, 0)
			}
			if bindingMatches(id, k.End) {
				from = to.AddDate(0, 0, -1)
			}
			a.changeCalendarDate(from)
		} else {
			n := -1 << 20
			if bindingMatches(id, k.End) {
				n = 1 << 20
			}
			a.moveCalendarRow(n)
		}
	case bindingMatches(id, k.Open):
		if a.focus == focusMessages && c.gridFocus {
			c.gridFocus = false
		} else if a.focus == focusFolders {
			a.focus = focusMessages
			c.gridFocus = c.mode == "month" || c.mode == "week"
		} else {
			a.focus = focusPreview
		}
	default:
		return false
	}
	return true
}

func calendarTime(row calendarRow, agenda bool) string {
	if row.problem != "" {
		return "[source]"
	}
	label := row.Start.Format("15:04") + "–" + row.End.Format("15:04")
	if row.AllDay {
		label = "All day"
	}
	if agenda {
		label = row.Start.Format("Mon Jan 02") + " " + label
	}
	return label
}

func (a *App) renderCalendarViews(w, h int) {
	a.calendarRows()
	c := a.calendarViews
	left := clamp(w/5, 18, 30)
	footer := h - 3
	a.accountBar.SetRect(0, 1, left, 4)
	a.updateBar.SetRect(0, 4, left, 7)
	a.folderList.SetRect(0, 7, left, footer)
	a.populateAccountBar()
	a.populateUpdateBar()
	a.updateBar.Title = "Sync collections"
	a.folderList.Title = "Calendars"
	a.folderOffset = keepVisible(a.pimCollection, a.folderOffset, max(1, a.folderList.Inner.Dy()), len(a.pimCollections))
	a.folderList.Rows = nil
	for i := a.folderOffset; i < min(len(a.pimCollections), a.folderOffset+a.folderList.Inner.Dy()); i++ {
		name := a.pimCollections[i].Name
		if a.pimCollections[i].Account == "" {
			name = "Shared: " + name
		}
		a.folderList.Rows = append(a.folderList.Rows, safeUI(name))
	}
	a.folderList.SelectedRow = a.pimCollection - a.folderOffset
	if len(a.pimCollections) == 0 {
		a.folderList.Rows = []string{"(no calendars)"}
		a.folderList.SelectedRow = 0
	}
	items := append(a.layoutViewTabs(w), a.accountBar, a.updateBar, a.folderList)
	labels := []string{"Month", "Week", "Day", "Agenda"}
	modes := []string{"month", "week", "day", "agenda"}
	keys := []string{keyLabel(a.bindings().CalendarMonth), keyLabel(a.bindings().CalendarWeek), keyLabel(a.bindings().CalendarDay), keyLabel(a.bindings().CalendarAgenda)}
	for i, p := range c.tabs {
		setBarRect(p, left+(w-left)*i/4, 1, left+(w-left)*(i+1)/4, 2)
		p.Text = keys[i] + " " + labels[i]
		a.styleCalendarParagraph(p, c.mode == modes[i])
		p.TextAlignment = ui.AlignCenter
		items = append(items, p)
	}
	navLabels := []string{"‹ Previous", "Today", "Next ›"}
	for i, p := range c.nav {
		setBarRect(p, left+(w-left)*i/3, 2, left+(w-left)*(i+1)/3, 3)
		p.Text = navLabels[i]
		p.TextAlignment = ui.AlignCenter
		a.styleCalendarParagraph(p, false)
		items = append(items, p)
	}
	title := c.date.Format("Monday, 2 January 2006")
	if c.mode == "month" {
		title = c.date.Format("January 2006")
	} else if c.mode == "week" {
		title = c.from.Format("2 Jan") + " – " + c.to.AddDate(0, 0, -1).Format("2 Jan 2006")
	} else if c.mode == "agenda" {
		title = c.date.Format("2 Jan") + " – " + c.to.AddDate(0, 0, -1).Format("2 Jan 2006")
	}
	if len(c.problems) > 0 {
		title = fmt.Sprintf("%s: %d source items · ", keyLabel(a.bindings().CalendarAgenda), len(c.problems)) + title
	}
	setBarRect(c.period, left, 3, w, 4)
	c.period.Text = safeUI(title + " · " + time.Now().Location().String())
	a.styleCalendarParagraph(c.period, false)
	items = append(items, c.period)
	top := 4
	if c.mode == "month" || c.mode == "week" {
		end := max(top+8, footer-max(6, h/4))
		end = min(end, footer-4)
		c.grid.SetRect(left, top, w, end)
		c.grid.date = c.date
		c.grid.first = c.from
		c.grid.week = c.mode == "week"
		c.grid.rows = c.occurrences
		c.grid.theme = a.theme
		c.grid.focused = a.focus == focusMessages && c.gridFocus
		items = append(items, c.grid)
		top = end
	}
	split := left + (w-left)*55/100
	if c.mode == "day" || c.mode == "agenda" {
		split = left + (w-left)*60/100
	}
	a.messageTbl.SetRect(left, top, split, footer)
	a.preview.SetRect(split, top, w, footer)
	a.messageTbl.Title = c.date.Format("Mon 2 Jan") + " events"
	if c.mode == "agenda" {
		a.messageTbl.Title = "Upcoming 30 days + source items"
	}
	visible := max(1, a.messageTbl.Inner.Dy()-1)
	c.offset = keepVisible(c.selected, c.offset, visible, len(c.rows))
	a.messageTbl.Rows = [][]string{{"Time", "Event"}}
	for i := c.offset; i < min(len(c.rows), c.offset+visible); i++ {
		r := c.rows[i]
		title := r.Title
		if r.Recurring {
			title += " ↻"
		}
		a.messageTbl.Rows = append(a.messageTbl.Rows, []string{safeUI(calendarTime(r, c.mode == "agenda")), safeUI(title)})
	}
	if len(c.rows) == 0 {
		a.messageTbl.Rows = append(a.messageTbl.Rows, []string{"", "No events"})
	}
	timeWidth := 12
	if c.mode == "agenda" {
		timeWidth = 26
	}
	timeWidth = min(timeWidth, max(8, a.messageTbl.Inner.Dx()/2))
	a.messageTbl.ColumnWidths = []int{timeWidth, max(1, a.messageTbl.Inner.Dx()-timeWidth-1)}
	a.messageTbl.SelectedRow = c.selected - c.offset + 1
	a.preview.Title = "Event details"
	a.preview.TitleBottom = "Scroll / wheel"
	details := "No events on this date.\nChoose another day, or press n to create an event."
	if len(a.pimCollections) == 0 {
		details = "No collections configured for this view.\nAdd a calendar in config.toml; see docs/contacts-calendar.md."
	}
	if len(c.rows) > 0 {
		r := c.rows[c.selected]
		item := a.pimItems[r.source]
		if r.problem != "" {
			details = r.Title + "\nCannot place this resource on the calendar: " + r.problem + "\n\nStored start: " + item.Start
		} else {
			details = r.Title + "\n" + r.Start.Format("Mon 2 Jan 2006 15:04 MST") + "\nEnd: " + r.End.Format("Mon 2 Jan 2006 15:04 MST")
			if r.AllDay {
				details = r.Title + "\nAll day: " + r.Start.Format("2006-01-02") + "\nEnd (exclusive): " + r.End.Format("2006-01-02")
			}
			details += "\nLocation: " + r.Location + "\n" + r.Notes
		}
		if r.Recurring || item.Recurring {
			details += "\nRecurring series: Edit/Delete affects the entire stored series."
		}
		details += "\n\nStored source:\n" + string(item.Data)
	}
	lines := wrapLines(safeUI(details), max(1, a.preview.Inner.Dx()))
	a.previewScroll = clamp(a.previewScroll, 0, max(0, len(lines)-a.preview.Inner.Dy()))
	a.preview.Text = strings.Join(lines[a.previewScroll:], "\n")
	setBarRect(a.footer, 0, footer, w, h)
	k := a.bindings()
	a.footer.Text = safeUI(fmt.Sprintf(" [%s] %s — %s\n %s/%s/%s/%s Views  %s/%s Period  %s Today  Arrows Dates  Enter Events  Tab Focus\n n New  %s Edit source  %s Delete  %s Search  %s Sync  %s Refresh  %s Account  %s Quit", a.currentAccount().Name, a.pimScope(), a.status, keyLabel(k.CalendarMonth), keyLabel(k.CalendarWeek), keyLabel(k.CalendarDay), keyLabel(k.CalendarAgenda), keyLabel(k.CalendarPrevious), keyLabel(k.CalendarNext), keyLabel(k.CalendarToday), keyLabel(k.Archive), keyLabel(k.Delete), keyLabel(k.Search), keyLabel(k.Sync), keyLabel(k.Refresh), keyLabel(k.SwitchAccount), keyLabel(k.Quit)))
	a.updateFocusStyles()
	if a.focus == focusMessages && c.gridFocus {
		a.messageTbl.BorderStyle = ui.NewStyle(a.theme.border, a.theme.background)
	}
	items = append(items, a.messageTbl, a.preview, a.footer)
	if a.searchActive {
		a.searchPrompt.SetRect(left+2, max(1, h/2-2), w-2, max(1, h/2-2)+3)
		items = append(items, a.searchPrompt)
	}
	ui.Render(items...)
}

func (a *App) styleCalendarParagraph(p *widgets.Paragraph, active bool) {
	p.Border = false
	p.BackgroundColor = a.theme.background
	p.TextStyle = ui.NewStyle(a.theme.foreground, a.theme.background)
	p.WrapText = false
	if active {
		p.BackgroundColor = a.theme.selectedBG
		p.TextStyle = ui.NewStyle(a.theme.selectedFG, a.theme.selectedBG)
	}
}

func (a *App) handleCalendarViewMouse(e ui.Event) {
	if a.handleViewTabMouse(e) {
		return
	}
	m, ok := e.Payload.(ui.Mouse)
	if !ok {
		return
	}
	p := image.Pt(m.X, m.Y)
	a.ensureCalendarViews()
	c := a.calendarViews
	if e.ID == "<MouseLeft>" || e.ID == "MouseLeft" {
		for i, tab := range c.tabs {
			if p.In(tab.Rectangle) {
				a.changeCalendarMode([]string{"month", "week", "day", "agenda"}[i])
				return
			}
		}
		for i, nav := range c.nav {
			if p.In(nav.Rectangle) {
				if i == 1 {
					a.changeCalendarDate(time.Now())
				} else {
					a.changeCalendarPeriod(i - 1)
				}
				return
			}
		}
		switch {
		case p.In(a.accountBar.Rectangle):
			a.switchAccount(1)
		case p.In(a.updateBar.Rectangle):
			a.runSync()
		case p.In(a.folderList.Inner):
			a.focus = focusFolders
			row := a.folderOffset + p.Y - a.folderList.Inner.Min.Y
			if row >= 0 && row < len(a.pimCollections) {
				a.pimCollection = row
				a.pimQuery = ""
				if err := a.loadPIM(); err != nil {
					a.setError(err)
				}
			}
		case (c.mode == "month" || c.mode == "week") && p.In(c.grid.Inner):
			if date, ok := c.grid.dateAt(p); ok {
				a.changeCalendarDate(date)
				a.focus = focusMessages
				c.gridFocus = true
			}
		case p.In(a.messageTbl.Inner):
			a.focus = focusMessages
			c.gridFocus = false
			row := c.offset + p.Y - a.messageTbl.Inner.Min.Y - 1
			if row >= 0 && row < len(c.rows) {
				c.selected = row
				a.previewScroll = 0
				a.deleteArmed = false
			}
		case p.In(a.preview.Inner):
			a.focus = focusPreview
		}
	} else if strings.Contains(e.ID, "MouseWheel") {
		n := 1
		if strings.Contains(e.ID, "Up") {
			n = -1
		}
		switch {
		case p.In(a.folderList.Inner):
			a.focus = focusFolders
			a.movePIM(n)
		case p.In(a.preview.Inner):
			a.focus = focusPreview
			a.previewScroll = max(0, a.previewScroll+n*3)
		case p.In(a.messageTbl.Inner):
			a.focus = focusMessages
			c.gridFocus = false
			a.moveCalendarRow(n)
		case (c.mode == "month" || c.mode == "week") && p.In(c.grid.Inner):
			a.focus = focusMessages
			c.gridFocus = true
			a.changeCalendarPeriod(n)
		}
	}
}
