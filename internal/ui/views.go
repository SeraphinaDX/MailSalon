package uiapp

import (
	"fmt"
	"image"

	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

// gotui's Block.SetRect reserves a border-sized inset even for borderless
// paragraphs. Give bars their full rectangle so every help row is visible.
func setBarRect(p *widgets.Paragraph, x1, y1, x2, y2 int) {
	p.SetRect(x1, y1, x2, y2)
	p.Inner = p.Rectangle
}

func (a *App) layoutViewTabs(width int) []ui.Drawable {
	keys := a.bindings()
	labels := []string{keyLabel(keys.MailView) + " Mail", keyLabel(keys.ContactsView) + " Contacts", keyLabel(keys.CalendarView) + " Calendar"}
	items := make([]ui.Drawable, 0, 3)
	for i, label := range labels {
		if a.viewTabs[i] == nil {
			a.viewTabs[i] = widgets.NewParagraph()
			a.viewTabs[i].Border = false
			a.viewTabs[i].WrapText = false
		}
		tab := a.viewTabs[i]
		tab.BackgroundColor = a.theme.background
		tab.TextStyle = ui.NewStyle(a.theme.title, a.theme.background)
		tab.Text = safeUI("  " + label)
		if i == a.view {
			tab.BackgroundColor = a.theme.selectedBG
			tab.TextStyle = ui.NewStyle(a.theme.selectedFG, a.theme.selectedBG)
			tab.Text = safeUI(" ● " + label)
		}
		setBarRect(tab, i*width/3, 0, (i+1)*width/3, 1)
		items = append(items, tab)
	}
	return items
}

func (a *App) handleViewTabMouse(event ui.Event) bool {
	if event.ID != "<MouseLeft>" && event.ID != "MouseLeft" {
		return false
	}
	m, ok := event.Payload.(ui.Mouse)
	if !ok {
		return false
	}
	for i, tab := range a.viewTabs {
		if tab != nil && image.Pt(m.X, m.Y).In(tab.Rectangle) {
			if i != a.view {
				a.setView(i)
			}
			return true
		}
	}
	return false
}

func (a *App) pimScope() string {
	if len(a.pimCollections) == 0 {
		return "No collections for " + a.currentAccount().Name
	}
	c := a.pimCollections[a.pimCollection]
	if c.Account == "" {
		return c.Name + " (shared across accounts)"
	}
	return fmt.Sprintf("%s (%s only)", c.Name, c.Account)
}
