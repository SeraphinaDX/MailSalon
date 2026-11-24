# Development

[Documentation index](README.md)

## Build and test

Use Go 1.26 or newer. Dependencies are declared in `go.mod` and downloaded by Go.

```sh
go build -o MailSalon ./cmd/MailSalon
make test-tui
make check
```

`make build-tui` and `make test-tui` run terminal-only checks without graphics
dependencies. `make test` and `make check` cover both clients with the Fyne CI
driver; `make build` remains the GUI build for compatibility. The OpenPGP
round-trip integration test needs GnuPG and a working `gpg-agent`; install them
if you want to exercise real signing, encryption, decryption, and verification.

## Project layout

| Directory | Responsibility |
| --- | --- |
| `cmd/MailSalon` | Command-line startup |
| `internal/config` | TOML loading, defaults, validation, signatures |
| `internal/maildir` | Folder discovery, message metadata, flags and moves |
| `internal/mimeutil` | MIME parsing/building and attachment handling |
| `internal/transport` | External receive/send commands |
| `internal/pgp` | GnuPG integration |
| `internal/pim` | Native contact/calendar storage, invitation import and recurrence projection |
| `internal/ui` | Terminal views, input widgets and application actions |
| `internal/version` | Version string |
| `docs/terminal` | Terminal user and contributor guides |

MailSalon owns local files and UI behavior. Network protocol implementations
and transport credentials belong to the configured external tools.

## Contributions

Keep changes focused, preserve unrelated files, and document changes to user
settings or controls. Format Go changes with `gofmt` and run checks appropriate
to the change. Use generic identities and paths in public examples.

## LLM code policy

This project accepts LLM-assisted contributions and already includes code
written with LLM assistance. Contributions must compile, pass the relevant
checks, and avoid regressions. Go was chosen in part for its memory safety.
