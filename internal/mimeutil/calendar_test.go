package mimeutil

import (
	"encoding/base64"
	"strings"
	"testing"
)

const incomingCalendar = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:incoming\r\nDTSTART:20261002T100000Z\r\nDTEND:20261002T110000Z\r\nSUMMARY:Meeting\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

func TestCalendarMIMEForms(t *testing.T) {
	for _, header := range []string{"Content-Type: text/calendar; method=REQUEST", "Content-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=\"MEETING.ICS\"", "Content-Type: application/ics"} {
		raw := []byte("From: alice@example.com\r\nTo: me@example.com\r\nSubject: Calendar\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=test\r\n\r\n--test\r\nContent-Type: text/plain\r\n\r\nHello\r\n--test\r\n" + header + "\r\nContent-Transfer-Encoding: base64\r\n\r\n" + base64.StdEncoding.EncodeToString([]byte(incomingCalendar)) + "\r\n--test--\r\n")
		p, err := ParseBytes(raw)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.CalendarEvents) != 1 || len(p.Attachments) != 1 || len(p.CalendarErrors) != 0 || !strings.Contains(p.Body, "Hello") || !strings.HasSuffix(strings.ToLower(p.Attachments[0].Filename), ".ics") {
			t.Fatalf("calendar MIME not detected: %+v", p)
		}
	}
}
func TestMalformedCalendarKeepsMailAndOriginal(t *testing.T) {
	p, err := ParseBytes([]byte("From: alice@example.com\r\nContent-Type: text/calendar\r\n\r\ninvalid calendar"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.CalendarEvents) != 0 || len(p.CalendarErrors) != 1 || len(p.Attachments) != 1 || string(p.Attachments[0].Data) != "invalid calendar" {
		t.Fatal(p)
	}
}
func TestMIMEMethodCannotHideCancellation(t *testing.T) {
	raw := "Content-Type: text/calendar; method=CANCEL\r\n\r\n" + strings.Replace(incomingCalendar, "METHOD:REQUEST\r\n", "", 1)
	p, err := ParseBytes([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.CalendarEvents) != 1 || p.CalendarEvents[0].CanImport() == nil {
		t.Fatal("MIME cancellation imported")
	}
}
