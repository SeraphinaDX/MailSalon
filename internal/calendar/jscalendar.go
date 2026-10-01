package calendar

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"strings"
	"time"

	ical "github.com/emersion/go-ical"
)

// JSCalendar maps ordinary events to RFC 8984. Complex resources must use a
// CalDAV destination until their semantics can be mapped without data loss.
func (e Event) JSCalendar() ([]byte, error) {
	if err := e.CanImport(); err != nil {
		return nil, err
	}
	if e.Recurring || len(e.main.Children) > 0 {
		return nil, fmt.Errorf("recurrences and alarms require a CalDAV destination; the original attachment can be saved with a")
	}
	locale := ""
	allowed := map[string]bool{}
	for _, key := range strings.Fields("UID SUMMARY DTSTART DTEND DURATION LOCATION DESCRIPTION ORGANIZER ATTENDEE DTSTAMP CREATED LAST-MODIFIED SEQUENCE STATUS TRANSP CLASS URL PRIORITY") {
		allowed[key] = true
	}
	for key, values := range e.main.Props {
		if !allowed[key] {
			return nil, fmt.Errorf("%s cannot yet be imported to JMAP; choose CalDAV to retain the complete event", key)
		}
		if key != "ATTENDEE" && len(values) > 1 {
			return nil, fmt.Errorf("multiple %s properties cannot be mapped to JMAP", key)
		}
		if key != "ATTENDEE" && key != "ORGANIZER" {
			for _, p := range values {
				if lang := p.Params.Get("LANGUAGE"); lang != "" {
					if locale != "" && !strings.EqualFold(locale, lang) {
						return nil, fmt.Errorf("mixed event languages require CalDAV")
					}
					locale = lang
				}
				for param := range p.Params {
					if param != "VALUE" && param != "TZID" && param != "LANGUAGE" {
						return nil, fmt.Errorf("%s parameter %s requires a CalDAV destination", key, param)
					}
				}
			}
		}
	}
	startProp := e.main.Props.Get("DTSTART")
	start, err := startProp.DateTime(nil)
	if err != nil {
		return nil, fmt.Errorf("start timezone/date cannot be mapped to JMAP: %w", err)
	}
	zone := e.Zone
	var tz any
	if zone != "" {
		tz = zone
	}
	end, err := (&ical.Event{Component: e.main}).DateTimeEnd(start.Location())
	if err != nil {
		return nil, err
	}
	if end.Before(start) || (e.main.Props.Get("DTEND") != nil && !end.After(start)) {
		return nil, fmt.Errorf("event end precedes start")
	}
	if e.main.Props.Get("DTEND") != nil && e.main.Props.Get("DURATION") != nil {
		return nil, fmt.Errorf("event cannot have both DTEND and DURATION")
	}
	if endProp := e.main.Props.Get("DTEND"); endProp != nil && (endProp.ValueType() == ical.ValueDate) != e.AllDay {
		return nil, fmt.Errorf("start and end must use the same date type")
	}
	duration := fmt.Sprintf("PT%dS", int64(end.Sub(start)/time.Second))
	if e.AllDay {
		if end.Sub(start)%(24*time.Hour) != 0 {
			return nil, fmt.Errorf("all-day duration must use whole days")
		}
		duration = fmt.Sprintf("P%dD", int64(end.Sub(start)/(24*time.Hour)))
		tz = nil
	}
	v := map[string]any{"@type": "Event", "uid": e.UID, "title": e.Title, "start": start.Format("2006-01-02T15:04:05"), "duration": duration, "timeZone": tz, "showWithoutTime": e.AllDay}
	if locale != "" {
		v["locale"] = locale
	}
	if e.Location != "" {
		v["locations"] = map[string]any{"main": map[string]any{"@type": "Location", "name": e.Location}}
	}
	if e.Description != "" {
		v["description"] = e.Description
		v["descriptionContentType"] = "text/plain"
	}
	for prop, key := range map[string]string{"CREATED": "created", "LAST-MODIFIED": "updated"} {
		if p := e.main.Props.Get(prop); p != nil {
			t, err := p.DateTime(nil)
			if err != nil {
				return nil, err
			}
			v[key] = t.UTC().Format("2006-01-02T15:04:05Z")
		}
	}
	for prop, key := range map[string]string{"SEQUENCE": "sequence", "PRIORITY": "priority"} {
		if p := e.main.Props.Get(prop); p != nil {
			n, err := p.Int()
			if err != nil || n < 0 || (prop == "PRIORITY" && n > 9) {
				return nil, fmt.Errorf("invalid %s", prop)
			}
			v[key] = n
		}
	}
	if p := e.main.Props.Get("STATUS"); p != nil {
		status := strings.ToLower(p.Value)
		if status != "confirmed" && status != "tentative" && status != "cancelled" {
			return nil, fmt.Errorf("unsupported STATUS")
		}
		v["status"] = status
	}
	if p := e.main.Props.Get("TRANSP"); p != nil {
		switch strings.ToUpper(p.Value) {
		case "OPAQUE":
			v["freeBusyStatus"] = "busy"
		case "TRANSPARENT":
			v["freeBusyStatus"] = "free"
		default:
			return nil, fmt.Errorf("unsupported TRANSP")
		}
	}
	if p := e.main.Props.Get("CLASS"); p != nil {
		switch strings.ToUpper(p.Value) {
		case "PUBLIC":
			v["privacy"] = "public"
		case "PRIVATE":
			v["privacy"] = "private"
		case "CONFIDENTIAL":
			v["privacy"] = "secret"
		default:
			return nil, fmt.Errorf("unsupported CLASS")
		}
	}
	if p := e.main.Props.Get("URL"); p != nil {
		v["links"] = map[string]any{"url": map[string]any{"@type": "Link", "href": p.Value}}
	}
	participants := map[string]any{}
	if organizer := e.main.Props.Get("ORGANIZER"); organizer != nil {
		person, err := jsParticipant(organizer, true)
		if err != nil {
			return nil, err
		}
		participants["organizer"] = person
		v["replyTo"] = map[string]string{"imip": organizer.Value}
	}
	for i, p := range e.main.Props.Values("ATTENDEE") {
		person, err := jsParticipant(&p, false)
		if err != nil {
			return nil, err
		}
		if v["replyTo"] != nil {
			person["sendTo"] = map[string]string{"imip": p.Value}
		}
		participants[fmt.Sprintf("attendee%d", i)] = person
	}
	if len(participants) > 0 {
		v["participants"] = participants
	}
	return json.MarshalIndent(v, "", "  ")
}

func jsParticipant(p *ical.Prop, owner bool) (map[string]any, error) {
	if !strings.HasPrefix(strings.ToLower(p.Value), "mailto:") {
		return nil, fmt.Errorf("non-email participants require CalDAV")
	}
	addr, err := mail.ParseAddress(p.Value[7:])
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	for _, key := range strings.Fields("CN ROLE RSVP PARTSTAT CUTYPE LANGUAGE VALUE SCHEDULE-AGENT") {
		allowed[key] = true
	}
	for key := range p.Params {
		if !allowed[key] {
			return nil, fmt.Errorf("participant parameter %s requires CalDAV", key)
		}
	}
	roles := map[string]bool{"attendee": true}
	if owner {
		roles = map[string]bool{"owner": true}
	}
	switch strings.ToUpper(p.Params.Get("ROLE")) {
	case "", "REQ-PARTICIPANT":
	case "OPT-PARTICIPANT":
		roles["optional"] = true
	case "CHAIR":
		roles["chair"] = true
	case "NON-PARTICIPANT":
		delete(roles, "attendee")
		roles["informational"] = true
	default:
		return nil, fmt.Errorf("unsupported participant ROLE")
	}
	v := map[string]any{"@type": "Participant", "email": addr.Address, "roles": roles, "scheduleAgent": "none"}
	if name := p.Params.Get("CN"); name != "" {
		v["name"] = name
	}
	if lang := p.Params.Get("LANGUAGE"); lang != "" {
		v["language"] = lang
	}
	if kind := p.Params.Get("CUTYPE"); kind != "" {
		kind = strings.ToLower(kind)
		if kind == "room" {
			kind = "location"
		}
		if kind != "individual" && kind != "group" && kind != "location" && kind != "resource" && kind != "unknown" {
			return nil, fmt.Errorf("unsupported participant CUTYPE")
		}
		if kind != "unknown" {
			v["kind"] = kind
		}
	}
	if status := p.Params.Get("PARTSTAT"); status != "" {
		status = strings.ToLower(status)
		if status != "needs-action" && status != "accepted" && status != "declined" && status != "tentative" {
			return nil, fmt.Errorf("unsupported participant PARTSTAT")
		}
		v["participationStatus"] = status
	}
	if rsvp := p.Params.Get("RSVP"); rsvp != "" {
		if !strings.EqualFold(rsvp, "TRUE") && !strings.EqualFold(rsvp, "FALSE") {
			return nil, fmt.Errorf("invalid RSVP")
		}
		v["expectReply"] = strings.EqualFold(rsvp, "TRUE")
	}
	return v, nil
}
