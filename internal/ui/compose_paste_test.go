package uiapp

import (
	"testing"

	ui "github.com/metaspartan/gotui/v5"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
)

func TestComposeBodyTabStaysInBody(t *testing.T) {
	a := &App{
		cfg: config.Config{
			Accounts: []config.Account{{Name: "test", From: "test@example.com"}},
			Keybindings: config.Keybindings{
				NextField:     "Tab",
				PreviousField: "Shift+Tab",
			},
		},
	}
	a.startCompose(nil, false)
	a.compose.field = composeBody

	for _, id := range []string{"a", "<Tab>", "b"} {
		a.handleComposeEvent(ui.Event{Type: ui.KeyboardEvent, ID: id})
	}

	if a.compose.field != composeBody {
		t.Fatalf("compose field = %v, want Body", a.compose.field)
	}
	if got, want := a.compose.body.Text, "a    b"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if a.compose.to.Text != "" || a.compose.cc.Text != "" || a.compose.bcc.Text != "" || a.compose.subject.Text != "" {
		t.Fatalf("body input leaked into header fields: to=%q cc=%q bcc=%q subject=%q", a.compose.to.Text, a.compose.cc.Text, a.compose.bcc.Text, a.compose.subject.Text)
	}
}

func TestComposeBodyShiftTabStillLeavesBody(t *testing.T) {
	a := &App{
		cfg: config.Config{
			Accounts: []config.Account{{Name: "test", From: "test@example.com"}},
			Keybindings: config.Keybindings{
				NextField:     "Tab",
				PreviousField: "Shift+Tab",
			},
		},
	}
	a.startCompose(nil, false)
	a.compose.field = composeBody

	a.handleComposeEvent(ui.Event{Type: ui.KeyboardEvent, ID: "<Backtab>"})

	if a.compose.field != composeSubject {
		t.Fatalf("compose field = %v, want Subject", a.compose.field)
	}
}
