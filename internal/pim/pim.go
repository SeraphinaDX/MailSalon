// Package pim reads the native local files written by MailSalonSync. Preview
// extraction never rewrites the source; advanced editing retains every field.
package pim

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
)

type Item struct {
	Path                                                        string
	Data                                                        []byte
	UID, Title, Email, Phone, Start, End, Zone, Location, Notes string
	Recurring                                                   bool
}

func Contacts(c config.Collection) bool {
	return c.Protocol == "carddav" || c.Protocol == "jmap-contacts"
}
func Extension(c config.Collection) string {
	switch c.Protocol {
	case "carddav":
		return ".vcf"
	case "caldav":
		return ".ics"
	default:
		return ".json"
	}
}
func str(v any) string { s, _ := v.(string); return s }
func first(v any) map[string]any {
	m, _ := v.(map[string]any)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if x, ok := m[k].(map[string]any); ok {
			return x
		}
	}
	return nil
}

func Parse(c config.Collection, data []byte) (Item, error) {
	i := Item{Data: data}
	if Extension(c) == ".json" {
		var v map[string]any
		if err := json.Unmarshal(data, &v); err != nil {
			return i, err
		}
		i.UID = str(v["uid"])
		if Contacts(c) {
			if v["@type"] != "Card" || str(v["version"]) == "" {
				return i, errors.New("JSContact needs @type Card and version")
			}
			name, _ := v["name"].(map[string]any)
			i.Title = str(name["full"])
			if i.Title == "" {
				if parts, ok := name["components"].([]any); ok {
					for _, part := range parts {
						if p, ok := part.(map[string]any); ok {
							i.Title += str(p["value"]) + " "
						}
					}
					i.Title = strings.TrimSpace(i.Title)
				}
			}
			i.Email = str(first(v["emails"])["address"])
			i.Phone = str(first(v["phones"])["number"])
			i.Notes = str(first(v["notes"])["note"])
		} else {
			if v["@type"] != "Event" {
				return i, errors.New("JSCalendar needs @type Event")
			}
			i.Title = str(v["title"])
			i.Start = str(v["start"])
			i.End = str(v["duration"])
			i.Zone = str(v["timeZone"])
			i.Location = str(first(v["locations"])["name"])
			i.Notes = str(v["description"])
			i.Recurring = v["recurrenceRules"] != nil || v["recurrenceOverrides"] != nil
		}
	} else {
		lines := unfold(data)
		outer := "VCARD"
		if !Contacts(c) {
			outer = "VCALENDAR"
		}
		if len(lines) < 3 || strings.ToUpper(lines[0]) != "BEGIN:"+outer || strings.ToUpper(lines[len(lines)-1]) != "END:"+outer {
			return i, fmt.Errorf("expected one %s", outer)
		}
		stack := []string{}
		outerCount := 0
		firstEvent := false
		inMain := Contacts(c)
		for _, line := range lines {
			head, value, ok := strings.Cut(line, ":")
			if !ok {
				return i, errors.New("malformed content line")
			}
			key := strings.ToUpper(strings.Split(head, ";")[0])
			if p := strings.LastIndex(key, "."); p >= 0 {
				key = key[p+1:]
			}
			if key == "BEGIN" {
				if len(stack) == 0 {
					outerCount++
					if outerCount != 1 || strings.ToUpper(value) != outer {
						return i, errors.New("expected a single outer container")
					}
				}
				stack = append(stack, strings.ToUpper(value))
				if (strings.EqualFold(value, "VEVENT") || strings.EqualFold(value, "VTODO")) && !firstEvent {
					firstEvent = true
					inMain = true
				}
				continue
			}
			if key == "END" {
				if len(stack) == 0 || stack[len(stack)-1] != strings.ToUpper(value) {
					return i, errors.New("unbalanced content containers")
				}
				if strings.EqualFold(value, "VEVENT") || strings.EqualFold(value, "VTODO") {
					inMain = false
				}
				stack = stack[:len(stack)-1]
				continue
			}
			if len(stack) == 0 {
				return i, errors.New("property outside container")
			}
			top := stack[len(stack)-1]
			if top != "VCARD" && top != "VEVENT" && top != "VTODO" {
				continue
			}
			if key == "UID" {
				if i.UID != "" && i.UID != value {
					return i, errors.New("split multiple UIDs into individual files")
				}
				i.UID = value
			}
			if !inMain {
				continue
			}
			switch key {
			case "FN", "SUMMARY":
				if i.Title == "" {
					i.Title = unescape(value)
				}
			case "EMAIL":
				if i.Email == "" {
					i.Email = unescape(value)
				}
			case "TEL":
				if i.Phone == "" {
					i.Phone = unescape(value)
				}
			case "DTSTART":
				i.Start = readableDate(value)
				for _, p := range strings.Split(head, ";")[1:] {
					if strings.HasPrefix(strings.ToUpper(p), "TZID=") {
						i.Zone = strings.Trim(p[5:], "\"")
					}
				}
				if strings.HasSuffix(value, "Z") {
					i.Zone = "UTC"
				}
			case "DTEND", "DUE":
				i.End = readableDate(value)
			case "DURATION":
				i.End = value
			case "LOCATION":
				i.Location = unescape(value)
			case "NOTE", "DESCRIPTION":
				i.Notes = unescape(value)
			case "RRULE", "RDATE":
				i.Recurring = true
			}
		}
		if len(stack) != 0 {
			return i, errors.New("unclosed content containers")
		}
	}
	if i.UID == "" {
		return i, errors.New("item needs a UID")
	}
	if i.Title == "" {
		i.Title = i.UID
	}
	return i, nil
}

func unfold(data []byte) []string {
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n ", ""), "\n\t", "")
	return strings.Split(strings.TrimSpace(s), "\n")
}
func unescape(s string) string {
	return strings.NewReplacer("\\n", "\n", "\\N", "\n", "\\,", ",", "\\;", ";", "\\\\", "\\").Replace(s)
}
func escape(s string) string {
	return strings.NewReplacer("\\", "\\\\", "\r\n", "\\n", "\n", "\\n", ";", "\\;", ",", "\\,").Replace(s)
}
func readableDate(s string) string {
	for _, layout := range []string{"20060102T150405Z", "20060102T150405", "20060102"} {
		if t, err := time.Parse(layout, s); err == nil {
			if layout == "20060102" {
				return t.Format("2006-01-02")
			}
			return t.Format("2006-01-02T15:04:05")
		}
	}
	return s
}

func Load(c config.Collection) ([]Item, error) {
	files, err := os.ReadDir(c.LocalDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items := []Item{}
	for _, f := range files {
		if strings.HasPrefix(f.Name(), ".") || filepath.Ext(f.Name()) != Extension(c) {
			continue
		}
		if !f.Type().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file", f.Name())
		}
		path := filepath.Join(c.LocalDir, f.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		i, err := Parse(c, data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name(), err)
		}
		i.Path = path
		items = append(items, i)
	}
	sort.SliceStable(items, func(a, b int) bool {
		if !Contacts(c) && items[a].Start != items[b].Start {
			return items[a].Start < items[b].Start
		}
		return strings.ToLower(items[a].Title) < strings.ToLower(items[b].Title)
	})
	return items, nil
}

// New creates an ordinary contact or single event. Start/end accept local ISO
// datetimes or dates; calendar JSON stores duration instead of an end datetime.
func New(c config.Collection, fields []string) ([]byte, string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, "", err
	}
	uid := hex.EncodeToString(random[:]) + "@mailsalon"
	if len(fields) < 3 || strings.TrimSpace(fields[0]) == "" {
		return nil, "", errors.New("name/title is required")
	}
	if Contacts(c) {
		if fields[1] != "" {
			addr, err := mail.ParseAddress(fields[1])
			if err != nil {
				return nil, "", errors.New("invalid email address")
			}
			fields[1] = addr.Address
		}
		if Extension(c) == ".vcf" {
			lines := []string{"BEGIN:VCARD", "VERSION:4.0", "UID:" + uid, "FN:" + escape(fields[0])}
			if fields[1] != "" {
				lines = append(lines, "EMAIL:"+escape(fields[1]))
			}
			if fields[2] != "" {
				lines = append(lines, "TEL;VALUE=text:"+escape(fields[2]))
			}
			lines = append(lines, "END:VCARD")
			return serialize(lines), uid, nil
		}
		v := map[string]any{"@type": "Card", "version": "1.0", "uid": uid, "name": map[string]any{"full": fields[0]}}
		if fields[1] != "" {
			v["emails"] = map[string]any{"primary": map[string]any{"address": fields[1]}}
		}
		if fields[2] != "" {
			v["phones"] = map[string]any{"primary": map[string]any{"number": fields[2]}}
		}
		data, err := json.MarshalIndent(v, "", "  ")
		return data, uid, err
	}
	if len(fields) < 5 {
		return nil, "", errors.New("event needs title, start, end, timezone and location fields")
	}
	allDay := len(fields[1]) == 10
	layout := "2006-01-02T15:04:05"
	if allDay {
		layout = "2006-01-02"
	}
	zone := time.UTC
	if !allDay && fields[3] != "" {
		var err error
		zone, err = time.LoadLocation(fields[3])
		if err != nil {
			return nil, "", errors.New("invalid IANA timezone")
		}
	}
	start, err := time.ParseInLocation(layout, fields[1], zone)
	if err != nil {
		return nil, "", errors.New("start must be YYYY-MM-DDTHH:MM:SS or YYYY-MM-DD")
	}
	end, err := time.ParseInLocation(layout, fields[2], zone)
	if err != nil || !end.After(start) {
		return nil, "", errors.New("end must be after start in the same date format (all-day end is exclusive)")
	}
	if Extension(c) == ".ics" {
		key := ""
		startValue, endValue := start.UTC().Format("20060102T150405Z"), end.UTC().Format("20060102T150405Z")
		if allDay {
			key = ";VALUE=DATE"
			startValue = start.Format("20060102")
			endValue = end.Format("20060102")
		} else if fields[3] == "" {
			startValue = start.Format("20060102T150405")
			endValue = end.Format("20060102T150405")
		}
		lines := []string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//MailSalon//Calendar//EN", "BEGIN:VEVENT", "UID:" + uid, "DTSTAMP:" + time.Now().UTC().Format("20060102T150405Z"), "SUMMARY:" + escape(fields[0]), "DTSTART" + key + ":" + startValue, "DTEND" + key + ":" + endValue}
		if fields[4] != "" {
			lines = append(lines, "LOCATION:"+escape(fields[4]))
		}
		lines = append(lines, "END:VEVENT", "END:VCALENDAR")
		return serialize(lines), uid, nil
	}
	var tz any
	if !allDay && fields[3] != "" {
		tz = fields[3]
	}
	seconds := int64(end.Sub(start) / time.Second)
	duration := fmt.Sprintf("PT%dS", seconds)
	if allDay {
		duration = fmt.Sprintf("P%dD", seconds/86400)
	}
	v := map[string]any{"@type": "Event", "uid": uid, "title": fields[0], "start": start.Format("2006-01-02T15:04:05"), "duration": duration, "timeZone": tz, "showWithoutTime": allDay}
	if fields[4] != "" {
		v["locations"] = map[string]any{"primary": map[string]any{"@type": "Location", "name": fields[4]}}
	}
	data, err := json.MarshalIndent(v, "", "  ")
	return data, uid, err
}

func serialize(lines []string) []byte {
	var out strings.Builder
	for _, line := range lines {
		limit := 75
		for len(line) > limit {
			n := limit
			for !utf8.RuneStart(line[n]) {
				n--
			}
			out.WriteString(line[:n] + "\r\n ")
			line = line[n:]
			limit = 74
		}
		out.WriteString(line + "\r\n")
	}
	return []byte(out.String())
}

func lock(c config.Collection) (func(), error) {
	if err := os.MkdirAll(c.LocalDir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(c.LocalDir, ".mss-lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, errors.New("collection is busy syncing/editing; retry when it finishes")
	}
	f.Close()
	return func() { os.Remove(path) }, nil
}
func write(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".mss-tmp-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Save checks the original bytes under the shared collection lock, preventing a
// stale editor from overwriting a change downloaded by background sync.
func Save(c config.Collection, original *Item, data []byte, uid string) error {
	release, err := lock(c)
	if err != nil {
		return err
	}
	defer release()
	return saveLocked(c, original, data, uid)
}

func saveLocked(c config.Collection, original *Item, data []byte, uid string) error {
	i, err := Parse(c, data)
	if err != nil {
		return err
	}
	path := filepath.Join(c.LocalDir, uid+Extension(c))
	if original != nil {
		if i.UID != original.UID {
			return errors.New("changing a tracked UID is not supported")
		}
		path = original.Path
		current, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(current, original.Data) {
			return errors.New("item changed since editor opened; reopen it before saving")
		}
	} else {
		if i.UID != uid || filepath.Base(uid) != uid {
			return errors.New("invalid new item UID")
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return errors.New("new item path already exists")
		}
	}
	return write(path, data)
}

func Delete(c config.Collection, item Item) error {
	release, err := lock(c)
	if err != nil {
		return err
	}
	defer release()
	current, err := os.ReadFile(item.Path)
	if err != nil || !bytes.Equal(current, item.Data) {
		return errors.New("item changed; refresh before deleting")
	}
	dir := filepath.Join(c.LocalDir, ".mss-trash")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	// Include a timestamp to retain multiple deleted versions of the same item.
	if err := write(filepath.Join(dir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), filepath.Base(item.Path))), current); err != nil {
		return err
	}
	return os.Remove(item.Path)
}
