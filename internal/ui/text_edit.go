package uiapp

import (
	"image"
	"strings"

	"github.com/gdamore/tcell/v3"
	rw "github.com/mattn/go-runewidth"
	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

// Selection offsets count runes, including newlines, rather than bytes or
// terminal cells. Both kinds of editor use the same replacement/movement rules.
type textSelection struct {
	anchor       int
	marked       bool
	dragging     bool
	keyboardMark bool
}

func (s *textSelection) bounds(cursor, length int) (int, int) {
	cursor = clamp(cursor, 0, length)
	if !s.marked {
		return cursor, cursor
	}
	anchor := clamp(s.anchor, 0, length)
	return min(anchor, cursor), max(anchor, cursor)
}

func (s *textSelection) reset() { *s = textSelection{} }

func (s *textSelection) move(cursor, target int, extend bool) int {
	s.dragging = false
	if extend {
		if !s.marked {
			s.anchor = cursor
		}
		s.marked = true
	} else {
		s.reset()
	}
	return target
}

func (s *textSelection) edit(text string, cursor int, id string, multiline bool, page int) (string, int, bool) {
	runes := []rune(text)
	cursor = clamp(cursor, 0, len(runes))
	lo, hi := s.bounds(cursor, len(runes))
	if id == "<C-a>" || id == "<M-a>" || id == "<M-A>" {
		s.reset()
		s.anchor, s.marked = 0, true
		return text, len(runes), false
	}
	// Ctrl+Space offers a keyboard mark when a terminal reserves Shift+arrows.
	if id == "<C-Space>" {
		if s.marked {
			s.reset()
		} else {
			s.anchor, s.marked, s.keyboardMark = cursor, true, true
		}
		return text, cursor, false
	}
	extend := strings.Contains(id, "S-")
	nav := strings.ReplaceAll(id, "S-", "")
	target := cursor
	point := textPoint(text, cursor)
	lines := strings.Split(text, "\n")
	switch nav {
	case "<Left>":
		target = max(0, cursor-1)
		if !extend && !s.keyboardMark && lo != hi {
			target = lo
		}
	case "<Right>":
		target = min(len(runes), cursor+1)
		if !extend && !s.keyboardMark && lo != hi {
			target = hi
		}
	case "<Home>":
		target = textOffset(text, image.Pt(0, point.Y))
	case "<End>":
		target = textOffset(text, image.Pt(len([]rune(lines[point.Y])), point.Y))
	case "<C-Home>":
		target = 0
	case "<C-End>":
		target = len(runes)
	case "<Up>", "<Down>", "<PageUp>", "<PageDown>":
		if !multiline {
			return text, cursor, false
		}
		delta := 1
		if strings.Contains(nav, "Page") {
			delta = max(1, page)
		}
		if nav == "<Up>" || nav == "<PageUp>" {
			delta = -delta
		}
		target = textOffset(text, image.Pt(point.X, clamp(point.Y+delta, 0, len(lines)-1)))
	default:
		insert := ""
		switch id {
		case "<Backspace>", "<Backspace2>":
			if lo == hi {
				lo = max(0, cursor-1)
			}
		case "<Delete>":
			if lo == hi {
				hi = min(len(runes), cursor+1)
			}
		case "<Enter>":
			if !multiline {
				return text, cursor, false
			}
			insert = "\n"
		case "<Tab>":
			if !multiline {
				return text, cursor, false
			}
			insert = "    "
		default:
			if r, ok := printableRune(id); ok {
				insert = string(r)
			} else {
				return text, cursor, false
			}
		}
		s.reset()
		result := string(runes[:lo]) + insert + string(runes[hi:])
		return result, lo + len([]rune(insert)), result != text
	}
	// A mark set by Ctrl+Space extends ordinary navigation too.
	return text, s.move(cursor, target, extend || s.keyboardMark), false
}

// textPoint/textOffset translate the flat selection offsets to TextArea's
// logical cursor coordinates. Newline boundaries belong to the preceding line.
func textPoint(text string, offset int) image.Point {
	runes := []rune(text)
	offset = clamp(offset, 0, len(runes))
	p := image.Point{}
	for _, r := range runes[:offset] {
		if r == '\n' {
			p.X = 0
			p.Y++
		} else {
			p.X++
		}
	}
	return p
}

func textOffset(text string, p image.Point) int {
	lines := strings.Split(text, "\n")
	p.Y = clamp(p.Y, 0, len(lines)-1)
	offset := 0
	for _, line := range lines[:p.Y] {
		offset += len([]rune(line)) + 1
	}
	return offset + clamp(p.X, 0, len([]rune(lines[p.Y])))
}

// gotui retains the original tcell event but omits its Shift/Ctrl modifiers
// from navigation IDs. Recover them here, without changing global bindings.
func editorEventID(event ui.Event) string {
	key, ok := event.Payload.(*tcell.EventKey)
	if !ok {
		return event.ID
	}
	if key.Key() == tcell.KeyBacktab {
		return "<Backtab>"
	}
	if key.Key() == tcell.KeyRune && key.Str() == " " && key.Modifiers()&tcell.ModCtrl != 0 {
		return "<C-Space>"
	}
	names := map[tcell.Key]string{tcell.KeyLeft: "Left", tcell.KeyRight: "Right", tcell.KeyUp: "Up", tcell.KeyDown: "Down", tcell.KeyHome: "Home", tcell.KeyEnd: "End", tcell.KeyPgUp: "PageUp", tcell.KeyPgDn: "PageDown"}
	name, ok := names[key.Key()]
	if !ok {
		return event.ID
	}
	if key.Modifiers()&tcell.ModCtrl != 0 {
		name = "C-" + name
	}
	if key.Modifiers()&tcell.ModShift != 0 {
		name = "S-" + name
	}
	return "<" + name + ">"
}

type textInput struct {
	*widgets.Input
	selection      textSelection
	SelectionStyle ui.Style
	start          int // first visible rune; kept aligned to wide character boundaries
}

func newTextInput() *textInput {
	return &textInput{Input: widgets.NewInput(), SelectionStyle: ui.NewStyle(ui.ColorBlack, ui.ColorWhite)}
}

func (in *textInput) Draw(buf *ui.Buffer) {
	in.Block.Draw(buf)
	if in.Inner.Empty() {
		return
	}
	runes := []rune(in.Text)
	if in.EchoMode == widgets.EchoPassword {
		for i := range runes {
			runes[i] = '*'
		}
	}
	in.Cursor = clamp(in.Cursor, 0, len(runes))
	in.start = clamp(in.start, 0, in.Cursor)
	for runeCells(runes[in.start:in.Cursor]) >= in.Inner.Dx() {
		in.start++
	}
	if runeCells(runes) < in.Inner.Dx() {
		in.start = 0
	}
	lo, hi := in.selection.bounds(in.Cursor, len(runes))
	x := in.Inner.Min.X
	for i := in.start; i < len(runes); i++ {
		r := runes[i]
		style := in.TextStyle
		if i >= lo && i < hi {
			style = in.SelectionStyle
		}
		width := runeCellWidth(r)
		if x+width > in.Inner.Max.X {
			break
		}
		drawEditorRune(buf, r, style, image.Pt(x, in.Inner.Min.Y))
		x += width
	}
	if len(runes) == 0 && in.Placeholder != "" {
		buf.SetString(in.Placeholder, ui.NewStyle(ui.ColorGrey), in.Inner.Min)
	}
	// Inactive inputs use TextStyle for CursorStyle. Do not erase a selected
	// character's highlight with that invisible cursor.
	if in.CursorStyle != in.TextStyle {
		p := image.Pt(in.Inner.Min.X+runeCells(runes[in.start:in.Cursor]), in.Inner.Min.Y)
		drawEditorCursor(buf, p, in.Inner, in.CursorStyle)
	}
}

func (in *textInput) mouse(event ui.Event) bool {
	m, ok := event.Payload.(ui.Mouse)
	if !ok {
		return false
	}
	if event.ID == "<MouseRelease>" || event.ID == "MouseRelease" {
		in.selection.dragging = false
		return false
	}
	if event.ID != "<MouseLeft>" && event.ID != "MouseLeft" {
		return false
	}
	p := image.Pt(m.X, m.Y)
	if !in.selection.dragging && !p.In(in.Inner) {
		return false
	}
	runes := []rune(in.Text)
	if in.EchoMode == widgets.EchoPassword {
		for i := range runes {
			runes[i] = '*'
		}
	}
	pos := runeAtCell(runes, in.start, m.X-in.Inner.Min.X)
	if !in.selection.dragging {
		in.selection.anchor = pos
		in.selection.marked = true
		in.selection.dragging = true
	}
	in.Cursor = pos
	return true
}

type textArea struct {
	*widgets.TextArea
	selection      textSelection
	SelectionStyle ui.Style
	top, left      int
	hardWrap       bool // compose only; raw contact/calendar source must stay intact
}

func newTextArea() *textArea {
	return &textArea{TextArea: widgets.NewTextArea(), SelectionStyle: ui.NewStyle(ui.ColorBlack, ui.ColorWhite)}
}

func (ta *textArea) Draw(buf *ui.Buffer) {
	ta.Block.Draw(buf)
	if ta.Inner.Empty() {
		return
	}
	offset := textOffset(ta.Text, ta.Cursor)
	ta.Cursor = textPoint(ta.Text, offset)
	lines := strings.Split(ta.Text, "\n")
	ta.top = clamp(ta.top, max(0, ta.Cursor.Y-ta.Inner.Dy()+1), ta.Cursor.Y)
	cursorX := runeCells([]rune(lines[ta.Cursor.Y])[:ta.Cursor.X])
	ta.left = clamp(ta.left, max(0, cursorX-ta.Inner.Dx()+1), cursorX)
	if runeCells([]rune(lines[ta.Cursor.Y])) < ta.Inner.Dx() {
		ta.left = 0
	}
	lo, hi := ta.selection.bounds(offset, len([]rune(ta.Text)))
	lineOffset := 0
	for y, line := range lines {
		runes := []rune(line)
		if y >= ta.top && y < ta.top+ta.Inner.Dy() {
			x := 0
			for i, r := range runes {
				width := runeCellWidth(r)
				style := ta.TextStyle
				if lineOffset+i >= lo && lineOffset+i < hi {
					style = ta.SelectionStyle
				}
				if x >= ta.left && x+width <= ta.left+ta.Inner.Dx() {
					drawEditorRune(buf, r, style, image.Pt(ta.Inner.Min.X+x-ta.left, ta.Inner.Min.Y+y-ta.top))
				}
				x += width
			}
			// Make selected newline characters visible, including blank lines.
			if y < len(lines)-1 && lineOffset+len(runes) >= lo && lineOffset+len(runes) < hi {
				p := image.Pt(ta.Inner.Min.X+x-ta.left, ta.Inner.Min.Y+y-ta.top)
				if p.In(ta.Inner) {
					buf.SetCell(ui.NewCell(' ', ta.SelectionStyle), p)
				}
			}
		}
		lineOffset += len(runes) + 1
	}
	if ta.ShowCursor {
		drawEditorCursor(buf, image.Pt(ta.Inner.Min.X+cursorX-ta.left, ta.Inner.Min.Y+ta.Cursor.Y-ta.top), ta.Inner, ta.CursorStyle)
	}
}

func (ta *textArea) mouse(event ui.Event) bool {
	m, ok := event.Payload.(ui.Mouse)
	if !ok {
		return false
	}
	if event.ID == "<MouseRelease>" || event.ID == "MouseRelease" {
		ta.selection.dragging = false
		return false
	}
	if event.ID != "<MouseLeft>" && event.ID != "MouseLeft" {
		return false
	}
	if !ta.selection.dragging && !image.Pt(m.X, m.Y).In(ta.Inner) {
		return false
	}
	lines := strings.Split(ta.Text, "\n")
	y := clamp(m.Y-ta.Inner.Min.Y+ta.top, 0, len(lines)-1)
	x := runeAtCell([]rune(lines[y]), 0, m.X-ta.Inner.Min.X+ta.left)
	pos := textOffset(ta.Text, image.Pt(x, y))
	if !ta.selection.dragging {
		ta.selection.anchor = pos
		ta.selection.marked = true
		ta.selection.dragging = true
	}
	ta.Cursor = textPoint(ta.Text, pos)
	return true
}

func runeCellWidth(r rune) int { return max(1, rw.RuneWidth(r)) }
func runeCells(runes []rune) int {
	n := 0
	for _, r := range runes {
		n += runeCellWidth(r)
	}
	return n
}
func runeAtCell(runes []rune, start, cell int) int {
	start = clamp(start, 0, len(runes))
	for i := start; i < len(runes); i++ {
		if cell < runeCellWidth(runes[i]) {
			return i
		}
		cell -= runeCellWidth(runes[i])
	}
	return len(runes)
}
func drawEditorRune(buf *ui.Buffer, r rune, style ui.Style, p image.Point) {
	buf.SetCell(ui.NewCell(r, style), p)
	for i := 1; i < runeCellWidth(r); i++ {
		buf.SetCell(ui.NewCell(0, style), p.Add(image.Pt(i, 0)))
	}
}
func drawEditorCursor(buf *ui.Buffer, p image.Point, rect image.Rectangle, style ui.Style) {
	if !p.In(rect) {
		return
	}
	cell := buf.GetCell(p)
	if cell.Rune == 0 {
		cell.Rune = ' '
	}
	cell.Style = style
	buf.SetCell(cell, p)
}
