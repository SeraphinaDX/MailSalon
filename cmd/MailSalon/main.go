package main

import (
	"flag"
	"fmt"
	"os"

	ui "github.com/metaspartan/gotui/v5"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
	uiapp "git.cerberusgames.ca/Starstreak/MailSalon/internal/ui"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/version"
)

// main is intentionally small: configuration and UI behavior live in internal
// packages so command-line startup remains easy to follow and test independently.
func main() {
	configPath := flag.String("config", config.DefaultPath(), "path to MailSalon configuration")
	noStartupSync := flag.Bool("no-startup-sync", false, "disable startup receive command for this run")
	showVersion := flag.Bool("version", false, "show MailSalon version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("MailSalon %s\n", version.Version)
		return
	}

	// Load applies defaults and validates the TOML before any terminal state is
	// changed. A command-line startup override is applied afterward so it affects
	// only this invocation and never rewrites the user's configuration file.
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "MailSalon:", err)
		os.Exit(1)
	}
	if *noStartupSync {
		cfg.StartupSync = false
	}

	// gotui owns the terminal while MailSalon is running. Always Close it on a
	// normal return so terminal modes/cursor state are restored for the shell.
	if err := ui.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "MailSalon: initialize terminal:", err)
		os.Exit(1)
	}
	defer ui.Close()

	app, err := uiapp.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "MailSalon:", err)
		os.Exit(1)
	}
	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "MailSalon:", err)
		os.Exit(1)
	}
}
