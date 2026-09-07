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

func main() {
	configPath := flag.String("config", config.DefaultPath(), "path to MailSalon configuration")
	noStartupSync := flag.Bool("no-startup-sync", false, "disable startup receive command for this run")
	showVersion := flag.Bool("version", false, "show MailSalon version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("MailSalon %s\n", version.Version)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "MailSalon:", err)
		os.Exit(1)
	}
	if *noStartupSync {
		cfg.StartupSync = false
	}

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
