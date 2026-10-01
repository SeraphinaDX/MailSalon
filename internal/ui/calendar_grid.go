package uiapp

import (
	"fmt"
	"image"
	"strings"
	"time"
	"unicode"

	"github.com/mattn/go-runewidth"
	ui "github.com/metaspartan/gotui/v5"
)

// A date grid uses terminal cells directly so event titles cannot inject styles.
type calendarGrid struct {
	ui.Block
	date, first   time.Time
	week, focused bool
	rows          []calendarRow
	theme         resolvedTheme
}

func newCalendarGrid() *calendarGrid { return &calendarGrid{Block: *ui.NewBlock()} }

func (g *calendarGrid) cellRect(index int) image.Rectangle {
	weeks := 6
	if g.week {
		weeks = 1
	}
	col, row := index%7, index/7
	return image.Rect(g.Inner.Min.X+g.Inner.Dx()*col/7, g.Inner.Min.Y+1+(g.Inner.Dy()-1)*row/weeks, g.Inner.Min.X+g.Inner.Dx()*(col+1)/7, g.Inner.Min.Y+1+(g.Inner.Dy()-1)*(row+1)/weeks)
}

func (g *calendarGrid) dateAt(p image.Point) (time.Time, bool) {
	count := 42
	if g.week {
		count = 7
	}
	for i := 0; i < count; i++ {
		if p.In(g.cellRect(i)) {
			return g.first.AddDate(0, 0, i), true
		}
	}
	return time.Time{}, false
}

func gridText(buf *ui.Buffer, text string, style ui.Style, rect image.Rectangle, y int) {
	if y >= rect.Max.Y || y < rect.Min.Y {
		return
	}
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	text = runewidth.Truncate(text, max(0, rect.Dx()-1), "…")
	buf.SetString(text, style, image.Pt(rect.Min.X, y))
}

func (g *calendarGrid) Draw(buf *ui.Buffer) {
	g.BackgroundColor = g.theme.background
	g.BorderStyle = ui.NewStyle(g.theme.border, g.theme.background)
	if g.focused {
		g.BorderStyle = ui.NewStyle(g.theme.activeBorder, g.theme.background)
	}
	g.Title = "Dates · * Today · arrows / click"
	g.TitleStyle = ui.NewStyle(g.theme.title, g.theme.background)
	g.Block.Draw(buf)
	normal := ui.NewStyle(g.theme.foreground, g.theme.background)
	for col := 0; col < 7; col++ {
		r := g.cellRect(col)
		gridText(buf, g.first.AddDate(0, 0, col).Format("Mon"), ui.NewStyle(g.theme.title, g.theme.background), image.Rect(r.Min.X, g.Inner.Min.Y, r.Max.X, g.Inner.Min.Y+1), g.Inner.Min.Y)
	}
	count := 42
	if g.week {
		count = 7
	}
	for i := 0; i < count; i++ {
		r := g.cellRect(i)
		day := g.first.AddDate(0, 0, i)
		style := normal
		if !g.week && day.Month() != g.date.Month() {
			style = ui.NewStyle(g.theme.muted, g.theme.background)
		}
		if sameDate(day, g.date) {
			style = ui.NewStyle(g.theme.selectedFG, g.theme.selectedBG)
			buf.Fill(ui.NewCell(' ', style), r)
		}
		var events []calendarRow
		for _, row := range g.rows {
			if row.Intersects(day, day.AddDate(0, 0, 1)) {
				events = append(events, row)
			}
		}
		header := fmt.Sprint(day.Day())
		if g.week {
			header = day.Format("2 Jan")
		}
		if sameDate(day, time.Now()) {
			header += "*"
		}
		if len(events) > 0 {
			header += fmt.Sprintf("·%d", len(events))
		}
		gridText(buf, header, style, r, r.Min.Y)
		available := max(0, r.Dy()-1)
		shown := min(len(events), available)
		if len(events) > available && available > 0 {
			shown = available - 1
		}
		for n := 0; n < shown; n++ {
			row := events[n]
			text := row.Title
			if r.Dx() >= 12 || g.week {
				prefix := row.Start.Format("15:04")
				if row.AllDay {
					prefix = "All day"
				}
				text = prefix + " " + text
			}
			gridText(buf, text, style, r, r.Min.Y+n+1)
		}
		if len(events) > shown && available > 0 {
			gridText(buf, fmt.Sprintf("+%d more", len(events)-shown), style, r, r.Min.Y+shown+1)
		}
		// A column separator keeps adjacent days legible even when titles fill it.
		if i%7 < 6 {
			for y := r.Min.Y; y < r.Max.Y; y++ {
				buf.SetCell(ui.NewCell('│', ui.NewStyle(g.theme.border, g.theme.background)), image.Pt(r.Max.X-1, y))
			}
		}
	}
}
