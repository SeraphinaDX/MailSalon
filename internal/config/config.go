package config

import (
	"errors"
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Account struct {
	Name           string
	Maildir        string
	From           string
	SignatureFile  string
	ReceiveCommand string
	SendCommand    string
	DownloadDir    string
	TrashFolder    string
}

type Theme struct {
	Background   string
	Foreground   string
	Muted        string
	Border       string
	ActiveBorder string
	Title        string
	SelectedFG   string
	SelectedBG   string
	Account      string
	Unread       string
	Status       string
	Error        string
	CursorFG     string
	CursorBG     string
}

type Config struct {
	Accounts       []Account
	DefaultAccount string
	StartupSync    bool
	Theme          Theme
}

type fileAccount struct {
	Name          string `toml:"name"`
	Maildir       string `toml:"maildir"`
	From          string `toml:"from"`
	SignatureFile string `toml:"signature_file"`
	TrashFolder   string `toml:"trash_folder"`
	DownloadDir   string `toml:"download_dir"`
	Receive       string `toml:"receive"`
	Send          string `toml:"send"`
}

type fileTheme struct {
	Background   string `toml:"background"`
	Foreground   string `toml:"foreground"`
	Muted        string `toml:"muted"`
	Border       string `toml:"border"`
	ActiveBorder string `toml:"active_border"`
	Title        string `toml:"title"`
	SelectedFG   string `toml:"selected_fg"`
	SelectedBG   string `toml:"selected_bg"`
	Account      string `toml:"account"`
	Unread       string `toml:"unread"`
	Status       string `toml:"status"`
	Error        string `toml:"error"`
	CursorFG     string `toml:"cursor_fg"`
	CursorBG     string `toml:"cursor_bg"`
}

type fileConfig struct {
	Accounts []fileAccount `toml:"accounts"`

	// Legacy single-account sections remain readable so early MailSalon
	// prototype configs do not suddenly stop working.
	Mail struct {
		Maildir     string `toml:"maildir"`
		TrashFolder string `toml:"trash_folder"`
		DownloadDir string `toml:"download_dir"`
	} `toml:"mail"`
	Identity struct {
		From          string `toml:"from"`
		SignatureFile string `toml:"signature_file"`
	} `toml:"identity"`
	Commands struct {
		Receive string `toml:"receive"`
		Send    string `toml:"send"`
	} `toml:"commands"`

	Options struct {
		StartupSync    bool   `toml:"startup_sync"`
		DefaultAccount string `toml:"default_account"`
	} `toml:"options"`

	Theme fileTheme `toml:"theme"`
}

func DefaultTheme() Theme {
	return Theme{
		Background:   "default",
		Foreground:   "white",
		Muted:        "grey",
		Border:       "cyan",
		ActiveBorder: "green",
		Title:        "yellow",
		SelectedFG:   "black",
		SelectedBG:   "cyan",
		Account:      "white",
		Unread:       "white",
		Status:       "white",
		Error:        "red",
		CursorFG:     "black",
		CursorBG:     "white",
	}
}

func Default() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Accounts: []Account{{
			Name:        "default",
			Maildir:     filepath.Join(home, "Maildir"),
			DownloadDir: filepath.Join(home, "Downloads"),
			TrashFolder: "Trash",
		}},
		DefaultAccount: "default",
		Theme:          DefaultTheme(),
	}
}

func DefaultPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "mailsalon", "config.toml")
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = DefaultPath()
	}

	f, err := os.Open(expandPath(path))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	defer f.Close()

	raw := fileConfig{}
	decoder := toml.NewDecoder(f).DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}

	cfg.StartupSync = raw.Options.StartupSync
	cfg.DefaultAccount = strings.TrimSpace(raw.Options.DefaultAccount)
	cfg.Theme = mergeTheme(DefaultTheme(), raw.Theme)
	cfg.Accounts = nil

	if len(raw.Accounts) > 0 {
		for i, a := range raw.Accounts {
			cfg.Accounts = append(cfg.Accounts, normalizeAccount(a, i))
		}
	} else {
		// Backward-compatible conversion of the original single-account TOML.
		legacy := fileAccount{
			Name:          "default",
			Maildir:       raw.Mail.Maildir,
			From:          raw.Identity.From,
			SignatureFile: raw.Identity.SignatureFile,
			TrashFolder:   raw.Mail.TrashFolder,
			DownloadDir:   raw.Mail.DownloadDir,
			Receive:       raw.Commands.Receive,
			Send:          raw.Commands.Send,
		}
		cfg.Accounts = []Account{normalizeAccount(legacy, 0)}
	}

	if err := validateAccounts(cfg.Accounts); err != nil {
		return cfg, err
	}
	if err := validateTheme(cfg.Theme); err != nil {
		return cfg, err
	}
	if cfg.DefaultAccount == "" {
		cfg.DefaultAccount = cfg.Accounts[0].Name
	}
	if cfg.AccountIndex(cfg.DefaultAccount) < 0 {
		return cfg, fmt.Errorf("default_account %q does not match any configured account", cfg.DefaultAccount)
	}
	return cfg, nil
}

func normalizeAccount(a fileAccount, index int) Account {
	home, _ := os.UserHomeDir()
	name := strings.TrimSpace(a.Name)
	if name == "" {
		name = accountNameFromAddress(a.From)
	}
	if name == "" {
		name = fmt.Sprintf("account-%d", index+1)
	}
	maildirPath := strings.TrimSpace(a.Maildir)
	if maildirPath == "" && index == 0 {
		maildirPath = filepath.Join(home, "Maildir")
	}
	downloadDir := strings.TrimSpace(a.DownloadDir)
	if downloadDir == "" {
		downloadDir = filepath.Join(home, "Downloads")
	}
	trash := strings.TrimSpace(a.TrashFolder)
	if trash == "" {
		trash = "Trash"
	}
	return Account{
		Name:           name,
		Maildir:        expandPath(maildirPath),
		From:           strings.TrimSpace(a.From),
		SignatureFile:  expandPath(a.SignatureFile),
		TrashFolder:    trash,
		DownloadDir:    expandPath(downloadDir),
		ReceiveCommand: strings.TrimSpace(a.Receive),
		SendCommand:    strings.TrimSpace(a.Send),
	}
}

func mergeTheme(base Theme, raw fileTheme) Theme {
	set := func(dst *string, src string) {
		if s := strings.TrimSpace(src); s != "" {
			*dst = s
		}
	}
	set(&base.Background, raw.Background)
	set(&base.Foreground, raw.Foreground)
	set(&base.Muted, raw.Muted)
	set(&base.Border, raw.Border)
	set(&base.ActiveBorder, raw.ActiveBorder)
	set(&base.Title, raw.Title)
	set(&base.SelectedFG, raw.SelectedFG)
	set(&base.SelectedBG, raw.SelectedBG)
	set(&base.Account, raw.Account)
	set(&base.Unread, raw.Unread)
	set(&base.Status, raw.Status)
	set(&base.Error, raw.Error)
	set(&base.CursorFG, raw.CursorFG)
	set(&base.CursorBG, raw.CursorBG)
	return base
}

func validateAccounts(accounts []Account) error {
	if len(accounts) == 0 {
		return fmt.Errorf("no accounts configured")
	}
	seen := make(map[string]bool, len(accounts))
	for _, a := range accounts {
		if strings.TrimSpace(a.Name) == "" {
			return fmt.Errorf("account name cannot be empty")
		}
		key := strings.ToLower(a.Name)
		if seen[key] {
			return fmt.Errorf("duplicate account name %q", a.Name)
		}
		seen[key] = true
		if strings.TrimSpace(a.Maildir) == "" {
			return fmt.Errorf("account %q has no maildir", a.Name)
		}
	}
	return nil
}

var hexColorRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

var namedThemeColors = map[string]bool{
	"default": true, "clear": true,
	"black": true, "red": true, "green": true, "yellow": true,
	"blue": true, "magenta": true, "cyan": true, "white": true,
	"grey": true, "gray": true, "darkgrey": true, "darkgray": true,
	"lightgrey": true, "lightgray": true, "silver": true, "orange": true,
	"purple": true, "pink": true, "coral": true, "crimson": true,
	"gold": true, "teal": true, "turquoise": true, "indigo": true,
	"violet": true, "olive": true, "navy": true, "aliceblue": true,
	"beige": true, "brown": true, "darkblue": true, "darkcyan": true,
	"darkgreen": true, "darkred": true, "hotpink": true, "lightblue": true,
	"lightcyan": true, "lightgreen": true, "lime": true, "maroon": true,
	"mintcream": true, "mistyrose": true, "orchid": true, "plum": true,
	"salmon": true, "seagreen": true, "skyblue": true, "slateblue": true,
	"tan": true, "tomato": true, "wheat": true,
}

func validateTheme(t Theme) error {
	values := map[string]string{
		"background": t.Background, "foreground": t.Foreground, "muted": t.Muted,
		"border": t.Border, "active_border": t.ActiveBorder, "title": t.Title,
		"selected_fg": t.SelectedFG, "selected_bg": t.SelectedBG, "account": t.Account,
		"unread": t.Unread, "status": t.Status, "error": t.Error,
		"cursor_fg": t.CursorFG, "cursor_bg": t.CursorBG,
	}
	for name, value := range values {
		v := strings.ToLower(strings.TrimSpace(value))
		if hexColorRE.MatchString(value) || namedThemeColors[v] {
			continue
		}
		return fmt.Errorf("theme.%s has invalid color %q; use #RRGGBB or a supported color name", name, value)
	}
	return nil
}

func (c Config) AccountIndex(name string) int {
	for i, a := range c.Accounts {
		if strings.EqualFold(a.Name, strings.TrimSpace(name)) {
			return i
		}
	}
	return -1
}

func (c Config) DefaultAccountIndex() int {
	if i := c.AccountIndex(c.DefaultAccount); i >= 0 {
		return i
	}
	if len(c.Accounts) > 0 {
		return 0
	}
	return -1
}

func ReadSignature(a Account) (string, error) {
	if strings.TrimSpace(a.SignatureFile) == "" {
		return "", nil
	}
	data, err := os.ReadFile(a.SignatureFile)
	if err != nil {
		return "", fmt.Errorf("read signature for %s: %w", a.Name, err)
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}

func accountNameFromAddress(raw string) string {
	addr, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil || addr.Address == "" {
		return ""
	}
	local := strings.SplitN(addr.Address, "@", 2)[0]
	return strings.TrimSpace(local)
}

func expandPath(s string) string {
	s = os.ExpandEnv(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	if s == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(s, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, s[2:])
	}
	return s
}
