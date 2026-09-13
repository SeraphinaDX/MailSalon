package uiapp

import (
	"strings"
	"unicode"

	rw "github.com/mattn/go-runewidth"
	"github.com/metaspartan/gotui/v5/widgets"
)

// wrapComposeBodyLine hard-wraps the logical line containing the cursor so the
// gotui TextArea never has to display text beyond its right edge. gotui's
// TextArea clips long logical lines instead of wrapping them, which makes the
// cursor and newly typed text disappear past the border.
//
// Keep one cell free at the right edge so the cursor remains visible while
// typing. Prefer a whitespace boundary when possible; fall back to a hard
// character-cell break for long words/URLs.
func wrapComposeBodyLine(ta *widgets.TextArea) {
	if ta == nil {
		return
	}
	width := ta.Inner.Dx() - 1
	if width < 1 {
		return
	}

	lines := strings.Split(ta.Text, "\n")
	if len(lines) == 0 {
		return
	}
	y := clamp(ta.Cursor.Y, 0, len(lines)-1)
	cursorX := max(0, ta.Cursor.X)
	segments := wrapComposeLine(lines[y], width)
	if len(segments) <= 1 {
		return
	}

	before := append([]string(nil), lines[:y]...)
	after := append([]string(nil), lines[y+1:]...)
	newLines := make([]string, 0, len(before)+len(segments)+len(after))
	newLines = append(newLines, before...)
	newLines = append(newLines, segments...)
	newLines = append(newLines, after...)
	ta.Text = strings.Join(newLines, "\n")

	remaining := cursorX
	segmentIndex := 0
	for segmentIndex < len(segments)-1 {
		n := len([]rune(segments[segmentIndex]))
		if remaining < n {
			break
		}
		remaining -= n
		segmentIndex++
	}
	segmentRunes := len([]rune(segments[segmentIndex]))
	if remaining > segmentRunes {
		remaining = segmentRunes
	}
	ta.Cursor.X = remaining
	ta.Cursor.Y = y + segmentIndex
}

func wrapComposeLine(line string, width int) []string {
	if width < 1 {
		return []string{line}
	}
	runes := []rune(line)
	if len(runes) == 0 || rw.StringWidth(line) <= width {
		return []string{line}
	}

	out := make([]string, 0, 2)
	for len(runes) > 0 {
		if rw.StringWidth(string(runes)) <= width {
			out = append(out, string(runes))
			break
		}

		fit := 0
		cells := 0
		for fit < len(runes) {
			w := rw.RuneWidth(runes[fit])
			if w < 1 {
				w = 1
			}
			if cells+w > width {
				break
			}
			cells += w
			fit++
		}
		if fit == 0 {
			fit = 1
		}

		split := fit
		for i := fit - 1; i > 0; i-- {
			if unicode.IsSpace(runes[i]) {
				split = i + 1
				break
			}
		}
		out = append(out, string(runes[:split]))
		runes = runes[split:]
	}
	return out
}
