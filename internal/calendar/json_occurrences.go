package calendar

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	ical "github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

func jsonOccurrence(v map[string]any, loc *time.Location) (Occurrence, error) {
	get := func(key string) string { s, _ := v[key].(string); return s }
	o := Occurrence{Title: get("title"), Notes: get("description")}
	o.AllDay, _ = v["showWithoutTime"].(bool)
	zone := loc
	if name := get("timeZone"); name != "" && !o.AllDay {
		var err error
		zone, err = time.LoadLocation(name)
		if err != nil {
			return o, err
		}
	}
	var err error
	o.Start, err = time.ParseInLocation("2006-01-02T15:04:05", get("start"), zone)
	if err != nil {
		return o, err
	}
	p := ical.NewProp("DURATION")
	p.Value = get("duration")
	if _, ok := v["duration"]; !ok {
		p.Value = "PT0S"
	}
	duration, err := p.Duration()
	if err != nil || duration < 0 {
		return o, fmt.Errorf("invalid JSCalendar duration")
	}
	if o.AllDay {
		if duration%(24*time.Hour) != 0 {
			return o, fmt.Errorf("all-day duration is not whole days")
		}
		o.Start = wallDate(o.Start, loc)
		o.End = o.Start.AddDate(0, 0, int(duration/(24*time.Hour)))
	} else {
		o.End = o.Start.Add(duration)
	}
	locations, _ := v["locations"].(map[string]any)
	keys := make([]string, 0, len(locations))
	for k := range locations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if l, ok := locations[k].(map[string]any); ok {
			o.Location, _ = l["name"].(string)
			if o.Location != "" {
				break
			}
		}
	}
	if o.Title == "" {
		o.Title = get("uid")
	}
	return o, nil
}

func jsonOccurrences(data []byte, from, to time.Time, loc *time.Location) ([]Occurrence, error) {
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	if v["@type"] != "Event" {
		return nil, fmt.Errorf("expected JSCalendar Event")
	}
	base, err := jsonOccurrence(v, loc)
	if err != nil {
		return nil, err
	}
	if v["status"] == "cancelled" {
		return nil, nil
	}
	set := &rrule.Set{}
	set.RDate(base.Start)
	rules, _ := v["recurrenceRules"].([]any)
	if len(rules) > 1 {
		return nil, fmt.Errorf("multiple recurrenceRules are unsupported")
	}
	if ex, _ := v["excludedRecurrenceRules"].([]any); len(ex) > 0 {
		return nil, fmt.Errorf("excludedRecurrenceRules are unsupported")
	}
	for _, raw := range rules {
		rule, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid recurrenceRules")
		}
		op, err := jsonRule(rule, base.Start)
		if err != nil {
			return nil, err
		}
		if err := addRule(set, op); err != nil {
			return nil, err
		}
	}
	var overrides []Occurrence
	patches, _ := v["recurrenceOverrides"].(map[string]any)
	for key, raw := range patches {
		t, err := time.ParseInLocation("2006-01-02T15:04:05", key, base.Start.Location())
		if err != nil {
			return nil, err
		}
		patch, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid recurrence override")
		}
		set.ExDate(t)
		if patch["excluded"] == true {
			continue
		}
		copy := map[string]any{}
		for k, value := range v {
			copy[k] = value
		}
		copy["start"] = key
		for k, value := range patch {
			switch k {
			case "start", "duration", "title", "description", "timeZone", "showWithoutTime", "locations", "status":
				copy[k] = value
			case "excluded":
			default:
				return nil, fmt.Errorf("unsupported recurrence override property %s", k)
			}
		}
		if copy["status"] == "cancelled" {
			continue
		}
		o, err := jsonOccurrence(copy, loc)
		if err != nil {
			return nil, err
		}
		o.Recurring = true
		overrides = append(overrides, o)
	}
	base.Recurring = len(rules) > 0 || len(patches) > 0
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

func jsonRule(v map[string]any, start time.Time) (rrule.ROption, error) {
	parts := []string{}
	for k, raw := range v {
		switch k {
		case "@type":
		case "frequency", "firstDayOfWeek":
			value, ok := raw.(string)
			if !ok {
				return rrule.ROption{}, fmt.Errorf("invalid recurrence %s", k)
			}
			name := "FREQ"
			if k == "firstDayOfWeek" {
				name = "WKST"
			}
			parts = append(parts, name+"="+strings.ToUpper(value))
		case "interval", "count":
			n, ok := raw.(float64)
			if !ok || n < 1 || n != float64(int(n)) {
				return rrule.ROption{}, fmt.Errorf("invalid recurrence %s", k)
			}
			parts = append(parts, strings.ToUpper(k)+"="+strconv.Itoa(int(n)))
		case "until":
			s, _ := raw.(string)
			t, err := time.ParseInLocation("2006-01-02T15:04:05", s, start.Location())
			if err != nil {
				return rrule.ROption{}, err
			}
			parts = append(parts, "UNTIL="+t.Format("20060102T150405"))
		case "byDay":
			list, ok := raw.([]any)
			if !ok {
				return rrule.ROption{}, fmt.Errorf("invalid byDay")
			}
			days := []string{}
			for _, raw := range list {
				day, ok := raw.(map[string]any)
				if !ok {
					return rrule.ROption{}, fmt.Errorf("invalid byDay")
				}
				name, _ := day["day"].(string)
				prefix := ""
				if n, ok := day["nthOfPeriod"].(float64); ok {
					prefix = strconv.Itoa(int(n))
				}
				days = append(days, prefix+strings.ToUpper(name))
			}
			parts = append(parts, "BYDAY="+strings.Join(days, ","))
		case "byMonth", "byMonthDay", "byYearDay", "byWeekNo", "byHour", "byMinute", "bySecond", "bySetPosition":
			list, ok := raw.([]any)
			if !ok {
				return rrule.ROption{}, fmt.Errorf("invalid %s", k)
			}
			values := []string{}
			for _, x := range list {
				switch n := x.(type) {
				case string:
					if _, err := strconv.Atoi(n); err != nil {
						return rrule.ROption{}, fmt.Errorf("unsupported recurrence month %s", n)
					}
					values = append(values, n)
				case float64:
					if n != float64(int(n)) {
						return rrule.ROption{}, fmt.Errorf("invalid recurrence %s", k)
					}
					values = append(values, strconv.Itoa(int(n)))
				default:
					return rrule.ROption{}, fmt.Errorf("invalid recurrence %s", k)
				}
			}
			name := strings.ToUpper(k)
			if k == "bySetPosition" {
				name = "BYSETPOS"
			}
			parts = append(parts, name+"="+strings.Join(values, ","))
		default:
			return rrule.ROption{}, fmt.Errorf("unsupported recurrence rule property %s", k)
		}
	}
	option, err := rrule.StrToROptionInLocation(strings.Join(parts, ";"), start.Location())
	if err != nil {
		return rrule.ROption{}, err
	}
	option.Dtstart = start
	return *option, nil
}
