package pim

import (
	"net/mail"
	"os"
	"path/filepath"
	"testing"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
)

func TestAllContactAddressesAndEnsureDeduplicates(t *testing.T) {
	for _, protocol := range []string{"carddav", "jmap-contacts"} {
		t.Run(protocol, func(t *testing.T) {
			c := config.Collection{Protocol: protocol, LocalDir: t.TempDir()}
			data := []byte("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:existing\r\nFN:Alice Example\r\nEMAIL:alice@example.test\r\nitem1.EMAIL;TYPE=work:SECOND@example.test\r\nEMAIL:alice@example.test\r\nEMAIL:invalid address\r\nEND:VCARD\r\n")
			if protocol == "jmap-contacts" {
				data = []byte(`{"@type":"Card","version":"1.0","uid":"existing","name":{"full":"Alice Example"},"emails":{"a":{"address":"alice@example.test"},"b":{"address":"SECOND@example.test"},"c":{"address":"invalid address"}}}`)
			}
			if err := Save(c, nil, data, "existing"); err != nil {
				t.Fatal(err)
			}
			item, err := Parse(c, data)
			if err != nil {
				t.Fatal(err)
			}
			addresses := ContactAddresses(item)
			if len(addresses) != 2 || addresses[1].Address != "SECOND@example.test" {
				t.Fatalf("addresses=%v", addresses)
			}
			added, err := EnsureContact(c, mail.Address{Name: "New name", Address: "second@EXAMPLE.test"})
			if err != nil || added {
				t.Fatalf("duplicate added=%v err=%v", added, err)
			}
			current, _ := os.ReadFile(filepath.Join(c.LocalDir, "existing"+Extension(c)))
			if string(current) != string(data) {
				t.Fatal("existing source changed")
			}
			added, err = EnsureContact(c, mail.Address{Name: "Bob, Example", Address: "bob@example.test"})
			if err != nil || !added {
				t.Fatalf("new added=%v err=%v", added, err)
			}
			added, err = EnsureContact(c, mail.Address{Address: "BOB@example.test"})
			if err != nil || added {
				t.Fatal("repeated ensure duplicated contact")
			}
			items, err := Load(c)
			if err != nil || len(items) != 2 {
				t.Fatalf("items=%v err=%v", items, err)
			}
			if err := os.WriteFile(filepath.Join(c.LocalDir, ".mss-lock"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := EnsureContact(c, mail.Address{Address: "busy@example.test"}); err == nil {
				t.Fatal("ignored sync lock")
			}
		})
	}
}

func TestEnsureContactUnnamedAndInvalid(t *testing.T) {
	c := config.Collection{Protocol: "carddav", LocalDir: t.TempDir()}
	if added, err := EnsureContact(c, mail.Address{Address: "person@example.test"}); err != nil || !added {
		t.Fatalf("unnamed contact: %v %v", added, err)
	}
	items, _ := Load(c)
	if items[0].Title != "person@example.test" {
		t.Fatal("missing name fallback")
	}
	if _, err := EnsureContact(c, mail.Address{Address: "invalid"}); err == nil {
		t.Fatal("invalid email accepted")
	}
	if _, err := EnsureContact(config.Collection{Protocol: "caldav"}, mail.Address{Address: "person@example.test"}); err == nil {
		t.Fatal("saved contact into calendar")
	}
}
