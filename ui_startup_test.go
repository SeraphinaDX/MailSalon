package main

import (
	"strings"
	"testing"
)

func TestStartupSyncShowsLocalMailBeforeSync(t *testing.T) {
	m := newModel(Config{StartupSync: true})
	messages := []Message{{Subject: "existing local message"}}

	updated, cmd := m.Update(loadedMsg{messages: messages})
	next, ok := updated.(model)
	if !ok {
		t.Fatalf("updated model has type %T, want model", updated)
	}
	if len(next.messages) != 1 || next.messages[0].Subject != "existing local message" {
		t.Fatalf("local messages were not preserved before startup sync: %#v", next.messages)
	}
	if !next.startupSyncStarted {
		t.Fatal("startup sync was not started after loading local mail")
	}
	if !next.busy {
		t.Fatal("model should be busy while startup sync runs")
	}
	if cmd == nil {
		t.Fatal("expected startup sync command after local mail load")
	}
	if !strings.Contains(next.status, "loaded 1 messages") || !strings.Contains(next.status, "syncing with offlineimap") {
		t.Fatalf("unexpected status: %q", next.status)
	}
}
