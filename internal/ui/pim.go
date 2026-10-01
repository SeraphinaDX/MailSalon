package uiapp

import (
	"fmt"
	"image"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/pim"
	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

type pimEditor struct {
	original *pim.Item
	fields   []*textInput
	field    int
	raw      *textArea
}

func (a *App) setView(view int) {
	if view == 2 {
		a.ensureCalendarViews()
	}
	a.view = view
	a.deleteArmed = false
	a.searchActive = false
	a.previewScroll = 0
	a.pimQuery = ""
	if view == 0 {
		a.status = "Mail"
		return
	}
	a.pimCollections = nil
	a.pimCollection = 0
	for _, c := range a.cfg.Collections {
		if c.Account != "" && !strings.EqualFold(c.Account, a.currentAccount().Name) {
			continue
		}
		if (view == 1) == pim.Contacts(c) {
			a.pimCollections = append(a.pimCollections, c)
		}
	}
	if err := a.loadPIM(); err != nil {
		a.setError(err)
	} else {
		a.status = "Contacts/calendar loaded"
	}
	if len(a.pimCollections) == 0 {
		a.status = "No collections configured for this view; see docs/contacts-calendar.md"
	}
	a.focus = focusMessages
}

func (a *App) loadPIM() error {
	a.pimItems = nil
	a.pimAllItems = nil
	a.deleteArmed = false
	if a.calendarViews != nil {
		a.calendarViews.dirty = true
	}
	if len(a.pimCollections) == 0 {
		return nil
	}
	items, err := pim.Load(a.pimCollections[a.pimCollection])
	if err != nil {
		return err
	}
	a.pimAllItems = items
	a.filterPIM()
	return nil
}
func (a *App) filterPIM() {
	if a.calendarViews != nil {
		a.calendarViews.dirty = true
	}
	a.pimItems = nil
	a.pimSelected = 0
	a.pimOffset = 0
	a.previewScroll = 0
	query := strings.ToLower(a.pimQuery)
	for _, item := range a.pimAllItems {
		if query == "" || strings.Contains(strings.ToLower(strings.Join([]string{item.Title, item.Email, item.Phone, item.Start, item.Location, item.Notes}, " ")), query) {
			a.pimItems = append(a.pimItems, item)
		}
	}
}

func (a *App) handlePIMKey(id string) bool {
	k := a.bindings()
	if id == "<C-c>" || bindingMatches(id, k.Quit) {
		return true
	}
	if !bindingMatches(id, k.Delete) {
		a.deleteArmed = false
	}
	if a.view == 2 && a.handleCalendarViewKey(id) {
		return false
	}
	switch {
	case bindingMatches(id, k.FocusNext):
		a.focus = (a.focus + 1) % 3
	case id == "<Left>" || bindingMatches(id, k.FocusLeft):
		a.focus = focusFolders
	case id == "<Right>" || bindingMatches(id, k.FocusRight):
		if a.focus < focusPreview {
			a.focus++
		}
	case id == "<Up>" || bindingMatches(id, k.MoveUp):
		a.movePIM(-1)
	case id == "<Down>" || bindingMatches(id, k.MoveDown):
		a.movePIM(1)
	case bindingMatches(id, k.PageUp):
		a.movePIM(-max(1, a.messageTbl.Inner.Dy()-2))
	case bindingMatches(id, k.PageDown):
		a.movePIM(max(1, a.messageTbl.Inner.Dy()-2))
	case bindingMatches(id, k.Home):
		a.movePIM(-1 << 20)
	case bindingMatches(id, k.End):
		a.movePIM(1 << 20)
	case bindingMatches(id, k.Open):
		if a.focus == focusFolders {
			a.focus = focusMessages
		} else {
			a.focus = focusPreview
		}
	case bindingMatches(id, k.SwitchAccount):
		a.switchAccount(1)
	case bindingMatches(id, k.Sync):
		a.runSync()
	case bindingMatches(id, k.Refresh):
		if err := a.loadPIM(); err != nil {
			a.setError(err)
		} else {
			a.status = "Collections refreshed"
		}
	case bindingMatches(id, k.Search):
		a.startSearch()
	case id == "n":
		a.startPIMEditor(false)
	case bindingMatches(id, k.Archive):
		a.startPIMEditor(true)
	case bindingMatches(id, k.Compose):
		a.composeContact()
	case bindingMatches(id, k.Delete):
		if a.view == 2 {
			a.selectCalendarSource()
		}
		if len(a.pimItems) == 0 || a.pimSelected < 0 {
			a.status = "No item selected"
			break
		}
		if !a.deleteArmed {
			a.deleteArmed = true
			a.status = "Press " + keyLabel(k.Delete) + " again to delete locally (sync controls remote deletion)"
			if a.view == 2 && (a.pimItems[a.pimSelected].Recurring || a.calendarViews.rows[a.calendarViews.selected].Recurring) {
				a.status = "Press " + keyLabel(k.Delete) + " again to delete the entire recurring series locally"
			}
			break
		}
		if err := pim.Delete(a.pimCollections[a.pimCollection], a.pimItems[a.pimSelected]); err != nil {
			a.setError(err)
		} else {
			a.status = "Deleted locally; backup retained in .mss-trash"
			if err := a.loadPIM(); err != nil {
				a.setError(err)
			}
		}
		a.deleteArmed = false
	}
	return false
}

func (a *App) movePIM(delta int) {
	switch a.focus {
	case focusFolders:
		if len(a.pimCollections) == 0 {
			return
		}
		next := clamp(a.pimCollection+delta, 0, len(a.pimCollections)-1)
		if next != a.pimCollection {
			a.pimCollection = next
			a.pimQuery = ""
			if err := a.loadPIM(); err != nil {
				a.setError(err)
			}
		}
	case focusMessages:
		if len(a.pimItems) > 0 {
			a.pimSelected = clamp(a.pimSelected+delta, 0, len(a.pimItems)-1)
			a.previewScroll = 0
		}
	case focusPreview:
		a.previewScroll = max(0, a.previewScroll+delta)
	}
}

func (a *App) composeContact() {
	if a.view != 1 || len(a.pimItems) == 0 {
		a.status = "Select a contact first"
		return
	}
	item := a.pimItems[a.pimSelected]
	addr, err := mail.ParseAddress(item.Email)
	if err != nil {
		a.status = "Selected contact has no valid email address"
		return
	}
	addr.Name = item.Title
	a.startCompose(nil, false)
	a.compose.to.Text = addr.String()
	a.compose.to.Cursor = utf8.RuneCountInString(a.compose.to.Text)
	a.compose.field = composeSubject
}

func (a *App) renderPIM(w, h int) {
	if a.pimEditor != nil {
		a.renderPIMEditor(w, h)
		return
	}
	if a.view == 2 {
		a.renderCalendarViews(w, h)
		return
	}
	left := clamp(w/5, 18, 30)
	footer := h - 3
	split := max(8, h*45/100)
	a.accountBar.SetRect(0, 1, left, 4)
	a.updateBar.SetRect(0, 4, left, 7)
	a.folderList.SetRect(0, 7, left, footer)
	a.messageTbl.SetRect(left, 1, w, split)
	a.preview.SetRect(left, split, w, footer)
	setBarRect(a.footer, 0, footer, w, h)
	a.populateAccountBar()
	a.populateUpdateBar()
	a.updateBar.Title = "Sync collections"
	a.folderList.Title = "Address books"
	a.messageTbl.Title = "Contacts — " + a.currentAccount().Name
	a.preview.Title = "Contact"
	a.preview.TitleBottom = "Scroll / wheel"

	visibleCollections := max(1, a.folderList.Inner.Dy())
	a.folderOffset = keepVisible(a.pimCollection, a.folderOffset, visibleCollections, len(a.pimCollections))
	a.folderList.Rows = nil
	for i := a.folderOffset; i < min(len(a.pimCollections), a.folderOffset+visibleCollections); i++ {
		name := a.pimCollections[i].Name
		if a.pimCollections[i].Account == "" {
			name = "Shared: " + name
		}
		a.folderList.Rows = append(a.folderList.Rows, safeUI(name))
	}
	a.folderList.SelectedRow = a.pimCollection - a.folderOffset
	if len(a.pimCollections) == 0 {
		a.folderList.Rows = []string{"(no collections)"}
		// gotui List treats selection as its scroll anchor, so -1 is unsafe
		// even when a placeholder is the only row.
		a.folderList.SelectedRow = 0
	}
	visible := max(1, a.messageTbl.Inner.Dy()-1)
	a.pimOffset = keepVisible(a.pimSelected, a.pimOffset, visible, len(a.pimItems))
	rows := [][]string{{"Name", "Email", "Phone"}}
	for i := a.pimOffset; i < min(len(a.pimItems), a.pimOffset+visible); i++ {
		item := a.pimItems[i]
		rows = append(rows, []string{safeUI(item.Title), safeUI(item.Email), safeUI(item.Phone)})
	}
	a.messageTbl.Rows = rows
	a.messageTbl.ColumnWidths = []int{max(12, (w-left)/3), max(12, (w-left)/3), max(8, (w-left)/3-4)}
	a.messageTbl.SelectedRow = a.pimSelected - a.pimOffset + 1
	a.preview.Text = "No items yet. Sync first, or press n to create one."
	if len(a.pimCollections) == 0 {
		a.preview.Text = "No collections configured for this view.\nAdd an address book or calendar in config.toml; see docs/contacts-calendar.md."
	}
	if len(a.pimItems) > 0 {
		i := a.pimItems[a.pimSelected]
		text := fmt.Sprintf("%s\n", i.Title)
		text += fmt.Sprintf("Email: %s\nPhone: %s\n", i.Email, i.Phone)
		text += i.Notes + "\n\n" + string(i.Data)
		lines := strings.Split(safeUI(text), "\n")
		a.previewScroll = clamp(a.previewScroll, 0, max(0, len(lines)-1))
		a.preview.Text = strings.Join(lines[a.previewScroll:], "\n")
	}
	k := a.bindings()
	a.footer.Text = safeUI(fmt.Sprintf(" [%s] %s — %s\n %s Mail  %s Contacts  %s Calendar  n New  %s Edit source  %s Compose to contact\n %s Search  %s Delete  %s Sync  %s Refresh  Tab Focus  %s Account  %s Quit", a.currentAccount().Name, a.pimScope(), a.status, keyLabel(k.MailView), keyLabel(k.ContactsView), keyLabel(k.CalendarView), keyLabel(k.Archive), keyLabel(k.Compose), keyLabel(k.Search), keyLabel(k.Delete), keyLabel(k.Sync), keyLabel(k.Refresh), keyLabel(k.SwitchAccount), keyLabel(k.Quit)))
	a.updateFocusStyles()
	items := append(a.layoutViewTabs(w), a.accountBar, a.updateBar, a.folderList, a.messageTbl, a.preview, a.footer)
	if a.searchActive {
		a.searchPrompt.SetRect(left+2, max(1, h/2-2), w-2, max(1, h/2-2)+3)
		items = append(items, a.searchPrompt)
	}
	ui.Render(items...)
}

func (a *App) handlePIMMouse(e ui.Event) {
	if a.view == 2 {
		a.handleCalendarViewMouse(e)
		return
	}
	if a.handleViewTabMouse(e) {
		return
	}
	m, ok := e.Payload.(ui.Mouse)
	if !ok {
		return
	}
	p := image.Pt(m.X, m.Y)
	if e.ID == "<MouseLeft>" || e.ID == "MouseLeft" {
		switch {
		case p.In(a.accountBar.Rectangle):
			a.switchAccount(1)
		case p.In(a.updateBar.Rectangle):
			a.runSync()
		case p.In(a.folderList.Inner):
			a.focus = focusFolders
			row := a.folderOffset + p.Y - a.folderList.Inner.Min.Y
			if row < len(a.pimCollections) {
				a.pimCollection = row
				a.pimQuery = ""
				if err := a.loadPIM(); err != nil {
					a.setError(err)
				}
			}
		case p.In(a.messageTbl.Inner):
			a.focus = focusMessages
			row := a.pimOffset + p.Y - a.messageTbl.Inner.Min.Y - 1
			if row >= 0 && row < len(a.pimItems) {
				a.pimSelected = row
				a.previewScroll = 0
			}
		case p.In(a.preview.Inner):
			a.focus = focusPreview
		}
	} else if e.ID == "<MouseWheelUp>" || e.ID == "MouseWheelUp" || e.ID == "<MouseWheelDown>" || e.ID == "MouseWheelDown" {
		if p.In(a.folderList.Inner) {
			a.focus = focusFolders
		} else if p.In(a.messageTbl.Inner) {
			a.focus = focusMessages
		} else if p.In(a.preview.Inner) {
			a.focus = focusPreview
		} else {
			return
		}
		delta := 3
		if strings.Contains(e.ID, "Up") {
			delta = -3
		}
		a.movePIM(delta)
	}
}

func (a *App) startPIMEditor(edit bool) {
	if len(a.pimCollections) == 0 {
		a.status = "Configure a collection first"
		return
	}
	e := &pimEditor{}
	if edit {
		if a.view == 2 {
			a.selectCalendarSource()
		}
		if len(a.pimItems) == 0 || a.pimSelected < 0 {
			a.status = "Select an item first"
			return
		}
		item := a.pimItems[a.pimSelected]
		e.original = &item
		e.raw = newTextArea()
		e.raw.Text = strings.ReplaceAll(string(item.Data), "\r\n", "\n")
		e.raw.Title = "Edit source (all properties retained)"
		e.raw.ShowCursor = true
	} else {
		labels := []string{"Name", "Email", "Phone"}
		values := []string{"", "", ""}
		if a.view == 2 {
			labels = []string{"Title", "Start: YYYY-MM-DDTHH:MM:SS (or YYYY-MM-DD)", "End: same format (all-day end is exclusive)", "Time zone: IANA name, or empty for floating", "Location"}
			now := time.Now().Truncate(time.Hour)
			if a.calendarViews != nil {
				day := a.calendarViews.date
				now = time.Date(day.Year(), day.Month(), day.Day(), now.Hour(), 0, 0, 0, time.Local)
			}
			values = []string{"", now.Format("2006-01-02T15:04:05"), now.Add(time.Hour).Format("2006-01-02T15:04:05"), "", ""}
		}
		for i, label := range labels {
			f := newTextInput()
			f.Title = label
			f.Text = values[i]
			f.Cursor = utf8.RuneCountInString(f.Text)
			e.fields = append(e.fields, f)
		}
	}
	a.pimEditor = e
	a.status = "Ctrl+S saves locally; sync uploads changes"
}

func (a *App) handlePIMEditor(event ui.Event) bool {
	e := a.pimEditor
	k := a.bindings()
	if event.Type == ui.MouseEvent {
		if e.raw != nil {
			e.raw.mouse(event)
			return false
		}
		for i, f := range e.fields {
			if f.selection.dragging && f.mouse(event) {
				e.field = i
				return false
			}
		}
		for i, f := range e.fields {
			if f.mouse(event) {
				e.field = i
				return false
			}
		}
		return false
	}
	if event.Type != ui.KeyboardEvent {
		return false
	}
	id := editorEventID(event)
	if id == "<C-c>" {
		return true
	}
	if bindingMatches(id, k.Cancel) {
		a.pimEditor = nil
		a.status = "Edit cancelled"
		return false
	}
	if bindingMatches(id, k.Send) {
		c := a.pimCollections[a.pimCollection]
		var data []byte
		uid := ""
		var err error
		if e.raw != nil {
			data = []byte(e.raw.Text)
			if pim.Extension(c) != ".json" {
				text := strings.TrimRight(strings.ReplaceAll(e.raw.Text, "\r\n", "\n"), "\n")
				data = []byte(strings.ReplaceAll(text, "\n", "\r\n") + "\r\n")
			}
		} else {
			values := []string{}
			for _, f := range e.fields {
				values = append(values, f.Text)
			}
			data, uid, err = pim.New(c, values)
		}
		if err == nil {
			err = pim.Save(c, e.original, data, uid)
		}
		if err != nil {
			a.setError(err)
			return false
		}
		a.pimEditor = nil
		a.status = "Saved locally; sync to upload"
		if err := a.loadPIM(); err != nil {
			a.setError(err)
		}
		return false
	}
	if e.raw != nil {
		a.editTextArea(e.raw, id)
		return false
	}
	if bindingMatches(id, k.NextField) || id == "<Enter>" {
		e.field = (e.field + 1) % len(e.fields)
	} else if bindingMatches(id, k.PreviousField) || id == "<S-Tab>" {
		e.field = (e.field + len(e.fields) - 1) % len(e.fields)
	} else {
		a.editInput(e.fields[e.field], id)
	}
	return false
}

func (a *App) renderPIMEditor(w, h int) {
	e := a.pimEditor
	items := []ui.Drawable{}
	style := func(b *ui.Block) {
		b.BackgroundColor = a.theme.background
		b.BorderStyle = ui.NewStyle(a.theme.border, a.theme.background)
		b.TitleStyle = ui.NewStyle(a.theme.title, a.theme.background)
	}
	if e.raw != nil {
		style(&e.raw.Block)
		e.raw.TextStyle = ui.NewStyle(a.theme.foreground, a.theme.background)
		e.raw.CursorStyle = ui.NewStyle(a.theme.cursorFG, a.theme.cursorBG)
		e.raw.SelectionStyle = ui.NewStyle(a.theme.selectedFG, a.theme.selectedBG)
		e.raw.SetRect(0, 0, w, h-3)
		items = append(items, e.raw)
	} else {
		bg := widgets.NewParagraph()
		style(&bg.Block)
		bg.Title = "New " + a.pimCollections[a.pimCollection].Name
		bg.Text = ""
		bg.SetRect(0, 0, w, h-3)
		items = append(items, bg)
		for i, f := range e.fields {
			style(&f.Block)
			f.TextStyle = ui.NewStyle(a.theme.foreground, a.theme.background)
			f.CursorStyle = ui.NewStyle(a.theme.cursorFG, a.theme.cursorBG)
			f.SelectionStyle = ui.NewStyle(a.theme.selectedFG, a.theme.selectedBG)
			if i != e.field {
				f.CursorStyle = f.TextStyle
			}
			if i == e.field {
				f.BorderStyle = ui.NewStyle(a.theme.activeBorder, a.theme.background)
			}
			f.SetRect(2, 1+3*i, w-2, 4+3*i)
			items = append(items, f)
		}
	}
	setBarRect(a.footer, 0, h-3, w, h)
	a.footer.Text = safeUI(a.status + "\n" + keyLabel(a.bindings().Send) + " Save locally  " + keyLabel(a.bindings().Cancel) + " Cancel  Tab/Shift+Tab Fields\nShift+arrows/drag Select  Alt+A All  Backspace/Delete Edit")
	items = append(items, a.footer)
	ui.Render(items...)
}
