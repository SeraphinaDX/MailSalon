package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfigText(t *testing.T) {
	cfg, err := parseConfigText(`
# comment
maildir = ~/Mail/test/INBOX
download_dir = ~/Downloads/MailSalon
from = Jane Example <jane@example.com>
offlineimap = offlineimap
offlineimap_args = -a personal
msmtp = msmtp
msmtp_args = -t --read-envelope-from
startup_sync = false
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Maildir != "~/Mail/test/INBOX" {
		t.Fatalf("maildir = %q", cfg.Maildir)
	}
	if cfg.DownloadDir != "~/Downloads/MailSalon" {
		t.Fatalf("download_dir = %q", cfg.DownloadDir)
	}
	if cfg.From != "Jane Example <jane@example.com>" {
		t.Fatalf("from = %q", cfg.From)
	}
	if cfg.OfflineIMAPArgs != "-a personal" {
		t.Fatalf("offlineimap_args = %q", cfg.OfflineIMAPArgs)
	}
	if cfg.StartupSync {
		t.Fatal("startup_sync = true, want false")
	}
}

func TestParseConfigTextRejectsUnknownKey(t *testing.T) {
	_, err := parseConfigText("maildr = ~/Mail/INBOX\n")
	if err == nil {
		t.Fatal("expected unknown key error")
	}
}

func TestExplicitConfigOverridesEnvironment(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "cerberus.conf")
	if err := os.WriteFile(configPath, []byte("maildir = /config/maildir\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MAILTUI_MAILDIR", "/environment/maildir")

	oldArgs := os.Args
	oldCommandLine := flag.CommandLine
	t.Cleanup(func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
	})

	os.Args = []string{"mailsalon", "-config", configPath}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	cfg, err := parseConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Maildir != "/config/maildir" {
		t.Fatalf("maildir = %q, want %q", cfg.Maildir, "/config/maildir")
	}
}
