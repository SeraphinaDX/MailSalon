package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMultipleAccountsTOML(t *testing.T) {
	dir := t.TempDir()
	sig := filepath.Join(dir, "signature.txt")
	if err := os.WriteFile(sig, []byte("Example signature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.toml")
	data := `
[[accounts]]
name = "personal"
maildir = "~/Maildir"
from = "Example User <user@example.com>"
signature_file = "` + sig + `"
trash_folder = "Trash"
download_dir = "~/Downloads/MailSalon"
receive = "MailSalonSync -plain sync"
send = "MailSalonSync -plain jmap-send -account personal-jmap"

[[accounts]]
name = "work"
maildir = "~/Maildir-work"
from = "Example User <user@work.example>"
receive = "mbsync work"
send = "msmtp -a work -t"

[options]
default_account = "work"
startup_sync = true
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(cfg.Accounts))
	}
	if cfg.DefaultAccountIndex() != 1 {
		t.Fatalf("default account index = %d", cfg.DefaultAccountIndex())
	}
	if cfg.Accounts[0].SendCommand != "MailSalonSync -plain jmap-send -account personal-jmap" {
		t.Fatalf("send command not loaded: %#v", cfg.Accounts[0])
	}
	if !cfg.StartupSync {
		t.Fatal("startup_sync was not loaded")
	}
	if !strings.HasSuffix(cfg.Accounts[1].Maildir, filepath.Join("Maildir-work")) {
		t.Fatalf("maildir was not expanded: %q", cfg.Accounts[1].Maildir)
	}
	s, err := ReadSignature(cfg.Accounts[0])
	if err != nil || s != "Example signature" {
		t.Fatalf("signature = %q, err=%v", s, err)
	}
}

func TestLoadLegacySingleAccountTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	data := `
[mail]
maildir = "~/TestMaildir"
trash_folder = "Trash"
download_dir = "~/Downloads/MailSalon"

[identity]
from = "Example User <user@example.com>"

[commands]
receive = "offlineimap"
send = "msmtp -t"
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Accounts) != 1 || cfg.Accounts[0].From != "Example User <user@example.com>" {
		t.Fatalf("legacy config conversion failed: %#v", cfg)
	}
}

func TestLoadRejectsUnknownTOMLKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	data := `
[[accounts]]
name = "test"
maildir = "~/Maildir"
made_up_setting = true
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected unknown TOML field to fail")
	}
}

func TestLoadAccountGPGTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	data := `
[[accounts]]
name = "personal"
maildir = "~/Maildir"
from = "Alice Example <alice@example.com>"

[accounts.gpg]
enabled = true
command = "gpg2"
homedir = "~/.gnupg-mail"
sign_key = "alice@example.com"
auto_sign = true
auto_encrypt = false
encrypt_to_self = false
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	g := cfg.Accounts[0].GPG
	if !g.Enabled || !g.AutoSign || g.AutoEncrypt || g.EncryptToSelf {
		t.Fatalf("unexpected GPG booleans: %#v", g)
	}
	if g.Command != "gpg2" || g.SignKey != "alice@example.com" {
		t.Fatalf("unexpected GPG settings: %#v", g)
	}
	if !strings.HasSuffix(g.HomeDir, filepath.Join(".gnupg-mail")) {
		t.Fatalf("GPG homedir was not expanded: %q", g.HomeDir)
	}
}

func TestLoadGPGDefaultsEncryptToSelf(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	data := `
[[accounts]]
name = "personal"
maildir = "~/Maildir"

[accounts.gpg]
enabled = true
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Accounts[0].GPG.Command != "gpg" || !cfg.Accounts[0].GPG.EncryptToSelf {
		t.Fatalf("unexpected GPG defaults: %#v", cfg.Accounts[0].GPG)
	}
}

func TestLoadThemeTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	data := `
[[accounts]]
name = "personal"
maildir = "~/Maildir"

[theme]
background = "#090d16"
title = "hotpink"
active_border = "#64d8cb"
selected_bg = "pink"
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme.Background != "#090d16" {
		t.Fatalf("theme background = %q", cfg.Theme.Background)
	}
	if cfg.Theme.Title != "hotpink" {
		t.Fatalf("theme title = %q", cfg.Theme.Title)
	}
	if cfg.Theme.Foreground != "white" {
		t.Fatalf("omitted theme foreground did not keep default: %q", cfg.Theme.Foreground)
	}
}

func TestLoadRejectsInvalidThemeColor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	data := `
[[accounts]]
name = "personal"
maildir = "~/Maildir"

[theme]
border = "#zzzzzz"
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "theme.border") {
		t.Fatalf("expected invalid theme.border error, got %v", err)
	}
}
