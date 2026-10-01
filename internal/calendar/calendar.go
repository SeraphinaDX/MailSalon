// Package calendar decodes incoming iCalendar resources without changing their
// event semantics. Each imported resource contains one UID and its exceptions.
package calendar

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"time"

	ical "github.com/emersion/go-ical"
)

type Event struct {
	UID, Title, Start, End, Zone, EndZone, Location, Description, Organizer, Method string
	Attendees                                                                       []string
	AllDay, Recurring                                                               bool
	calendar                                                                        *ical.Calendar
	main                                                                            *ical.Component
}

func text(props ical.Props, key string) string {
	s, err := props.Text(key)
	if err != nil {
		return props.Get(key).Value
	}
	return s
}

// Parse handles multiple calendars/events and groups recurrence exceptions with
// their master UID. It never expands recurrences or guesses the user's timezone.
func Parse(data []byte) ([]Event, error) {
	decoder := ical.NewDecoder(bytes.NewReader(data))
	var events []Event
	for {
		cal, err := decoder.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if cal.Props.Get("VERSION") == nil || cal.Props.Get("VERSION").Value != "2.0" {
			return nil, errors.New("calendar needs VERSION:2.0")
		}
		groups := map[string][]*ical.Component{}
		var order []string
		for _, child := range cal.Children {
			if child.Name != "VEVENT" {
				continue
			}
			uid := text(child.Props, "UID")
			if strings.TrimSpace(uid) == "" {
				return nil, errors.New("event needs a UID")
			}
			if _, ok := groups[uid]; !ok {
				order = append(order, uid)
			}
			groups[uid] = append(groups[uid], child)
		}
		for _, uid := range order {
			group := groups[uid]
			main := group[0]
			masters := 0
			for _, event := range group {
				if event.Props.Get("RECURRENCE-ID") == nil {
					main = event
					masters++
				}
			}
			if masters > 1 {
				return nil, fmt.Errorf("multiple master events for UID %s", uid)
			}
			start := main.Props.Get("DTSTART")
			method := strings.ToUpper(text(cal.Props, "METHOD"))
			if start == nil && method != "CANCEL" && method != "REPLY" {
				return nil, errors.New("event needs DTSTART")
			}
			e := Event{UID: uid, Title: text(main.Props, "SUMMARY"), Location: text(main.Props, "LOCATION"), Description: text(main.Props, "DESCRIPTION"), Method: method, main: main}
			if e.Title == "" {
				e.Title = uid
			}
			if start != nil {
				var err error
				e.Start, e.Zone, e.AllDay, err = previewDate(start)
				if err != nil {
					return nil, err
				}
			}
			if end := main.Props.Get("DTEND"); end != nil {
				var err error
				e.End, e.EndZone, _, err = previewDate(end)
				if err != nil {
					return nil, err
				}
			}
			if duration := main.Props.Get("DURATION"); duration != nil {
				if _, err := duration.Duration(); err != nil {
					return nil, err
				}
				e.End = duration.Value
			}
			e.Organizer = person(main.Props.Get("ORGANIZER"))
			for _, p := range main.Props.Values("ATTENDEE") {
				e.Attendees = append(e.Attendees, person(&p))
			}
			e.Recurring = len(group) > 1 || main.Props.Get("RRULE") != nil || main.Props.Get("RDATE") != nil || main.Props.Get("RECURRENCE-ID") != nil
			// Retain calendar properties and timezone definitions, but only this UID.
			e.calendar = ical.NewCalendar()
			for key, values := range cal.Props {
				e.calendar.Props[key] = values
			}
			for _, child := range cal.Children {
				if child.Name == "VTIMEZONE" {
					e.calendar.Children = append(e.calendar.Children, child)
				}
			}
			// Put the master first so local previews show the series start even if
			// the sender listed an exception before it.
			e.calendar.Children = append(e.calendar.Children, main)
			for _, child := range group {
				if child != main {
					e.calendar.Children = append(e.calendar.Children, child)
				}
			}
			events = append(events, e)
		}
	}
	if len(events) == 0 {
		return nil, errors.New("calendar contains no VEVENTs")
	}
	return events, nil
}

func previewDate(p *ical.Prop) (string, string, bool, error) {
	zone := p.Params.Get("TZID")
	layout := "20060102T150405"
	allDay := p.ValueType() == ical.ValueDate
	if allDay {
		layout = "20060102"
	} else if strings.HasSuffix(p.Value, "Z") {
		layout = "20060102T150405Z"
		zone = "UTC"
	}
	// Parse the written wall clock without interpreting unfamiliar VTIMEZONE IDs.
	t, err := time.Parse(layout, p.Value)
	if err != nil {
		return "", "", false, fmt.Errorf("invalid %s: %w", p.Name, err)
	}
	if allDay {
		return t.Format("2006-01-02"), "", true, nil
	}
	return t.Format("2006-01-02T15:04:05"), zone, false, nil
}

func person(p *ical.Prop) string {
	if p == nil {
		return ""
	}
	address := p.Value
	if strings.HasPrefix(strings.ToLower(address), "mailto:") {
		address = address[7:]
	}
	name := p.Params.Get("CN")
	if parsed, err := mail.ParseAddress(address); err == nil {
		parsed.Name = name
		return parsed.String()
	}
	if name != "" {
		return name + " <" + address + ">"
	}
	return address
}

func (e Event) CanImport() error {
	switch e.Method {
	case "", "REQUEST", "PUBLISH":
	default:
		return fmt.Errorf("%s is a scheduling message; adding it as a new event is not supported", e.Method)
	}
	if e.main == nil || e.main.Props.Get("DTSTART") == nil {
		return errors.New("event has no start")
	}
	if strings.EqualFold(text(e.main.Props, "STATUS"), "CANCELLED") {
		return errors.New("cancelled event cannot be added as a new event")
	}
	return nil
}

// ICalendar produces a calendar object for storage, not an iTIP scheduling
// message. Disable server scheduling on the copy while retaining event data.
func (e Event) ICalendar() ([]byte, error) {
	if err := e.CanImport(); err != nil {
		return nil, err
	}
	cal := ical.NewCalendar()
	for key, values := range e.calendar.Props {
		if key != "METHOD" {
			cal.Props[key] = values
		}
	}
	if cal.Props.Get("PRODID") == nil {
		cal.Props.SetText("PRODID", "-//MailSalon//Imported Calendar//EN")
	}
	for _, child := range e.calendar.Children {
		if child.Name != "VEVENT" {
			cal.Children = append(cal.Children, child)
			continue
		}
		copy := ical.NewComponent("VEVENT")
		copy.Children = child.Children
		for key, values := range child.Props {
			copy.Props[key] = append([]ical.Prop(nil), values...)
			if key == "ORGANIZER" || key == "ATTENDEE" {
				for i, p := range values {
					params := ical.Params{}
					for k, v := range p.Params {
						params[k] = append([]string(nil), v...)
					}
					params.Set("SCHEDULE-AGENT", "NONE")
					copy.Props[key][i].Params = params
				}
			}
		}
		if copy.Props.Get("DTSTAMP") == nil {
			copy.Props.SetDateTime("DTSTAMP", time.Now().UTC())
		}
		cal.Children = append(cal.Children, copy)
	}
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
