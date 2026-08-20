package main

import (
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
)

func normalizeAttachmentPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	if value == "" {
		return "", fmt.Errorf("attachment path is empty")
	}
	path, err := expandPath(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	return path, nil
}

func saveMessageAttachments(msg Message, downloadDir string) ([]string, error) {
	if len(msg.Attachments) == 0 {
		return nil, fmt.Errorf("message has no attachments")
	}
	if strings.TrimSpace(msg.Path) == "" {
		return nil, fmt.Errorf("message has no Maildir path")
	}

	f, err := os.Open(msg.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	parsed, err := mail.ReadMessage(f)
	if err != nil {
		return nil, err
	}
	_, attachments, err := decodeMessageContent(parsed.Header, parsed.Body, true)
	if err != nil {
		return nil, err
	}
	if len(attachments) == 0 {
		return nil, fmt.Errorf("message has no decodable attachments")
	}

	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		return nil, err
	}

	var saved []string
	for i, attachment := range attachments {
		name := safeAttachmentName(attachment.Filename, i+1)
		dest, err := unusedAttachmentPath(downloadDir, name)
		if err != nil {
			return saved, err
		}
		if err := os.WriteFile(dest, attachment.Data, 0o600); err != nil {
			return saved, err
		}
		saved = append(saved, dest)
	}
	return saved, nil
}

func safeAttachmentName(name string, index int) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	name = filepath.Base(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return fmt.Sprintf("attachment-%d", index)
	}
	return name
}

func unusedAttachmentPath(dir, name string) (string, error) {
	candidate := filepath.Join(dir, name)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate, nil
	} else if err != nil {
		return "", err
	}

	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; i < 10000; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("could not choose a free filename for %q", name)
}
