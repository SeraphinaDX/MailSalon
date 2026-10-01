package calendar

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func occurrenceFixture(body string) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" + strings.ReplaceAll(body, "\n", "\r\n") + "END:VCALENDAR\r\n")
}
func occurrenceDay(s string, loc *time.Location) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", s, loc)
	return t
}

func TestOccurrenceExclusiveEndAndTimezones(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body, day   string
		count, hour int
		allDay      bool
	}{
		{"BEGIN:VEVENT\nUID:multi\nDTSTART;VALUE=DATE:20260307\nDTEND;VALUE=DATE:20260310\nEND:VEVENT\n", "2026-03-09", 1, 0, true},
		{"BEGIN:VEVENT\nUID:multi\nDTSTART;VALUE=DATE:20260307\nDTEND;VALUE=DATE:20260310\nEND:VEVENT\n", "2026-03-10", 0, 0, true},
		{"BEGIN:VEVENT\nUID:utc\nDTSTART:20260308T040000Z\nDTEND:20260308T050000Z\nEND:VEVENT\n", "2026-03-07", 1, 23, false},
		{"BEGIN:VEVENT\nUID:float\nDTSTART:20260308T100000\nDTEND:20260308T110000\nEND:VEVENT\n", "2026-03-08", 1, 10, false},
		{"BEGIN:VEVENT\nUID:point\nDTSTART:20260308T100000\nEND:VEVENT\n", "2026-03-08", 1, 10, false},
		{"BEGIN:VEVENT\nUID:night\nDTSTART:20260307T230000\nDTEND:20260308T010000\nEND:VEVENT\n", "2026-03-08", 1, 23, false},
	} {
		from := occurrenceDay(tc.day, loc)
		out, err := Occurrences(occurrenceFixture(tc.body), false, from, from.AddDate(0, 0, 1), loc)
		if err != nil || len(out) != tc.count {
			t.Fatalf("%s: %v %v", tc.day, out, err)
		}
		if len(out) > 0 && (out[0].Start.Hour() != tc.hour || out[0].AllDay != tc.allDay) {
			t.Fatal(out)
		}
	}
}

func TestICalendarRecurrenceExdatesMovedAndCancelledExceptions(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	data := occurrenceFixture("BEGIN:VEVENT\nUID:weekly\nSUMMARY:Weekly\nDTSTART;TZID=America/New_York:20260301T100000\nDTEND;TZID=America/New_York:20260301T110000\nRRULE:FREQ=WEEKLY;COUNT=5\nEXDATE;TZID=America/New_York:20260315T100000,20260329T100000\nRDATE;TZID=America/New_York:20260305T100000,20260306T100000\nEND:VEVENT\nBEGIN:VEVENT\nUID:weekly\nRECURRENCE-ID;TZID=America/New_York:20260308T100000\nDTSTART;TZID=America/New_York:20260309T120000\nSUMMARY:Moved\nEND:VEVENT\nBEGIN:VEVENT\nUID:weekly\nRECURRENCE-ID;TZID=America/New_York:20260322T100000\nSTATUS:CANCELLED\nEND:VEVENT\n")
	from := occurrenceDay("2026-03-01", loc)
	out, err := Occurrences(data, false, from, from.AddDate(0, 1, 0), loc)
	if err != nil || len(out) != 4 {
		t.Fatal(out, err)
	}
	if out[3].Title != "Moved" || out[3].Start.Day() != 9 || out[3].Start.Hour() != 12 || out[3].End.Hour() != 13 {
		t.Fatal(out[3])
	}
	for _, o := range out {
		if !o.Recurring {
			t.Fatal("missing series marker")
		}
	}
	// An exception moved into a window must show even when its original date is outside it.
	from = occurrenceDay("2026-03-09", loc)
	out, err = Occurrences(data, false, from, from.AddDate(0, 0, 1), loc)
	if err != nil || len(out) != 1 || out[0].Title != "Moved" {
		t.Fatal(out, err)
	}
}

func TestDailyRecurrenceAcrossDST(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	from := occurrenceDay("2026-03-07", loc)
	data := occurrenceFixture("BEGIN:VEVENT\nUID:dst\nDTSTART;TZID=America/New_York:20260307T090000\nDURATION:PT1H\nRRULE:FREQ=DAILY;COUNT=3\nEND:VEVENT\n")
	out, err := Occurrences(data, false, from, from.AddDate(0, 0, 3), loc)
	if err != nil || len(out) != 3 {
		t.Fatal(out, err)
	}
	for _, o := range out {
		if o.Start.Hour() != 9 || o.End.Hour() != 10 {
			t.Fatal(o)
		}
	}
	data = occurrenceFixture("BEGIN:VEVENT\nUID:dates\nDTSTART;VALUE=DATE:20261101\nDURATION:P1D\nRRULE:FREQ=DAILY;COUNT=2\nEND:VEVENT\n")
	from = occurrenceDay("2026-11-01", loc)
	out, err = Occurrences(data, false, from, from.AddDate(0, 0, 2), loc)
	if err != nil || len(out) != 2 || out[0].End.Day() != 2 || out[0].End.Hour() != 0 {
		t.Fatal(out, err)
	}
}

func TestJSCalendarRulesOverridesAndAllDay(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	from := occurrenceDay("2026-03-01", loc)
	v := map[string]any{"@type": "Event", "uid": "js", "title": "JS weekly", "start": "2026-03-01T09:00:00", "duration": "PT1H", "timeZone": "America/New_York", "recurrenceRules": []any{map[string]any{"@type": "RecurrenceRule", "frequency": "weekly", "count": 4, "byDay": []any{map[string]any{"day": "su"}}}}, "recurrenceOverrides": map[string]any{"2026-03-08T09:00:00": map[string]any{"excluded": true}, "2026-03-15T09:00:00": map[string]any{"start": "2026-03-16T12:00:00", "title": "Moved JS"}}}
	data, _ := json.Marshal(v)
	out, err := Occurrences(data, true, from, from.AddDate(0, 1, 0), loc)
	if err != nil || len(out) != 3 || out[1].Title != "Moved JS" || out[1].Start.Day() != 16 || out[1].End.Hour() != 13 || out[2].Start.Hour() != 9 {
		t.Fatal(out, err)
	}
	v = map[string]any{"@type": "Event", "uid": "all", "start": "2026-11-01T00:00:00", "duration": "P2D", "showWithoutTime": true, "timeZone": "UTC"}
	data, _ = json.Marshal(v)
	from = occurrenceDay("2026-11-02", loc)
	out, err = Occurrences(data, true, from, from.AddDate(0, 0, 1), loc)
	if err != nil || len(out) != 1 || !out[0].AllDay || out[0].End.Day() != 3 {
		t.Fatal(out, err)
	}
}

func TestUnsupportedCalendarProjectionIsExplicit(t *testing.T) {
	from := occurrenceDay("2026-03-01", time.UTC)
	for _, body := range []string{
		"BEGIN:VEVENT\nUID:hourly\nDTSTART:20260301T100000Z\nRRULE:FREQ=HOURLY\nEND:VEVENT\n",
		"BEGIN:VEVENT\nUID:zone\nDTSTART;TZID=CustomZone:20260301T100000\nEND:VEVENT\n",
		"BEGIN:VEVENT\nUID:backward\nDTSTART:20260301T100000Z\nDTEND:20260301T090000Z\nEND:VEVENT\n",
	} {
		if _, err := Occurrences(occurrenceFixture(body), false, from, from.AddDate(0, 1, 0), time.UTC); err == nil {
			t.Fatal("silently displayed unsupported resource")
		}
	}
}
