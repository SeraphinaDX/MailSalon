// Package config loads, normalizes, and validates MailSalon's TOML configuration.
//
// Configuration is intentionally split into two representations. The private
// file* structs mirror the TOML file exactly, including optional/legacy fields,
// while Config, Account, Theme, Keybindings, and GPG are the normalized values
// used by the rest of the program. Load applies built-in defaults, expands
// paths, converts the old single-account format when necessary, and validates
// the result before returning it to the UI.
//
// Keeping parsing details here means the rest of MailSalon does not need to
// know whether a value came from TOML, a default, or a backward-compatibility
// conversion.
package config
