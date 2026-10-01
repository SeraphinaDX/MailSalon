package calendar

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	ical "github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

// Occurrence is a display projection. Source bytes are never rewritten here.
type Occurrence struct {
	Start, End             time.Time
	Title, Location, Notes string
	AllDay, Recurring      bool
}

func (o Occurrence) Intersects(start, end time.Time) bool {
	if !o.End.After(o.Start) {
		return !o.Start.Before(start) && o.Start.Before(end)
	}
	return o.Start.Before(end) && o.End.After(start)
}

// Occurrences projects one stored resource into a bounded, half-open window.
// Timed events use the display location; dates and floating times keep their
// wall-clock meaning. Unsupported resources return an error for the UI to show.
func Occurrences(data []byte, jsonFormat bool, from, to time.Time, loc *time.Location) ([]Occurrence, error) {
	if loc == nil {
		loc = time.Local
	}
	if jsonFormat {
		return jsonOccurrences(data, from, to, loc)
	}
	cal, err := ical.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		return nil, err
	}
	var master *ical.Component
	var exceptions []*ical.Component
	for _, c := range cal.Children {
		if c.Name != "VEVENT" && c.Name != "VTODO" {
			continue
		}
		if c.Props.Get("RECURRENCE-ID") == nil {
			if master != nil {
				return nil, fmt.Errorf("multiple series masters")
			}
			master = c
		} else {
			exceptions = append(exceptions, c)
		}
	}
	if master == nil {
		return nil, fmt.Errorf("no event/dated task master")
	}
	base, err := componentOccurrence(master, loc, nil)
	if err != nil {
		return nil, err
	}
	set := &rrule.Set{}
	set.RDate(base.Start)
	rules := master.Props.Values("RRULE")
	if len(rules) > 1 {
		return nil, fmt.Errorf("multiple RRULEs are unsupported")
	}
	if len(rules) == 1 {
		op, err := rrule.StrToROptionInLocation(rules[0].Value, base.Start.Location())
		if err != nil {
			return nil, err
		}
		op.Dtstart = base.Start
		if err := addRule(set, *op); err != nil {
			return nil, err
		}
	}
	for _, key := range []string{"RDATE", "EXDATE"} {
		for _, p := range master.Props.Values(key) {
			for _, value := range strings.Split(p.Value, ",") {
				p.Value = value
				t, err := p.DateTime(loc)
				if err != nil {
					return nil, err
				}
				if key == "RDATE" {
					set.RDate(t)
				} else {
					set.ExDate(t)
				}
			}
		}
	}
	var overrides []Occurrence
	for _, c := range exceptions {
		id := c.Props.Get("RECURRENCE-ID")
		if id.Params.Get("RANGE") != "" {
			return nil, fmt.Errorf("RANGE recurrence exceptions are unsupported")
		}
		t, err := id.DateTime(loc)
		if err != nil {
			return nil, err
		}
		set.ExDate(t)
		if strings.EqualFold(text(c.Props, "STATUS"), "CANCELLED") {
			continue
		}
		o, err := componentOccurrence(c, loc, &base)
		if err != nil {
			return nil, err
		}
		o.Recurring = true
		overrides = append(overrides, o)
	}
	if strings.EqualFold(text(master.Props, "STATUS"), "CANCELLED") {
		return nil, nil
	}
	base.Recurring = len(rules) != 0 || len(master.Props.Values("RDATE")) != 0 || len(exceptions) != 0
	out, err := expand(set, base, from, to, loc)
	if err != nil {
		return nil, err
	}
	for _, o := range overrides {
		o = displayOccurrence(o, loc)
		if o.Intersects(from, to) {
			out = append(out, o)
		}
	}
	sortOccurrences(out)
	return out, nil
}

func componentOccurrence(c *ical.Component, loc *time.Location, fallback *Occurrence) (Occurrence, error) {
	o := Occurrence{}
	if fallback != nil {
		o = *fallback
	}
	start := c.Props.Get("DTSTART")
	if start == nil && c.Name == "VTODO" {
		start = c.Props.Get("DUE")
	}
	if start == nil {
		return o, fmt.Errorf("event has no start/date")
	}
	t, err := start.DateTime(loc)
	if err != nil {
		return o, err
	}
	o.Start = t
	o.AllDay = len(start.Value) == 8
	o.End = t
	end := c.Props.Get("DTEND")
	if end == nil && c.Name == "VTODO" {
		end = c.Props.Get("DUE")
	}
	if end != nil {
		o.End, err = end.DateTime(loc)
	} else if duration := c.Props.Get("DURATION"); duration != nil {
		var d time.Duration
		d, err = duration.Duration()
		o.End = t.Add(d)
		if o.AllDay {
			if d%(24*time.Hour) != 0 {
				return o, fmt.Errorf("all-day duration is not whole days")
			}
			o.End = t.AddDate(0, 0, int(d/(24*time.Hour)))
		}
	} else if fallback != nil {
		o.End = occurrenceEnd(t, *fallback)
	} else if o.AllDay {
		o.End = t.AddDate(0, 0, 1)
	}
	if err != nil {
		return o, err
	}
	if o.End.Before(o.Start) {
		return o, fmt.Errorf("event ends before it starts")
	}
	for key, value := range map[string]*string{"SUMMARY": &o.Title, "LOCATION": &o.Location, "DESCRIPTION": &o.Notes} {
		if c.Props.Get(key) != nil {
			*value = text(c.Props, key)
		}
	}
	if o.Title == "" {
		o.Title = text(c.Props, "UID")
	}
	return o, nil
}

func addRule(set *rrule.Set, op rrule.ROption) error {
	// Avoid subdaily rules and huge time combinations blocking the UI. Source
	// remains available in Agenda's stored-items fallback.
	if op.Freq > rrule.DAILY || len(op.Byhour) > 1 || len(op.Byminute) > 1 || len(op.Bysecond) > 1 {
		return fmt.Errorf("subdaily or multiple-times-per-day recurrence is unsupported")
	}
	r, err := rrule.NewRRule(op)
	if err == nil {
		set.RRule(r)
	}
	return err
}

func occurrenceEnd(start time.Time, base Occurrence) time.Time {
	if base.AllDay {
		// Calendar days, not 24-hour intervals, across DST boundaries.
		y, m, d := base.Start.Date()
		ey, em, ed := base.End.Date()
		days := int(time.Date(ey, em, ed, 0, 0, 0, 0, time.UTC).Sub(time.Date(y, m, d, 0, 0, 0, 0, time.UTC)).Hours() / 24)
		return start.AddDate(0, 0, days)
	}
	return start.Add(base.End.Sub(base.Start))
}

func displayOccurrence(o Occurrence, loc *time.Location) Occurrence {
	if o.AllDay {
		o.Start = wallDate(o.Start, loc)
		o.End = wallDate(o.End, loc)
	} else {
		o.Start = o.Start.In(loc)
		o.End = o.End.In(loc)
	}
	return o
}

func wallDate(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func expand(set *rrule.Set, base Occurrence, from, to time.Time, loc *time.Location) ([]Occurrence, error) {
	var out []Occurrence
	next := set.Iterator()
	for n := 0; n < 50000; n++ {
		t, ok := next()
		if !ok || !t.Before(to) {
			sortOccurrences(out)
			return out, nil
		}
		o := base
		o.Start = t
		o.End = occurrenceEnd(t, base)
		o = displayOccurrence(o, loc)
		if o.Intersects(from, to) {
			out = append(out, o)
		}
		if len(out) > 4096 {
			return nil, fmt.Errorf("too many occurrences in this window")
		}
	}
	return nil, fmt.Errorf("recurrence exceeds the 50,000-occurrence expansion limit")
}

func sortOccurrences(out []Occurrence) {
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Start.Equal(out[j].Start) {
			return out[i].Title < out[j].Title
		}
		return out[i].Start.Before(out[j].Start)
	})
}
