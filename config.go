package main

import (
	"bufio"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

//go:embed mailsalon.conf
var embeddedConfig string

type Config struct {
	Maildir         string
	DownloadDir     string
	From            string
	OfflineIMAP     string
	OfflineIMAPArgs []string
	MSMTP           string
	MSMTPArgs       []string
	StartupSync     bool
}

type rawConfig struct {
	Maildir         string
	DownloadDir     string
	From            string
	OfflineIMAP     string
	OfflineIMAPArgs string
	MSMTP           string
	MSMTPArgs       string
	StartupSync     bool
}

func parseConfig() (Config, error) {
	var cfg Config

	raw, err := parseConfigText(embeddedConfig)
	if err != nil {
		return cfg, fmt.Errorf("embedded config: %w", err)
	}

	// Environment variables override only the embedded defaults. An explicitly
	// selected config file must win over ambient MAILTUI_* values.
	applyEnvironment(&raw)

	configPath, err := configPathFromArgs(os.Args[1:])
	if err != nil {
		return cfg, err
	}
	if configPath == "" {
		configPath = strings.TrimSpace(os.Getenv("MAILTUI_CONFIG"))
	}
	if configPath != "" {
		configPath, err = expandPath(configPath)
		if err != nil {
			return cfg, fmt.Errorf("config path: %w", err)
		}
		data, err := os.ReadFile(configPath)
		if err != nil {
			return cfg, fmt.Errorf("read config %q: %w", configPath, err)
		}
		if err := applyConfigText(&raw, string(data)); err != nil {
			return cfg, fmt.Errorf("config %q: %w", configPath, err)
		}
	}

	maildir := flag.String("maildir", raw.Maildir, "Maildir folder containing cur/new/tmp")
	downloadDir := flag.String("download-dir", raw.DownloadDir, "directory used when saving received attachments")
	from := flag.String("from", raw.From, "From address used for replies, e.g. 'Jane <jane@example.com>'")
	offlineimap := flag.String("offlineimap", raw.OfflineIMAP, "offlineimap executable")
	offlineArgs := flag.String("offlineimap-args", raw.OfflineIMAPArgs, "arguments passed to offlineimap")
	msmtp := flag.String("msmtp", raw.MSMTP, "msmtp executable")
	msmtpArgs := flag.String("msmtp-args", raw.MSMTPArgs, "arguments passed to msmtp")
	_ = flag.String("config", configPath, "optional external config file; overrides the config embedded in the binary")
	noStartupSync := flag.Bool("no-startup-sync", !raw.StartupSync, "open the local Maildir without running offlineimap first")
	flag.Parse()

	expanded, err := expandPath(*maildir)
	if err != nil {
		return cfg, err
	}
	cfg.Maildir = expanded
	downloadExpanded, err := expandPath(*downloadDir)
	if err != nil {
		return cfg, fmt.Errorf("download dir: %w", err)
	}
	cfg.DownloadDir = downloadExpanded
	cfg.From = strings.TrimSpace(*from)
	cfg.OfflineIMAP = strings.TrimSpace(*offlineimap)
	cfg.OfflineIMAPArgs, err = splitCommandLine(*offlineArgs)
	if err != nil {
		return cfg, fmt.Errorf("offlineimap args: %w", err)
	}
	cfg.MSMTP = strings.TrimSpace(*msmtp)
	cfg.MSMTPArgs, err = splitCommandLine(*msmtpArgs)
	if err != nil {
		return cfg, fmt.Errorf("msmtp args: %w", err)
	}
	cfg.StartupSync = !*noStartupSync

	if cfg.Maildir == "" {
		return cfg, errors.New("maildir cannot be empty")
	}
	if cfg.DownloadDir == "" {
		return cfg, errors.New("download directory cannot be empty")
	}
	if cfg.OfflineIMAP == "" {
		return cfg, errors.New("offlineimap executable cannot be empty")
	}
	if cfg.MSMTP == "" {
		return cfg, errors.New("msmtp executable cannot be empty")
	}
	return cfg, nil
}

func parseConfigText(text string) (rawConfig, error) {
	cfg := rawConfig{DownloadDir: "~/Downloads", StartupSync: true}
	if err := applyConfigText(&cfg, text); err != nil {
		return rawConfig{}, err
	}
	return cfg, nil
}

func applyConfigText(cfg *rawConfig, text string) error {
	scanner := bufio.NewScanner(strings.NewReader(text))
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("line %d: expected key = value", lineNo)
		}

		key := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(parts[0]), "-", "_"))
		value := strings.TrimSpace(parts[1])

		switch key {
		case "maildir":
			cfg.Maildir = value
		case "download_dir":
			cfg.DownloadDir = value
		case "from":
			cfg.From = value
		case "offlineimap":
			cfg.OfflineIMAP = value
		case "offlineimap_args":
			cfg.OfflineIMAPArgs = value
		case "msmtp":
			cfg.MSMTP = value
		case "msmtp_args":
			cfg.MSMTPArgs = value
		case "startup_sync":
			v, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("line %d: startup_sync: %w", lineNo, err)
			}
			cfg.StartupSync = v
		default:
			return fmt.Errorf("line %d: unknown setting %q", lineNo, key)
		}
	}
	return scanner.Err()
}

func applyEnvironment(cfg *rawConfig) {
	if v := os.Getenv("MAILTUI_MAILDIR"); v != "" {
		cfg.Maildir = v
	}
	if v := os.Getenv("MAILTUI_DOWNLOAD_DIR"); v != "" {
		cfg.DownloadDir = v
	}
	if v := os.Getenv("MAILTUI_FROM"); v != "" {
		cfg.From = v
	} else if cfg.From == "" {
		cfg.From = os.Getenv("EMAIL")
	}
	if v := os.Getenv("MAILTUI_OFFLINEIMAP"); v != "" {
		cfg.OfflineIMAP = v
	}
	if v := os.Getenv("MAILTUI_OFFLINEIMAP_ARGS"); v != "" {
		cfg.OfflineIMAPArgs = v
	}
	if v := os.Getenv("MAILTUI_MSMTP"); v != "" {
		cfg.MSMTP = v
	}
	if v := os.Getenv("MAILTUI_MSMTP_ARGS"); v != "" {
		cfg.MSMTPArgs = v
	}
	if v := os.Getenv("MAILTUI_STARTUP_SYNC"); v != "" {
		if enabled, err := strconv.ParseBool(v); err == nil {
			cfg.StartupSync = enabled
		}
	}
}

func configPathFromArgs(args []string) (string, error) {
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "-config" || arg == "--config" {
			if i+1 >= len(args) {
				return "", errors.New("-config requires a file path")
			}
			return args[i+1], nil
		}
		if strings.HasPrefix(arg, "-config=") {
			return strings.TrimPrefix(arg, "-config="), nil
		}
		if strings.HasPrefix(arg, "--config=") {
			return strings.TrimPrefix(arg, "--config="), nil
		}
	}
	return "", nil
}

func expandPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return filepath.Clean(path), nil
}

// splitCommandLine is intentionally small: it supports whitespace, single quotes,
// double quotes, and backslash escaping without invoking a shell.
func splitCommandLine(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		if escaped {
			cur.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case ' ', '\t', '\n':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	if escaped {
		return nil, errors.New("trailing backslash")
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	flush()
	return out, nil
}
