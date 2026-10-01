package pim

import (
	"os"
	"path/filepath"
	"testing"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/calendar"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
)

func TestCalendarImportFormatsDedupeAndLocks(t *testing.T) {
	events, err := calendar.Parse([]byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:../../remote/unsafe@example.com\r\nSUMMARY:Test\r\nDTSTART:20261002T100000Z\r\nDTEND:20261002T110000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, protocol := range []string{"caldav", "jmap-calendars"} {
		t.Run(protocol, func(t *testing.T) {
			c := config.Collection{Name: "Calendar", Protocol: protocol, LocalDir: filepath.Join(t.TempDir(), "events")}
			added, err := ImportCalendar(c, events[0])
			if err != nil || !added {
				t.Fatal(added, err)
			}
			items, err := Load(c)
			if err != nil || len(items) != 1 {
				t.Fatal(items, err)
			}
			if items[0].UID != events[0].UID || filepath.Dir(items[0].Path) != c.LocalDir {
				t.Fatal("UID or destination changed", items[0])
			}
			before, err := os.ReadFile(items[0].Path)
			if err != nil {
				t.Fatal(err)
			}
			added, err = ImportCalendar(c, events[0])
			if err != nil || added {
				t.Fatal("duplicate import", added, err)
			}
			after, _ := os.ReadFile(items[0].Path)
			if string(before) != string(after) {
				t.Fatal("existing record modified")
			}
			if err := os.WriteFile(filepath.Join(c.LocalDir, ".mss-lock"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := ImportCalendar(c, events[0]); err == nil {
				t.Fatal("import ignored sync lock")
			}
		})
	}
}
