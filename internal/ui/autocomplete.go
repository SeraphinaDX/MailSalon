package uiapp

import (
	"fmt"
	"image"
	"net/mail"
	"strings"
	"unicode"

	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

type recipientCompletion struct {
	addresses, matches           []mail.Address
	selected, start, end, offset int
	key, dismissed               string
	list                         *widgets.List
}

func (a *App) loadComposeContacts() {
	c := a.compose
	if c == nil {
		return
	}
	addresses, err := a.loadContactAddresses(c.account)
	c.completion = recipientCompletion{addresses: addresses}
	if err != nil {
		a.status += " — Some contacts unavailable: " + err.Error()
	}
}

// Find the recipient around the cursor, honoring commas inside quoted names
// and angle brackets. Offsets match textInput's rune-based cursor/selection.
func recipientSpan(text string, cursor int) (int, int, string) {
	runes := []rune(text)
	cursor = clamp(cursor, 0, len(runes))
	start, end := 0, len(runes)
	quoted, escaped, angle := false, false, false
	for i, r := range runes {
		if escaped {
			escaped = false
			continue
		}
		if quoted && r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		if r == '<' {
			angle = true
		}
		if r == '>' {
			angle = false
		}
		if r == ',' && !angle {
			if i < cursor {
				start = i + 1
			} else {
				end = i
				break
			}
		}
	}
	for start < end && unicode.IsSpace(runes[start]) {
		start++
	}
	for end > start && unicode.IsSpace(runes[end-1]) {
		end--
	}
	return start, end, strings.TrimSpace(string(runes[start:clamp(cursor, start, end)]))
}

func (a *App) refreshCompletion() {
	c := a.compose
	if c == nil {
		return
	}
	s := &c.completion
	in := a.activeInput()
	if c.field < composeTo || c.field > composeBcc || in == nil || c.attachPrompt.TitleBottom == "active" {
		s.matches = nil
		s.key = ""
		return
	}
	key := fmt.Sprintf("%d:%d:%s", c.field, in.Cursor, in.Text)
	if key != s.key {
		s.selected = 0
		s.offset = 0
		s.key = key
	}
	s.matches = nil
	if key == s.dismissed {
		return
	}
	lo, hi := in.selection.bounds(in.Cursor, len([]rune(in.Text)))
	if lo != hi {
		return
	}
	start, end, query := recipientSpan(in.Text, in.Cursor)
	if query == "" {
		return
	}
	// A completed address should let the next Tab move on to the next field.
	if parsed, err := mail.ParseAddress(string([]rune(in.Text)[start:end])); err == nil {
		for _, addr := range s.addresses {
			if strings.EqualFold(parsed.Address, addr.Address) {
				return
			}
		}
	}
	q := strings.ToLower(query)
	for _, addr := range s.addresses {
		if strings.Contains(strings.ToLower(addr.Name), q) || strings.Contains(strings.ToLower(addr.Address), q) || strings.Contains(strings.ToLower(addr.String()), q) {
			s.matches = append(s.matches, addr)
		}
	}
	s.start, s.end = start, end
	s.selected = clamp(s.selected, 0, max(0, len(s.matches)-1))
}

func (a *App) acceptCompletion() {
	c := a.compose
	if c == nil || len(c.completion.matches) == 0 {
		return
	}
	s := &c.completion
	in := a.activeInput()
	if in == nil {
		return
	}
	runes := []rune(in.Text)
	address := s.matches[s.selected].String()
	in.Text = string(runes[:s.start]) + address + string(runes[s.end:])
	in.Cursor = s.start + len([]rune(address))
	in.selection.reset()
	s.matches = nil
	s.key = ""
}

func (a *App) handleCompletionKey(id string) bool {
	a.refreshCompletion()
	s := &a.compose.completion
	if len(s.matches) == 0 {
		return false
	}
	switch id {
	case "<Down>":
		s.selected = (s.selected + 1) % len(s.matches)
	case "<Up>":
		s.selected = (s.selected + len(s.matches) - 1) % len(s.matches)
	case "<Enter>", "<Tab>":
		a.acceptCompletion()
	default:
		return false
	}
	return true
}

func (a *App) dismissCompletion() bool {
	a.refreshCompletion()
	s := &a.compose.completion
	if len(s.matches) == 0 {
		return false
	}
	s.dismissed = s.key
	s.matches = nil
	return true
}

func (a *App) completionPopup(w, h int) ui.Drawable {
	a.refreshCompletion()
	s := &a.compose.completion
	if len(s.matches) == 0 {
		s.list = nil
		return nil
	}
	rows := min(5, len(s.matches))
	s.offset = keepVisible(s.selected, s.offset, rows, len(s.matches))
	list := widgets.NewList()
	list.Title = "Contacts — Up/Down, Enter/Tab"
	list.TitleBottom = "Click to choose; Esc closes suggestions"
	list.BackgroundColor = a.theme.background
	list.TextStyle = ui.NewStyle(a.theme.foreground, a.theme.background)
	list.SelectedStyle = ui.NewStyle(a.theme.selectedFG, a.theme.selectedBG)
	list.BorderStyle = ui.NewStyle(a.theme.activeBorder, a.theme.background)
	list.TitleStyle = ui.NewStyle(a.theme.title, a.theme.background)
	list.TitleBottomStyle = ui.NewStyle(a.theme.muted, a.theme.background)
	for _, addr := range s.matches[s.offset:min(len(s.matches), s.offset+rows)] {
		list.Rows = append(list.Rows, safeUI(addr.Name+" <"+addr.Address+">"))
	}
	list.SelectedRow = s.selected - s.offset
	y := min(a.activeInput().Max.Y-1, h-4-rows-2)
	list.SetRect(1, max(0, y), w-1, max(0, y)+rows+2)
	s.list = list
	return list
}

func (a *App) handleCompletionMouse(event ui.Event) bool {
	s := &a.compose.completion
	if len(s.matches) == 0 || s.list == nil {
		return false
	}
	m, ok := event.Payload.(ui.Mouse)
	if !ok || !image.Pt(m.X, m.Y).In(s.list.Rectangle) {
		return false
	}
	switch event.ID {
	case "<MouseLeft>", "MouseLeft":
		if image.Pt(m.X, m.Y).In(s.list.Inner) {
			row := s.offset + m.Y - s.list.Inner.Min.Y
			if row < len(s.matches) {
				s.selected = row
				a.acceptCompletion()
			}
		}
	case "<MouseWheelUp>", "MouseWheelUp":
		s.selected = max(0, s.selected-1)
	case "<MouseWheelDown>", "MouseWheelDown":
		s.selected = min(len(s.matches)-1, s.selected+1)
	}
	return true
}
