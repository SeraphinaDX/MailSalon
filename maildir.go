package main

import (
	"fmt"
	"io/fs"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Message struct {
	Path        string
	Filename    string
	From        string
	FromAddr    string
	ReplyTo     string
	Subject     string
	Date        time.Time
	MessageID   string
	References  string
	Body        string
	Attachments []Attachment
	Unread      bool
}

func loadMaildir(root string) ([]Message, error) {
	for _, sub := range []string{"cur", "new", "tmp"} {
		info, err := os.Stat(filepath.Join(root, sub))
		if err != nil {
			return nil, fmt.Errorf("%s is not a Maildir: missing %s/: %w", root, sub, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%s/%s is not a directory", root, sub)
		}
	}

	var messages []Message
	for _, sub := range []string{"new", "cur"} {
		dir := filepath.Join(root, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			msg, err := parseMailFile(filepath.Join(dir, entry.Name()), sub == "new")
			if err != nil {
				// Maildirs can contain odd or partially-written messages. Keep the
				// client usable by skipping a malformed file rather than aborting.
				continue
			}
			messages = append(messages, msg)
		}
	}

	sort.SliceStable(messages, func(i, j int) bool {
		if messages[i].Date.Equal(messages[j].Date) {
			return messages[i].Filename > messages[j].Filename
		}
		return messages[i].Date.After(messages[j].Date)
	})
	return messages, nil
}

func countMaildirMessageFiles(root string) (int, error) {
	total := 0
	for _, sub := range []string{"new", "cur"} {
		dir := filepath.Join(root, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return 0, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				total++
			}
		}
	}
	return total, nil
}

func parseMailFile(path string, inNew bool) (Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return Message{}, err
	}
	defer f.Close()

	m, err := mail.ReadMessage(f)
	if err != nil {
		return Message{}, err
	}

	body, attachments, err := decodeMessageContent(m.Header, m.Body, false)
	if err != nil {
		body = "[Could not decode message body: " + err.Error() + "]"
	}

	date, err := m.Header.Date()
	if err != nil {
		if info, statErr := f.Stat(); statErr == nil {
			date = info.ModTime()
		}
	}

	fromName, fromAddr := firstAddress(decodeHeader(m.Header.Get("From")))
	replyToName, replyToAddr := firstAddress(decodeHeader(m.Header.Get("Reply-To")))
	_ = replyToName
	if replyToAddr == "" {
		replyToAddr = fromAddr
	}

	base := filepath.Base(path)
	return Message{
		Path:        path,
		Filename:    base,
		From:        firstNonEmpty(fromName, fromAddr, decodeHeader(m.Header.Get("From"))),
		FromAddr:    fromAddr,
		ReplyTo:     replyToAddr,
		Subject:     firstNonEmpty(decodeHeader(m.Header.Get("Subject")), "(no subject)"),
		Date:        date,
		MessageID:   strings.TrimSpace(m.Header.Get("Message-ID")),
		References:  strings.TrimSpace(m.Header.Get("References")),
		Body:        strings.TrimSpace(body),
		Attachments: attachments,
		Unread:      inNew || !maildirHasFlag(base, 'S'),
	}, nil
}

func firstAddress(value string) (name, address string) {
	if value == "" {
		return "", ""
	}
	list, err := mail.ParseAddressList(value)
	if err != nil || len(list) == 0 {
		return "", ""
	}
	return list[0].Name, list[0].Address
}

func maildirHasFlag(filename string, flag rune) bool {
	i := strings.LastIndex(filename, ":2,")
	if i < 0 {
		return false
	}
	return strings.ContainsRune(filename[i+3:], flag)
}

func markRead(msg *Message) error {
	if msg == nil || !msg.Unread {
		return nil
	}
	base := msg.Filename
	i := strings.LastIndex(base, ":2,")
	var next string
	if i < 0 {
		next = base + ":2,S"
	} else if !strings.ContainsRune(base[i+3:], 'S') {
		next = base + "S"
	} else {
		next = base
	}

	dest := filepath.Join(filepath.Dir(filepath.Dir(msg.Path)), "cur", next)
	if msg.Path != dest {
		if err := os.Rename(msg.Path, dest); err != nil {
			return err
		}
	}
	msg.Path = dest
	msg.Filename = next
	msg.Unread = false
	return nil
}

func deleteMessage(msg Message) error {
	if strings.TrimSpace(msg.Path) == "" {
		return fmt.Errorf("message has no Maildir path")
	}

	if err := os.Remove(msg.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func walkRegularFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Type().IsRegular() {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
