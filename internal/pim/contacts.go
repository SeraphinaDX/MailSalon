package pim

import (
	"encoding/json"
	"errors"
	"net/mail"
	"sort"
	"strings"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
)

// ContactAddresses reads every email, not just the first one shown in previews.
// Invalid email properties are skipped; the original contact is never modified.
func ContactAddresses(item Item) []mail.Address {
	values := []string{}
	if strings.HasPrefix(strings.TrimSpace(string(item.Data)), "{") {
		var card map[string]any
		if json.Unmarshal(item.Data, &card) != nil {
			return nil
		}
		emails, _ := card["emails"].(map[string]any)
		keys := make([]string, 0, len(emails))
		for k := range emails {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if email, ok := emails[k].(map[string]any); ok {
				values = append(values, str(email["address"]))
			}
		}
	} else {
		for _, line := range unfold(item.Data) {
			head, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			key := strings.ToUpper(strings.Split(head, ";")[0])
			if p := strings.LastIndex(key, "."); p >= 0 {
				key = key[p+1:]
			}
			if key == "EMAIL" {
				values = append(values, unescape(value))
			}
		}
	}
	seen := map[string]bool{}
	addresses := []mail.Address{}
	for _, value := range values {
		addr, err := mail.ParseAddress(strings.TrimSpace(value))
		if err != nil || seen[strings.ToLower(addr.Address)] {
			continue
		}
		seen[strings.ToLower(addr.Address)] = true
		addresses = append(addresses, mail.Address{Name: item.Title, Address: addr.Address})
	}
	return addresses
}

// EnsureContact checks again under the same lock used by sync and editing.
// Existing source data is untouched; new contacts use the collection's format.
func EnsureContact(c config.Collection, addr mail.Address) (bool, error) {
	if !Contacts(c) {
		return false, errors.New("destination is not an address book")
	}
	parsed, err := mail.ParseAddress(addr.Address)
	if err != nil {
		return false, err
	}
	addr.Address = parsed.Address
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
		for _, existing := range ContactAddresses(item) {
			if strings.EqualFold(existing.Address, addr.Address) {
				return false, nil
			}
		}
	}
	name := strings.TrimSpace(addr.Name)
	if name == "" {
		name = addr.Address
	}
	data, uid, err := New(c, []string{name, addr.Address, ""})
	if err != nil {
		return false, err
	}
	if err := saveLocked(c, nil, data, uid); err != nil {
		return false, err
	}
	return true, nil
}
