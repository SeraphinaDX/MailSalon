package uiapp

import (
	"errors"
	"fmt"
	"net/mail"
	"sort"
	"strings"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/pim"
)

// Account books precede shared books, both for deduplication and for choosing
// where a reply sender is saved. Another account's private books are excluded.
func (a *App) contactCollections(account int) []config.Collection {
	if account < 0 || account >= len(a.cfg.Accounts) {
		return nil
	}
	name := a.cfg.Accounts[account].Name
	books := []config.Collection{}
	for _, shared := range []bool{false, true} {
		for _, c := range a.cfg.Collections {
			if pim.Contacts(c) && (c.Account == "") == shared && (shared || strings.EqualFold(c.Account, name)) {
				books = append(books, c)
			}
		}
	}
	return books
}

func (a *App) loadContactAddresses(account int) ([]mail.Address, error) {
	addresses := []mail.Address{}
	seen := map[string]bool{}
	var errs []error
	for _, c := range a.contactCollections(account) {
		items, err := pim.Load(c)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", c.Name, err))
			continue
		}
		for _, item := range items {
			for _, addr := range pim.ContactAddresses(item) {
				key := strings.ToLower(addr.Address)
				if !seen[key] {
					addresses = append(addresses, addr)
					seen[key] = true
				}
			}
		}
	}
	sort.SliceStable(addresses, func(i, j int) bool {
		return strings.ToLower(addresses[i].String()) < strings.ToLower(addresses[j].String())
	})
	return addresses, errors.Join(errs...)
}

func (a *App) saveReplyContact(raw string, account int) string {
	if !a.cfg.AutoAddReplyContacts {
		return ""
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil {
		return "Contact not saved: sender address is invalid"
	}
	books := a.contactCollections(account)
	if len(books) == 0 {
		return "Contact not saved: configure an address book"
	}
	addresses, err := a.loadContactAddresses(account)
	if err != nil {
		return "Contact not saved: " + err.Error()
	}
	for _, existing := range addresses {
		if strings.EqualFold(existing.Address, addr.Address) {
			return ""
		}
	}
	added, err := pim.EnsureContact(books[0], *addr)
	if err != nil {
		return "Contact not saved: " + err.Error()
	}
	if added {
		return "Saved contact in " + books[0].Name + "; sync to upload"
	}
	return ""
}
