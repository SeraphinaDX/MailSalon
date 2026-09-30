package pim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
)

func TestCreateContactsAndEvents(t *testing.T) {
	for _, protocol := range []string{"carddav", "caldav", "jmap-contacts", "jmap-calendars"} {
		t.Run(protocol, func(t *testing.T) {
			c := config.Collection{Protocol: protocol, LocalDir: t.TempDir()}
			fields := []string{"Alice, Example", "alice@example.test", "+1 555 1234"}
			if !Contacts(c) {
				fields = []string{"Meeting, today", "2026-09-30T10:00:00", "2026-09-30T11:00:00", "America/Toronto", "Room; one"}
			}
			data, uid, err := New(c, fields)
			if err != nil {
				t.Fatal(err)
			}
			if err := Save(c, nil, data, uid); err != nil {
				t.Fatal(err)
			}
			items, err := Load(c)
			if err != nil || len(items) != 1 {
				t.Fatalf("load: %v %+v", err, items)
			}
			item := items[0]
			if item.Title != fields[0] {
				t.Fatalf("title: %q", item.Title)
			}
			if Contacts(c) && item.Email != "alice@example.test" {
				t.Fatal("missing contact email")
			}
			if !Contacts(c) && item.Location != "Room; one" {
				t.Fatal("escaped location not decoded")
			}
			if protocol == "caldav" && item.Start != "2026-09-30T14:00:00" {
				t.Fatalf("timezone conversion: %s", item.Start)
			}
			if err := Delete(c, item); err != nil {
				t.Fatal(err)
			}
			items, err = Load(c)
			if err != nil || len(items) != 0 {
				t.Fatal("delete failed")
			}
			backups, _ := os.ReadDir(filepath.Join(c.LocalDir, ".mss-trash"))
			if len(backups) != 1 {
				t.Fatal("delete backup missing")
			}
		})
	}
}
func TestStaleEditorAndSyncLock(t *testing.T) {
	c := config.Collection{Protocol: "carddav", LocalDir: t.TempDir()}
	data, uid, _ := New(c, []string{"Alice", "alice@example.test", ""})
	if err := Save(c, nil, data, uid); err != nil {
		t.Fatal(err)
	}
	items, _ := Load(c)
	changed := []byte(strings.ReplaceAll(string(data), "Alice", "Remote Alice"))
	os.WriteFile(items[0].Path, changed, 0600)
	if err := Save(c, &items[0], data, ""); err == nil {
		t.Fatal("overwrote updated item from stale editor")
	}
	if err := Delete(c, items[0]); err == nil {
		t.Fatal("deleted changed item")
	}
	os.WriteFile(filepath.Join(c.LocalDir, ".mss-lock"), nil, 0600)
	if err := Save(c, nil, data, uid); err == nil {
		t.Fatal("ignored sync lock")
	}
}
func TestNativePreviewPreservesRecurrenceAndExtensions(t *testing.T) {
	c := config.Collection{Protocol: "caldav"}
	data := []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VTIMEZONE\r\nTZID:America/Toronto\r\nEND:VTIMEZONE\r\nBEGIN:VEVENT\r\nUID:a\r\nSUMMARY:Long title\r\n continued\r\nDTSTART;TZID=America/Toronto:20260930T100000\r\nRRULE:FREQ=WEEKLY\r\nX-UNKNOWN:keep\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nUID:a\r\nRECURRENCE-ID:20261007T100000\r\nSUMMARY:Override\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	i, err := Parse(c, data)
	if err != nil {
		t.Fatal(err)
	}
	if i.Title != "Long titlecontinued" || !i.Recurring || i.Zone != "America/Toronto" || string(i.Data) != string(data) {
		t.Fatalf("preview damaged data: %+v", i)
	}
}
func TestAllDayAndInputValidation(t *testing.T) {
	for _, protocol := range []string{"caldav", "jmap-calendars"} {
		c := config.Collection{Protocol: protocol}
		data, _, err := New(c, []string{"Holiday", "2026-10-01", "2026-10-02", "", ""})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Parse(c, data); err != nil {
			t.Fatal(err)
		}
		for _, fields := range [][]string{{"Bad", "bad date", "bad", "", ""}, {"Bad", "2026-10-02", "2026-10-01", "", ""}, {"Bad", "2026-10-01T10:00:00", "2026-10-01T11:00:00", "Not/AZone", ""}} {
			if _, _, err := New(c, fields); err == nil {
				t.Fatal("accepted invalid event")
			}
		}
	}
	if _, _, err := New(config.Collection{Protocol: "carddav"}, []string{"Alice", "broken email", ""}); err == nil {
		t.Fatal("accepted invalid email")
	}
}
func TestUnicodeFolding(t *testing.T) {
	c := config.Collection{Protocol: "carddav"}
	title := strings.Repeat("é", 80)
	data, _, err := New(c, []string{title, "", ""})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\r\n") {
		if len(line) > 75 {
			t.Fatal("overlong content line")
		}
	}
	i, err := Parse(c, data)
	if err != nil || i.Title != title {
		t.Fatalf("folding corrupted Unicode: %v", err)
	}
}
