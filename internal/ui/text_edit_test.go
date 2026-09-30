package uiapp

import (
	"image"
	"path/filepath"
	"strings"
	"testing"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
	"github.com/gdamore/tcell/v3"
	ui "github.com/metaspartan/gotui/v5"
)

func keyboard(id string) ui.Event { return ui.Event{Type: ui.KeyboardEvent, ID: id} }
func mouse(id string, x, y int) ui.Event {
	return ui.Event{Type: ui.MouseEvent, ID: id, Payload: ui.Mouse{X: x, Y: y}}
}

func TestInputSelectionReplacesUnicodeAndCollapses(t *testing.T) {
	a := &App{}
	in := newTextInput()
	in.Text, in.Cursor = "a界éz", 4
	a.editInput(in, "<S-Left>")
	a.editInput(in, "<S-Left>")
	if lo, hi := in.selection.bounds(in.Cursor, 4); lo != 2 || hi != 4 {
		t.Fatalf("range=%d:%d", lo, hi)
	}
	a.editInput(in, "<S-Right>")
	a.editInput(in, "Ω")
	if in.Text != "a界éΩ" || in.Cursor != 4 {
		t.Fatalf("replacement: %q cursor %d", in.Text, in.Cursor)
	}
	a.editInput(in, "<C-a>")
	a.editInput(in, "<Left>")
	if in.Cursor != 0 || in.selection.marked {
		t.Fatal("left did not collapse selection")
	}
	a.editInput(in, "<C-a>")
	a.editInput(in, "<Right>")
	if in.Cursor != 4 || in.selection.marked {
		t.Fatal("right did not collapse selection")
	}
	for _, key := range []string{"<Backspace>", "<Delete>"} {
		in.Text, in.Cursor = "hello", 5
		a.editInput(in, "<C-a>")
		a.editInput(in, key)
		if in.Text != "" || in.Cursor != 0 {
			t.Fatalf("%s: %q", key, in.Text)
		}
	}
}

func TestMultilineSelectionAndDeleteDirections(t *testing.T) {
	a := &App{}
	ta := newTextArea()
	ta.Text, ta.Cursor = "ab\n界é\nend", image.Pt(1, 1)
	a.editTextArea(ta, "<S-Up>")
	a.editTextArea(ta, "X")
	if ta.Text != "aXé\nend" || ta.Cursor != image.Pt(2, 0) {
		t.Fatalf("cross-line replacement: %q %v", ta.Text, ta.Cursor)
	}
	ta.Text, ta.Cursor = "ab\ncd", image.Pt(2, 0)
	a.editTextArea(ta, "<Delete>")
	if ta.Text != "abcd" || ta.Cursor != image.Pt(2, 0) {
		t.Fatalf("Delete: %q %v", ta.Text, ta.Cursor)
	}
	ta.Text, ta.Cursor = "ab\ncd", image.Pt(0, 1)
	a.editTextArea(ta, "<Backspace>")
	if ta.Text != "abcd" || ta.Cursor != image.Pt(2, 0) {
		t.Fatalf("Backspace: %q %v", ta.Text, ta.Cursor)
	}
	for _, tt := range []struct {
		key, want string
		cursor    image.Point
	}{
		{"<Enter>", "\n", image.Pt(0, 1)}, {"<Tab>", "    ", image.Pt(4, 0)}, {"<Delete>", "", image.Point{}},
	} {
		ta.Text, ta.Cursor = "ab\ncd", image.Point{}
		a.editTextArea(ta, "<C-a>")
		a.editTextArea(ta, tt.key)
		if ta.Text != tt.want || ta.Cursor != tt.cursor {
			t.Fatalf("%s: %q %v", tt.key, ta.Text, ta.Cursor)
		}
	}
}

func TestSelectionHomeEndAndKeyboardMark(t *testing.T) {
	a := &App{}
	ta := newTextArea()
	ta.Text, ta.Cursor = "one\ntwo\nthree", image.Pt(1, 1)
	a.editTextArea(ta, "<S-Home>")
	if lo, hi := ta.selection.bounds(textOffset(ta.Text, ta.Cursor), 13); lo != 4 || hi != 5 {
		t.Fatalf("home range=%d:%d", lo, hi)
	}
	a.editTextArea(ta, "<End>")
	if ta.Cursor != image.Pt(3, 1) || ta.selection.marked {
		t.Fatal("End didn't clear selection")
	}
	a.editTextArea(ta, "<S-C-End>")
	a.editTextArea(ta, "<Backspace>")
	if ta.Text != "one\ntwo" {
		t.Fatalf("document end selection: %q", ta.Text)
	}
	in := newTextInput()
	in.Text, in.Cursor = "abc", 0
	a.editInput(in, "<C-Space>")
	a.editInput(in, "<Right>")
	a.editInput(in, "<Right>")
	a.editInput(in, "Z")
	if in.Text != "Zc" {
		t.Fatalf("mark replacement=%q", in.Text)
	}
}

func TestEditorRecoversTcellModifiers(t *testing.T) {
	for _, tt := range []struct {
		key  tcell.Key
		str  string
		mod  tcell.ModMask
		want string
	}{
		{tcell.KeyLeft, "", tcell.ModShift, "<S-Left>"},
		{tcell.KeyHome, "", tcell.ModShift | tcell.ModCtrl, "<S-C-Home>"},
		{tcell.KeyTab, "", tcell.ModShift, "<Backtab>"},
		{tcell.KeyRune, " ", tcell.ModCtrl, "<C-Space>"},
	} {
		e := ui.Event{Type: ui.KeyboardEvent, ID: "lost modifier", Payload: tcell.NewEventKey(tt.key, tt.str, tt.mod)}
		if got := editorEventID(e); got != tt.want {
			t.Fatalf("got %q, want %q", got, tt.want)
		}
	}
}

func TestSelectionRenderingAndMouseUseRunePositions(t *testing.T) {
	in := newTextInput()
	in.Text, in.Cursor = "a界éz", 0
	in.TextStyle = ui.NewStyle(ui.ColorWhite, ui.ColorBlack)
	in.CursorStyle = in.TextStyle // no cursor over an inactive field's selection
	in.SelectionStyle = ui.NewStyle(ui.ColorBlack, ui.ColorYellow)
	in.SetRect(0, 0, 12, 3)
	in.mouse(mouse("<MouseLeft>", 2, 1)) // leading edge of the wide character
	in.mouse(mouse("<MouseLeft>", 5, 1)) // after é
	in.mouse(mouse("<MouseRelease>", 5, 1))
	buf := ui.NewBuffer(in.GetRect())
	in.Draw(buf)
	for _, x := range []int{2, 3, 4} {
		if got := buf.GetCell(image.Pt(x, 1)).Style; got != in.SelectionStyle {
			t.Fatalf("cell %d not highlighted: %v", x, got)
		}
	}
	if in.selection.dragging {
		t.Fatal("drag not released")
	}
	(&App{}).editInput(in, "X")
	if in.Text != "aXz" {
		t.Fatalf("mouse replacement=%q", in.Text)
	}

	ta := newTextArea()
	ta.Text = "a\n\nb"
	ta.SelectionStyle = in.SelectionStyle
	ta.ShowCursor = false
	ta.SetRect(0, 0, 8, 6)
	(&App{}).editTextArea(ta, "<C-a>")
	buf = ui.NewBuffer(ta.GetRect())
	ta.Draw(buf)
	for _, p := range []image.Point{image.Pt(1, 1), image.Pt(2, 1), image.Pt(1, 2), image.Pt(1, 3)} {
		if buf.GetCell(p).Style != ta.SelectionStyle {
			t.Fatalf("multiline cell %v not highlighted", p)
		}
	}
}

func TestMousePositionsInScrolledEditors(t *testing.T) {
	in := newTextInput()
	in.Text, in.Cursor = "0123456789", 10
	in.SetRect(0, 0, 6, 3)
	in.Draw(ui.NewBuffer(in.GetRect()))
	in.mouse(mouse("<MouseLeft>", 1, 1))
	if in.Cursor != 7 {
		t.Fatalf("scrolled input click=%d", in.Cursor)
	}
	in.mouse(mouse("<MouseRelease>", 1, 1))

	ta := newTextArea()
	ta.Text, ta.Cursor = "first\nsecond\n0123456789", image.Pt(10, 2)
	ta.SetRect(0, 0, 6, 4)
	ta.Draw(ui.NewBuffer(ta.GetRect()))
	ta.mouse(mouse("<MouseLeft>", 1, 2))
	if ta.Cursor != image.Pt(7, 2) {
		t.Fatalf("scrolled area click=%v", ta.Cursor)
	}
}

func TestSelectionInEveryEditableForm(t *testing.T) {
	cfg := config.Default()
	cfg.Accounts[0].Maildir = filepath.Join(t.TempDir(), "mail")
	cfg.Collections = []config.Collection{{Name: "Contacts", Protocol: "carddav", LocalDir: filepath.Join(t.TempDir(), "contacts")}, {Name: "Calendar", Protocol: "caldav", LocalDir: filepath.Join(t.TempDir(), "calendar")}}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.startCompose(nil, false)
	for _, field := range []composeField{composeTo, composeCc, composeBcc, composeSubject} {
		a.compose.field = field
		in := a.activeInput()
		in.Text, in.Cursor = "old", 3
		a.handleComposeEvent(keyboard("<M-a>"))
		a.handleComposeEvent(keyboard("N"))
		if in.Text != "N" {
			t.Fatalf("field %v: %q", field, in.Text)
		}
	}
	a.compose.field = composeBody
	a.compose.body.Text = "old\nbody"
	a.handleComposeEvent(keyboard("<M-a>"))
	a.handleComposeEvent(keyboard("B"))
	if a.compose.body.Text != "B" {
		t.Fatal("body not replaced")
	}
	a.compose.attachPrompt.TitleBottom = "active"
	a.compose.attachPrompt.Text = "/old/path"
	a.handleComposeEvent(keyboard("<M-a>"))
	a.handleComposeEvent(keyboard("/"))
	if a.compose.attachPrompt.Text != "/" {
		t.Fatal("attachment path not replaced")
	}
	a.compose = nil
	a.startSearch()
	a.searchPrompt.Text = "old query"
	a.handleSearchEvent(keyboard("<M-a>"))
	a.handleSearchEvent(keyboard("Q"))
	if a.searchPrompt.Text != "Q" {
		t.Fatal("search not replaced")
	}
	a.searchActive = false
	for _, view := range []int{1, 2} {
		a.setView(view)
		a.startPIMEditor(false)
		for i, in := range a.pimEditor.fields {
			a.pimEditor.field = i
			in.Text, in.Cursor = "old", 3
			a.handlePIMEditor(keyboard("<M-a>"))
			a.handlePIMEditor(keyboard("P"))
			if in.Text != "P" {
				t.Fatalf("PIM %d field %d", view, i)
			}
		}
		a.pimEditor = nil
	}
	// Raw source editing must scroll without inserting hard-wrap newlines.
	ta := newTextArea()
	ta.Text = strings.Repeat("x", 100)
	ta.SetRect(0, 0, 10, 5)
	ta.Cursor = image.Pt(100, 0)
	a.pimEditor = &pimEditor{raw: ta}
	a.handlePIMEditor(keyboard("y"))
	if strings.Contains(ta.Text, "\n") {
		t.Fatal("raw source was hard-wrapped")
	}
	a.handlePIMEditor(keyboard("<M-a>"))
	a.handlePIMEditor(keyboard("R"))
	if ta.Text != "R" {
		t.Fatal("raw source not replaced")
	}
}

func TestComposerUsesShiftPayloadAndKeepsDragFocus(t *testing.T) {
	a := &App{cfg: config.Default()}
	a.startCompose(nil, false)
	a.compose.to.Text, a.compose.to.Cursor = "abc", 3
	a.handleComposeEvent(ui.Event{Type: ui.KeyboardEvent, ID: "<Left>", Payload: tcell.NewEventKey(tcell.KeyLeft, "", tcell.ModShift)})
	a.handleComposeEvent(keyboard("X"))
	if a.compose.to.Text != "abX" {
		t.Fatalf("shift payload replacement=%q", a.compose.to.Text)
	}
	a.compose.to.SetRect(0, 0, 12, 3)
	a.compose.cc.SetRect(0, 3, 12, 6)
	a.handleComposeEvent(mouse("<MouseLeft>", 1, 1))
	a.handleComposeEvent(mouse("<MouseLeft>", 3, 4))
	if a.compose.field != composeTo {
		t.Fatal("drag crossed into Cc")
	}
	a.handleComposeEvent(mouse("<MouseRelease>", 3, 4))
	a.handleComposeEvent(mouse("<MouseLeft>", 1, 4))
	if a.compose.field != composeCc {
		t.Fatal("next click didn't focus Cc")
	}
}

func TestSelectionPasteKeepsBodyFocusAndDefaultAttach(t *testing.T) {
	a := &App{cfg: config.Default()}
	a.startCompose(nil, false)
	a.compose.field = composeBody
	a.compose.body.Text = "replace me"
	a.handleComposeEvent(keyboard("<M-a>"))
	for _, id := range []string{"a", "<Tab>", "b", "<Enter>", "c"} {
		a.handleComposeEvent(keyboard(id))
	}
	if a.compose.body.Text != "a    b\nc" || a.compose.field != composeBody {
		t.Fatal("paste didn't replace the selection in the body")
	}
	a.handleComposeEvent(ui.Event{Type: ui.KeyboardEvent, ID: "<Key:279>", Payload: tcell.NewEventKey(tcell.KeyTab, "", tcell.ModShift)})
	if a.compose.field != composeSubject {
		t.Fatal("real Shift+Tab didn't leave body")
	}
	a.handleComposeEvent(keyboard("<C-a>"))
	if a.compose.attachPrompt.TitleBottom != "active" {
		t.Fatal("default Ctrl+A attach shortcut changed")
	}
}
