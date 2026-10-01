package calendar

import (
	"encoding/json"
	"strings"
	"testing"
)

const example = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:meeting/test@example.com\r\nDTSTAMP:20261001T120000Z\r\nSUMMARY:Planning\\, session\r\nDTSTART;TZID=America/Toronto:20261002T100000\r\nDTEND;TZID=America/Toronto:20261002T110000\r\nLOCATION:Room A\r\nDESCRIPTION:First line\\nSecond\r\n line\r\nORGANIZER;CN=Alice:mailto:alice@example.com\r\nATTENDEE;CN=Me;RSVP=TRUE;PARTSTAT=NEEDS-ACTION:mailto:me@example.com\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

func TestEventPreviewAndNativeImports(t *testing.T) {
	events, err := Parse([]byte(example))
	if err != nil {
		t.Fatal(err)
	}
	e := events[0]
	if e.Title != "Planning, session" || e.Zone != "America/Toronto" || e.Start != "2026-10-02T10:00:00" || e.Description != "First line\nSecondline" || !strings.Contains(e.Organizer, "Alice") || len(e.Attendees) != 1 {
		t.Fatalf("bad preview: %+v", e)
	}
	source, err := e.ICalendar()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "METHOD:") || !strings.Contains(string(source), "SCHEDULE-AGENT=NONE") {
		t.Fatalf("import must store an event, not scheduling message: %s", source)
	}
	if e.main.Props.Get("ORGANIZER").Params.Get("SCHEDULE-AGENT") != "" {
		t.Fatal("import modified the source invitation")
	}
	data, err := e.JSCalendar()
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	if object["uid"] != e.UID || object["duration"] != "PT3600S" || object["timeZone"] != "America/Toronto" || object["start"] != "2026-10-02T10:00:00" {
		t.Fatalf("bad JSCalendar: %s", data)
	}
	participants := object["participants"].(map[string]any)
	attendee := participants["attendee0"].(map[string]any)
	if attendee["scheduleAgent"] != "none" || attendee["participationStatus"] != "needs-action" || attendee["expectReply"] != true {
		t.Fatal(attendee)
	}
}

func TestMultipleUIDsAndRecurrencePreserved(t *testing.T) {
	s := strings.Replace(example, "END:VCALENDAR", "BEGIN:VEVENT\r\nUID:meeting/test@example.com\r\nRECURRENCE-ID;TZID=America/Toronto:20261003T100000\r\nDTSTART;TZID=America/Toronto:20261003T120000\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nUID:second\r\nDTSTART;VALUE=DATE:20261005\r\nSUMMARY:Day off\r\nEND:VEVENT\r\nEND:VCALENDAR", 1)
	s = strings.Replace(s, "DTSTAMP:", "RRULE:FREQ=DAILY;COUNT=2\r\nDTSTAMP:", 1)
	events, err := Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || !events[0].Recurring || !events[1].AllDay {
		t.Fatal(events)
	}
	raw, err := events[0].ICalendar()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "RECURRENCE-ID") || !strings.Contains(string(raw), "RRULE:") || strings.Contains(string(raw), "UID:second") {
		t.Fatal(string(raw))
	}
	if _, err := events[0].JSCalendar(); err == nil {
		t.Fatal("complex conversion silently dropped recurrence")
	}
	data, err := events[1].JSCalendar()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"duration": "P1D"`) || !strings.Contains(string(data), `"showWithoutTime": true`) {
		t.Fatal(string(data))
	}
}

func TestInvalidAndSchedulingResources(t *testing.T) {
	for _, s := range []string{"not a calendar", strings.ReplaceAll(example, "UID:meeting/test@example.com\r\n", ""), strings.ReplaceAll(example, "20261002T100000", "invalid"), strings.ReplaceAll(example, "END:VEVENT", "END:VTODO")} {
		if _, err := Parse([]byte(s)); err == nil {
			t.Fatal("invalid resource accepted")
		}
	}
	for _, method := range []string{"CANCEL", "REPLY", "COUNTER"} {
		events, err := Parse([]byte(strings.Replace(example, "METHOD:REQUEST", "METHOD:"+method, 1)))
		if err != nil {
			t.Fatal(err)
		}
		if events[0].CanImport() == nil {
			t.Fatalf("%s scheduling message accepted as new event", method)
		}
	}
	for _, insert := range []string{"X-UNKNOWN:keep me\r\n", "BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\n"} {
		events, err := Parse([]byte(strings.Replace(example, "END:VEVENT", insert+"END:VEVENT", 1)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := events[0].JSCalendar(); err == nil {
			t.Fatal("JMAP conversion lost unmapped data")
		}
		raw, err := events[0].ICalendar()
		if err != nil || !strings.Contains(string(raw), strings.Split(insert, "\r\n")[0]) {
			t.Fatal("CalDAV lost data", err)
		}
	}
}

func TestCustomTimezoneSurvivesCalDAVImport(t *testing.T) {
	zone := "BEGIN:VTIMEZONE\r\nTZID:Custom/Office\r\nBEGIN:STANDARD\r\nDTSTART:19700101T000000\r\nTZOFFSETFROM:+0200\r\nTZOFFSETTO:+0200\r\nEND:STANDARD\r\nEND:VTIMEZONE\r\n"
	s := strings.ReplaceAll(example, "America/Toronto", "Custom/Office")
	s = strings.Replace(s, "BEGIN:VEVENT", zone+"BEGIN:VEVENT", 1)
	events, err := Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	data, err := events[0].ICalendar()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "TZID:Custom/Office") || !strings.Contains(string(data), "TZOFFSETTO:+0200") {
		t.Fatal(string(data))
	}
	if _, err := events[0].JSCalendar(); err == nil {
		t.Fatal("custom timezone guessed during conversion")
	}
}
