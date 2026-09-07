package config

import (
	"errors"
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
)

type GPG struct {
	Enabled       bool
	Command       string
	HomeDir       string
	SignKey       string
	AutoSign      bool
	AutoEncrypt   bool
	EncryptToSelf bool
}

type Account struct {
	Name           string
	Maildir        string
	From           string
	SignatureFile  string
	ReceiveCommand string
	SendCommand    string
	DownloadDir    string
	TrashFolder    string
	ArchiveFolder  string
	GPG            GPG
}

type Keybindings struct {
	Quit            string
	Compose         string
	Sync            string
	Reply           string
	Forward         string
	Archive         string
	ToggleRead      string
	Search          string
	Delete          string
	SaveAttachments string
	SwitchAccount   string
	Refresh         string
	FocusNext       string
	FocusLeft       string
	FocusRight      string
	MoveUp          string
	MoveDown        string
	PageUp          string
	PageDown        string
	Home            string
	End             string
	Open            string
	Send            string
	Attach          string
	PGPMode         string
	Cancel          string
	NextField       string
	PreviousField   string
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
	Keybindings    Keybindings
}

type fileGPG struct {
	Enabled       *bool  `toml:"enabled"`
	Command       string `toml:"command"`
	HomeDir       string `toml:"homedir"`
	SignKey       string `toml:"sign_key"`
	AutoSign      bool   `toml:"auto_sign"`
	AutoEncrypt   bool   `toml:"auto_encrypt"`
	EncryptToSelf *bool  `toml:"encrypt_to_self"`
}

type fileAccount struct {
	Name          string  `toml:"name"`
	Maildir       string  `toml:"maildir"`
	From          string  `toml:"from"`
	SignatureFile string  `toml:"signature_file"`
	TrashFolder   string  `toml:"trash_folder"`
	ArchiveFolder string  `toml:"archive_folder"`
	DownloadDir   string  `toml:"download_dir"`
	Receive       string  `toml:"receive"`
	Send          string  `toml:"send"`
	GPG           fileGPG `toml:"gpg"`
}

type fileKeybindings struct {
	Quit            string `toml:"quit"`
	Compose         string `toml:"compose"`
	Sync            string `toml:"sync"`
	Reply           string `toml:"reply"`
	Forward         string `toml:"forward"`
	Archive         string `toml:"archive"`
	ToggleRead      string `toml:"toggle_read"`
	Search          string `toml:"search"`
	Delete          string `toml:"delete"`
	SaveAttachments string `toml:"save_attachments"`
	SwitchAccount   string `toml:"switch_account"`
	Refresh         string `toml:"refresh"`
	FocusNext       string `toml:"focus_next"`
	FocusLeft       string `toml:"focus_left"`
	FocusRight      string `toml:"focus_right"`
	MoveUp          string `toml:"move_up"`
	MoveDown        string `toml:"move_down"`
	PageUp          string `toml:"page_up"`
	PageDown        string `toml:"page_down"`
	Home            string `toml:"home"`
	End             string `toml:"end"`
	Open            string `toml:"open"`
	Send            string `toml:"send"`
	Attach          string `toml:"attach"`
	PGPMode         string `toml:"pgp_mode"`
	Cancel          string `toml:"cancel"`
	NextField       string `toml:"next_field"`
	PreviousField   string `toml:"previous_field"`
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
		Maildir       string `toml:"maildir"`
		TrashFolder   string `toml:"trash_folder"`
		ArchiveFolder string `toml:"archive_folder"`
		DownloadDir   string `toml:"download_dir"`
	} `toml:"mail"`
	Identity struct {
		From          string `toml:"from"`
		SignatureFile string `toml:"signature_file"`
	} `toml:"identity"`
	Commands struct {
		Receive string `toml:"receive"`
		Send    string `toml:"send"`
	} `toml:"commands"`
	GPG fileGPG `toml:"gpg"`

	Options struct {
		StartupSync    bool   `toml:"startup_sync"`
		DefaultAccount string `toml:"default_account"`
	} `toml:"options"`

	Theme fileTheme `toml:"theme"`

	Keybindings fileKeybindings `toml:"keybindings"`
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

func DefaultKeybindings() Keybindings {
	return Keybindings{
		Quit:            "q",
		Compose:         "c",
		Sync:            "u",
		Reply:           "r",
		Forward:         "f",
		Archive:         "e",
		ToggleRead:      "m",
		Search:          "/",
		Delete:          "d",
		SaveAttachments: "a",
		SwitchAccount:   "A",
		Refresh:         "R",
		FocusNext:       "Tab",
		FocusLeft:       "h",
		FocusRight:      "l",
		MoveUp:          "k",
		MoveDown:        "j",
		PageUp:          "PgUp",
		PageDown:        "PgDn",
		Home:            "Home",
		End:             "End",
		Open:            "Enter",
		Send:            "Ctrl+S",
		Attach:          "Ctrl+A",
		PGPMode:         "Ctrl+G",
		Cancel:          "Esc",
		NextField:       "Tab",
		PreviousField:   "Shift+Tab",
	}
}

// KeybindingsWithDefaults fills any empty binding with MailSalon's built-in
// default. This is useful for callers/tests that construct Config values in
// memory instead of loading TOML through Load.
func KeybindingsWithDefaults(k Keybindings) Keybindings {
	d := DefaultKeybindings()
	set := func(dst *string, src string) {
		if strings.TrimSpace(src) != "" {
			*dst = strings.TrimSpace(src)
		}
	}
	set(&d.Quit, k.Quit)
	set(&d.Compose, k.Compose)
	set(&d.Sync, k.Sync)
	set(&d.Reply, k.Reply)
	set(&d.Forward, k.Forward)
	set(&d.Archive, k.Archive)
	set(&d.ToggleRead, k.ToggleRead)
	set(&d.Search, k.Search)
	set(&d.Delete, k.Delete)
	set(&d.SaveAttachments, k.SaveAttachments)
	set(&d.SwitchAccount, k.SwitchAccount)
	set(&d.Refresh, k.Refresh)
	set(&d.FocusNext, k.FocusNext)
	set(&d.FocusLeft, k.FocusLeft)
	set(&d.FocusRight, k.FocusRight)
	set(&d.MoveUp, k.MoveUp)
	set(&d.MoveDown, k.MoveDown)
	set(&d.PageUp, k.PageUp)
	set(&d.PageDown, k.PageDown)
	set(&d.Home, k.Home)
	set(&d.End, k.End)
	set(&d.Open, k.Open)
	set(&d.Send, k.Send)
	set(&d.Attach, k.Attach)
	set(&d.PGPMode, k.PGPMode)
	set(&d.Cancel, k.Cancel)
	set(&d.NextField, k.NextField)
	set(&d.PreviousField, k.PreviousField)
	return d
}

func Default() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Accounts: []Account{{
			Name:          "default",
			Maildir:       filepath.Join(home, "Maildir"),
			DownloadDir:   filepath.Join(home, "Downloads"),
			TrashFolder:   "Trash",
			ArchiveFolder: "Archive",
		}},
		DefaultAccount: "default",
		Theme:          DefaultTheme(),
		Keybindings:    DefaultKeybindings(),
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
	cfg.Keybindings = mergeKeybindings(DefaultKeybindings(), raw.Keybindings)
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
			ArchiveFolder: raw.Mail.ArchiveFolder,
			DownloadDir:   raw.Mail.DownloadDir,
			Receive:       raw.Commands.Receive,
			Send:          raw.Commands.Send,
			GPG:           raw.GPG,
		}
		cfg.Accounts = []Account{normalizeAccount(legacy, 0)}
	}

	if err := validateAccounts(cfg.Accounts); err != nil {
		return cfg, err
	}
	if err := validateTheme(cfg.Theme); err != nil {
		return cfg, err
	}
	if err := validateKeybindings(cfg.Keybindings); err != nil {
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
	archive := strings.TrimSpace(a.ArchiveFolder)
	if archive == "" {
		archive = "Archive"
	}
	return Account{
		Name:           name,
		Maildir:        expandPath(maildirPath),
		From:           strings.TrimSpace(a.From),
		SignatureFile:  expandPath(a.SignatureFile),
		TrashFolder:    trash,
		ArchiveFolder:  archive,
		DownloadDir:    expandPath(downloadDir),
		ReceiveCommand: strings.TrimSpace(a.Receive),
		SendCommand:    strings.TrimSpace(a.Send),
		GPG:            normalizeGPG(a.GPG),
	}
}

func mergeKeybindings(base Keybindings, raw fileKeybindings) Keybindings {
	set := func(dst *string, src string) {
		if s := strings.TrimSpace(src); s != "" {
			*dst = s
		}
	}
	set(&base.Quit, raw.Quit)
	set(&base.Compose, raw.Compose)
	set(&base.Sync, raw.Sync)
	set(&base.Reply, raw.Reply)
	set(&base.Forward, raw.Forward)
	set(&base.Archive, raw.Archive)
	set(&base.ToggleRead, raw.ToggleRead)
	set(&base.Search, raw.Search)
	set(&base.Delete, raw.Delete)
	set(&base.SaveAttachments, raw.SaveAttachments)
	set(&base.SwitchAccount, raw.SwitchAccount)
	set(&base.Refresh, raw.Refresh)
	set(&base.FocusNext, raw.FocusNext)
	set(&base.FocusLeft, raw.FocusLeft)
	set(&base.FocusRight, raw.FocusRight)
	set(&base.MoveUp, raw.MoveUp)
	set(&base.MoveDown, raw.MoveDown)
	set(&base.PageUp, raw.PageUp)
	set(&base.PageDown, raw.PageDown)
	set(&base.Home, raw.Home)
	set(&base.End, raw.End)
	set(&base.Open, raw.Open)
	set(&base.Send, raw.Send)
	set(&base.Attach, raw.Attach)
	set(&base.PGPMode, raw.PGPMode)
	set(&base.Cancel, raw.Cancel)
	set(&base.NextField, raw.NextField)
	set(&base.PreviousField, raw.PreviousField)
	return base
}

func validateKeybindings(k Keybindings) error {
	main := map[string]string{
		"quit": k.Quit, "compose": k.Compose, "sync": k.Sync, "reply": k.Reply,
		"forward": k.Forward, "archive": k.Archive, "toggle_read": k.ToggleRead,
		"search": k.Search, "delete": k.Delete, "save_attachments": k.SaveAttachments,
		"switch_account": k.SwitchAccount, "refresh": k.Refresh, "focus_next": k.FocusNext,
		"focus_left": k.FocusLeft, "focus_right": k.FocusRight, "move_up": k.MoveUp,
		"move_down": k.MoveDown, "page_up": k.PageUp, "page_down": k.PageDown,
		"home": k.Home, "end": k.End, "open": k.Open,
	}
	compose := map[string]string{
		"send": k.Send, "attach": k.Attach, "pgp_mode": k.PGPMode,
		"cancel": k.Cancel, "next_field": k.NextField, "previous_field": k.PreviousField,
	}
	for groupName, group := range map[string]map[string]string{"main": main, "compose": compose} {
		seen := map[string]string{}
		for action, key := range group {
			key = strings.TrimSpace(key)
			if key == "" {
				return fmt.Errorf("keybindings.%s cannot be empty", action)
			}
			canonical := keybindingConflictKey(key)
			if previous, exists := seen[canonical]; exists {
				return fmt.Errorf("keybindings.%s conflicts with %s binding %q", action, previous, key)
			}
			seen[canonical] = action
		}
		_ = groupName
	}
	return nil
}

func keybindingConflictKey(key string) string {
	key = strings.TrimSpace(key)
	if utf8.RuneCountInString(key) == 1 {
		// gotui distinguishes printable lowercase and uppercase runes, so `a`
		// and `A` are valid separate bindings.
		return "rune:" + key
	}
	return "named:" + strings.ToLower(key)
}

func normalizeGPG(raw fileGPG) GPG {
	enabled := false
	if raw.Enabled != nil {
		enabled = *raw.Enabled
	}
	encryptToSelf := true
	if raw.EncryptToSelf != nil {
		encryptToSelf = *raw.EncryptToSelf
	}
	command := strings.TrimSpace(raw.Command)
	if command == "" {
		command = "gpg"
	}
	return GPG{
		Enabled:       enabled,
		Command:       command,
		HomeDir:       expandPath(raw.HomeDir),
		SignKey:       strings.TrimSpace(raw.SignKey),
		AutoSign:      raw.AutoSign,
		AutoEncrypt:   raw.AutoEncrypt,
		EncryptToSelf: encryptToSelf,
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
		if !a.GPG.Enabled && (a.GPG.AutoSign || a.GPG.AutoEncrypt) {
			return fmt.Errorf("account %q enables automatic OpenPGP protection but gpg.enabled is false", a.Name)
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
