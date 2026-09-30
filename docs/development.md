# Development

[Documentation index](README.md)

## Build and test

Use Go 1.24 or newer. Dependencies are declared in `go.mod` and downloaded by Go.

```sh
go build -o MailSalon ./cmd/MailSalon
go test ./...
go vet ./...
```

`make build` and `make test` provide the build/test commands as well. The OpenPGP
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
| `internal/pim` | Native contact/calendar files and local editing |
| `internal/ui` | Terminal views, input widgets and application actions |
| `internal/version` | Version string |
| `docs` | User and contributor guides |

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
