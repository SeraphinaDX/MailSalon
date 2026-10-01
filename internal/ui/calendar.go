package uiapp

import (
	"fmt"
	"image"
	"strings"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/calendar"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/pim"
	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

type calendarDialog struct {
	events                                                                []calendar.Event
	calendars                                                             []config.Collection
	event, destination, focus, eventOffset, calendarOffset, previewScroll int
	eventList, calendarList                                               *widgets.List
	preview, add, cancel, footer                                          *widgets.Paragraph
}

func (a *App) calendarCollections() []config.Collection {
	var calendars []config.Collection
	for _, shared := range []bool{false, true} {
		for _, c := range a.cfg.Collections {
			if (c.Protocol == "caldav" || c.Protocol == "jmap-calendars") && (c.Account == "") == shared && (shared || strings.EqualFold(c.Account, a.currentAccount().Name)) {
				calendars = append(calendars, c)
			}
		}
	}
	return calendars
}
func (a *App) hasCalendarEvents() bool { return a.parsed != nil && len(a.parsed.CalendarEvents) > 0 }

func (a *App) startCalendarImport() {
	if !a.hasCalendarEvents() {
		a.status = "This message has no readable calendar events"
		return
	}
	a.calendarDialog = &calendarDialog{events: append([]calendar.Event(nil), a.parsed.CalendarEvents...), calendars: a.calendarCollections(), eventList: widgets.NewList(), calendarList: widgets.NewList(), preview: widgets.NewParagraph(), add: widgets.NewParagraph(), cancel: widgets.NewParagraph(), footer: widgets.NewParagraph()}
	a.deleteArmed = false
	a.status = "Choose an event and calendar; Ctrl+S adds locally"
}

func eventDetails(e calendar.Event) string {
	kind := "Event"
	if e.Method != "" {
		kind += " (" + e.Method + ")"
	}
	if e.Recurring {
		kind += " — recurring series"
	}
	start := e.Start
	if e.AllDay {
		start += " (all-day; end date exclusive)"
	} else if e.Zone != "" {
		start += " [" + e.Zone + "]"
	} else if start != "" {
		start += " (floating time)"
	}
	details := fmt.Sprintf("%s: %s\nStart: %s\n", kind, e.Title, start)
	if e.End != "" {
		end := e.End
		if e.EndZone != "" && e.EndZone != e.Zone {
			end += " [" + e.EndZone + "]"
		}
		details += "End/duration: " + end + "\n"
	}
	if e.Location != "" {
		details += "Location: " + e.Location + "\n"
	}
	if e.Organizer != "" {
		details += "Organizer: " + e.Organizer + "\n"
	}
	if len(e.Attendees) > 0 {
		details += "Attendees: " + strings.Join(e.Attendees, ", ") + "\n"
	}
	if e.Description != "" {
		details += "\n" + e.Description + "\n"
	}
	return details
}

func (a *App) calendarSummary() string {
	if a.parsed == nil {
		return ""
	}
	var b strings.Builder
	for _, e := range a.parsed.CalendarEvents {
		b.WriteString("\n" + eventDetails(e))
	}
	for _, err := range a.parsed.CalendarErrors {
		b.WriteString("\nCalendar attachment could not be read: " + err + "\nSave the original using " + keyLabel(a.bindings().SaveAttachments) + ".\n")
	}
	return b.String()
}

func (a *App) addCalendarEvent() {
	d := a.calendarDialog
	if d == nil {
		return
	}
	if len(d.calendars) == 0 {
		a.status = "No calendar configured for this account; see docs/contacts-calendar.md"
		return
	}
	c := d.calendars[d.destination]
	added, err := pim.ImportCalendar(c, d.events[d.event])
	if err != nil {
		a.status = "Calendar import failed: " + err.Error()
		return
	}
	if added {
		a.status = "Added to " + c.Name + "; sync to upload. No RSVP sent."
	} else {
		a.status = "Already in " + c.Name + "; existing event unchanged"
	}
}

func (a *App) handleCalendarImport(event ui.Event) bool {
	d := a.calendarDialog
	if d == nil {
		return false
	}
	k := a.bindings()
	if event.Type == ui.MouseEvent {
		m, ok := event.Payload.(ui.Mouse)
		if !ok {
			return false
		}
		p := image.Pt(m.X, m.Y)
		if event.ID == "<MouseLeft>" || event.ID == "MouseLeft" {
			switch {
			case p.In(d.add.Rectangle):
				a.addCalendarEvent()
			case p.In(d.cancel.Rectangle):
				a.calendarDialog = nil
			case p.In(d.eventList.Inner):
				d.focus = 0
				row := d.eventOffset + p.Y - d.eventList.Inner.Min.Y
				if row >= 0 && row < len(d.events) {
					d.event = row
					d.previewScroll = 0
				}
			case p.In(d.preview.Inner):
				d.focus = 2
			case p.In(d.calendarList.Inner):
				d.focus = 1
				row := d.calendarOffset + p.Y - d.calendarList.Inner.Min.Y
				if row >= 0 && row < len(d.calendars) {
					d.destination = row
					d.previewScroll = 0
				}
			}
		} else if strings.Contains(event.ID, "MouseWheel") {
			if p.In(d.eventList.Inner) {
				d.focus = 0
			} else if p.In(d.calendarList.Inner) {
				d.focus = 1
			} else if p.In(d.preview.Inner) {
				d.focus = 2
			} else {
				return false
			}
			delta := 1
			if strings.Contains(event.ID, "Up") {
				delta = -1
			}
			a.moveCalendarChoice(delta)
		}
		return false
	}
	if event.Type != ui.KeyboardEvent {
		return false
	}
	switch {
	case event.ID == "<C-c>" || bindingMatches(event.ID, k.Quit):
		return true
	case event.ID == "<Escape>" || bindingMatches(event.ID, k.Cancel):
		a.calendarDialog = nil
	case bindingMatches(event.ID, k.Send) || event.ID == "<Enter>":
		a.addCalendarEvent()
	case event.ID == "<Tab>":
		d.focus = (d.focus + 1) % 3
	case event.ID == "<Backtab>" || event.ID == "<S-Tab>":
		d.focus = (d.focus + 2) % 3
	case event.ID == "<Left>":
		d.focus = 0
	case event.ID == "<Right>":
		d.focus = 1
	case event.ID == "<Up>" || bindingMatches(event.ID, k.MoveUp):
		a.moveCalendarChoice(-1)
	case event.ID == "<Down>" || bindingMatches(event.ID, k.MoveDown):
		a.moveCalendarChoice(1)
	case bindingMatches(event.ID, k.PageUp):
		a.moveCalendarChoice(-10)
	case bindingMatches(event.ID, k.PageDown):
		a.moveCalendarChoice(10)
	}
	return false
}
func (a *App) moveCalendarChoice(delta int) {
	d := a.calendarDialog
	if d.focus == 2 {
		d.previewScroll = max(0, d.previewScroll+delta)
		return
	}
	d.previewScroll = 0
	if d.focus == 0 {
		d.event = clamp(d.event+delta, 0, len(d.events)-1)
	} else if len(d.calendars) > 0 {
		d.destination = clamp(d.destination+delta, 0, len(d.calendars)-1)
	}
}

func (a *App) renderCalendarImport(w, h int) {
	d := a.calendarDialog
	split := w / 2
	topEnd := max(8, h/3)
	d.eventList.Title = "Calendar events"
	d.calendarList.Title = "Destination — " + a.currentAccount().Name
	d.eventList.SetRect(0, 0, split, topEnd)
	d.calendarList.SetRect(split, 0, w, topEnd)
	d.preview.Title = "Event details"
	d.preview.SetRect(0, topEnd, w, h-6)
	d.add.Title = "Add to calendar"
	d.add.Text = keyLabel(a.bindings().Send) + " / Enter / Click"
	d.add.SetRect(0, h-6, split, h-3)
	d.cancel.Title = "Back to mail"
	d.cancel.Text = "Esc / Click"
	d.cancel.SetRect(split, h-6, w, h-3)
	d.footer.Border = false
	setBarRect(d.footer, 0, h-3, w, h)
	d.footer.Text = safeUI(a.status + "\nTab/Left/Right choose pane  Up/Down choose item  Esc back\nAdds locally; the next sync uploads. This does not accept/decline the invitation.")
	for _, list := range []*widgets.List{d.eventList, d.calendarList} {
		list.TextStyle = ui.NewStyle(a.theme.foreground, a.theme.background)
		list.SelectedStyle = ui.NewStyle(a.theme.selectedFG, a.theme.selectedBG)
	}
	d.eventOffset = keepVisible(d.event, d.eventOffset, max(1, d.eventList.Inner.Dy()), len(d.events))
	d.eventList.Rows = nil
	for i := d.eventOffset; i < min(len(d.events), d.eventOffset+d.eventList.Inner.Dy()); i++ {
		e := d.events[i]
		d.eventList.Rows = append(d.eventList.Rows, safeUI(e.Title+" — "+e.Start))
	}
	d.eventList.SelectedRow = d.event - d.eventOffset
	d.calendarOffset = keepVisible(d.destination, d.calendarOffset, max(1, d.calendarList.Inner.Dy()), len(d.calendars))
	d.calendarList.Rows = nil
	for i := d.calendarOffset; i < min(len(d.calendars), d.calendarOffset+d.calendarList.Inner.Dy()); i++ {
		c := d.calendars[i]
		name := c.Name + " (" + c.Protocol + ")"
		if c.Account == "" {
			name = "Shared: " + name
		}
		d.calendarList.Rows = append(d.calendarList.Rows, safeUI(name))
	}
	d.calendarList.SelectedRow = d.destination - d.calendarOffset
	if len(d.calendars) == 0 {
		d.calendarList.Rows = []string{"(configure a calendar first)"}
		d.calendarList.SelectedRow = 0
		d.calendarList.SelectedStyle = d.calendarList.TextStyle
	}
	details := eventDetails(d.events[d.event])
	if len(d.calendars) > 0 {
		c := d.calendars[d.destination]
		var err error
		if c.Protocol == "jmap-calendars" {
			_, err = d.events[d.event].JSCalendar()
		} else {
			err = d.events[d.event].CanImport()
		}
		if err != nil {
			details += "\nCannot add to this calendar: " + err.Error()
		}
	} else {
		details += "\nConfigure a calendar collection for this account; see docs/contacts-calendar.md."
	}
	lines := wrapLines(safeUI(details), max(10, d.preview.Inner.Dx()))
	visible := max(1, d.preview.Inner.Dy())
	d.previewScroll = clamp(d.previewScroll, 0, max(0, len(lines)-visible))
	d.preview.Text = strings.Join(lines[d.previewScroll:min(len(lines), d.previewScroll+visible)], "\n")
	for _, p := range []*widgets.Paragraph{d.preview, d.add, d.cancel, d.footer} {
		p.TextStyle = ui.NewStyle(a.theme.foreground, a.theme.background)
	}
	for _, block := range []*ui.Block{&d.eventList.Block, &d.calendarList.Block, &d.preview.Block, &d.add.Block, &d.cancel.Block, &d.footer.Block} {
		block.BackgroundColor = a.theme.background
		block.BorderStyle = ui.NewStyle(a.theme.border, a.theme.background)
		block.TitleStyle = ui.NewStyle(a.theme.title, a.theme.background)
	}
	if d.focus == 0 {
		d.eventList.BorderStyle = ui.NewStyle(a.theme.activeBorder, a.theme.background)
	} else if d.focus == 1 {
		d.calendarList.BorderStyle = ui.NewStyle(a.theme.activeBorder, a.theme.background)
	} else {
		d.preview.BorderStyle = ui.NewStyle(a.theme.activeBorder, a.theme.background)
	}
	ui.Render(d.eventList, d.calendarList, d.preview, d.add, d.cancel, d.footer)
}

func (a *App) layoutCalendarButton(x, y, w int) *widgets.Paragraph {
	if a.calendarButton == nil {
		a.calendarButton = widgets.NewParagraph()
	}
	p := a.calendarButton
	p.Title = "Calendar attachment"
	p.Text = keyLabel(a.bindings().ImportCalendar) + " / Click: review events and add to calendar"
	p.BackgroundColor = a.theme.background
	p.TextStyle = ui.NewStyle(a.theme.title, a.theme.background)
	p.BorderStyle = ui.NewStyle(a.theme.activeBorder, a.theme.background)
	p.TitleStyle = p.TextStyle
	p.SetRect(x, y, w, y+3)
	return p
}
