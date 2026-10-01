package pim

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/calendar"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
)

// ImportCalendar deduplicates UID under the sync lock. It never replaces a
// tracked event; updates and cancellations need an explicit scheduling workflow.
func ImportCalendar(c config.Collection, event calendar.Event) (bool, error) {
	var data []byte
	var err error
	switch c.Protocol {
	case "caldav":
		data, err = event.ICalendar()
	case "jmap-calendars":
		data, err = event.JSCalendar()
	default:
		return false, fmt.Errorf("destination is not a calendar")
	}
	if err != nil {
		return false, err
	}
	if _, err := Parse(c, data); err != nil {
		return false, err
	}
	release, err := lock(c)
	if err != nil {
		return false, err
	}
	defer release()
	items, err := Load(c)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.UID == event.UID {
			return false, nil
		}
	}
	// Incoming UIDs may contain slashes or other filename characters. Hash only
	// the filename; retain the original UID inside the native calendar object.
	sum := sha256.Sum256([]byte(event.UID))
	path := filepath.Join(c.LocalDir, fmt.Sprintf("import-%x%s", sum, Extension(c)))
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return false, fmt.Errorf("import destination already exists")
	}
	if err := write(path, data); err != nil {
		return false, err
	}
	return true, nil
}
