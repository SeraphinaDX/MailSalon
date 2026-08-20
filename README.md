# MailSalon

![Logo](mailsalon.avif)

A small Maildir-first terminal e-mail client written in Go with Charm's v2 libraries.

It intentionally delegates transport to traditional Unix mail tools:

- reads messages directly from a **Maildir**
- runs **offlineimap** on startup and when you press `s`
- sends replies by piping an RFC 5322 message to **msmtp**
- uses **Bubble Tea**, **Bubbles**, and **Lip Gloss** from `charm.land`

## Requirements

- Go 1.25+
- `offlineimap` configured and working
- `msmtp` configured and working
- a local Maildir containing `cur/`, `new/`, and `tmp/`

The Go 1.25 requirement comes from the current Bubbles v2 release.

## Configuration

MailSalon has a plain-text `mailsalon.conf` file embedded directly into the executable with Go's `//go:embed`. Edit that file before building to change the defaults that travel with the binary:

```text
maildir = ~/Mail/INBOX
download_dir = ~/Downloads
from = Jane Example <jane@example.com>

offlineimap = offlineimap
offlineimap_args = -a personal

msmtp = msmtp
msmtp_args = -t

startup_sync = true
```

The embedded configuration means the resulting executable is self-contained. If you want to change settings without rebuilding, pass an external file with `-config`:

```sh
./mailtui -config ~/.config/mailsalon.conf
```

You can also set `MAILTUI_CONFIG` to the external config path. Settings are applied in this order, with later sources winning:

1. embedded `mailsalon.conf`
2. existing `MAILTUI_*` environment variables
3. external `-config` / `MAILTUI_CONFIG` file
4. command-line flags

Blank lines and lines beginning with `#` or `;` are ignored. Unknown setting names are reported as errors so configuration typos do not silently pass.

## Build

```sh
go mod tidy
go build -o mailtui .
```

## Run

```sh
./mailtui \
  -maildir ~/Mail/example/INBOX \
  -from 'Jane Example <jane@example.com>'
```

Or configure it with environment variables:

```sh
export MAILTUI_MAILDIR="$HOME/Mail/example/INBOX"
export MAILTUI_FROM='Jane Example <jane@example.com>'
./mailtui
```

Useful environment variables:

```text
MAILTUI_MAILDIR
MAILTUI_DOWNLOAD_DIR
MAILTUI_FROM
MAILTUI_OFFLINEIMAP
MAILTUI_OFFLINEIMAP_ARGS
MAILTUI_MSMTP
MAILTUI_MSMTP_ARGS
```

For example, to sync one OfflineIMAP account:

```sh
MAILTUI_OFFLINEIMAP_ARGS='-a personal' ./mailtui
```
The default msmtp arguments are `-t`, so recipients are read from the generated message headers.

## Fish shell users:
```
use the `-config=<file>` form.
```

## Keys

### Inbox

| Key | Action |
|---|---|
| `j` / `k`, arrows | select previous/next message |
| `Enter`, `l`, right arrow | focus message body and mark it read |
| `Tab` | toggle list/body focus |
| `h`, left arrow, `Esc` | return from body to message list |
| `r` | reply to selected message |
| `a` | save all attachments from the selected message to `download_dir` |
| `d` / `Delete` | delete selected message after confirmation |
| `s` | run `offlineimap`, then reload Maildir |
| `q`, `Ctrl-C` | quit |

When the message body has focus, its pager-style viewport accepts `j/k`, page keys, and other Bubbles viewport bindings.

### Reply editor

| Key | Action |
|---|---|
| `Ctrl-O` | add an attachment by file path |
| `Ctrl-X` | remove the most recently added attachment |
| `Ctrl-S` | send through `msmtp` |
| `Esc` | cancel |

The reply preserves `In-Reply-To` and `References` headers when the source message provides a `Message-ID`. When files are attached, MailSalon sends a `multipart/mixed` MIME message and base64-encodes attachment parts.

## Attachments

Received attachments are listed above the message body with their decoded sizes. Press `a` in the inbox to save every attachment from the selected message. Existing files are not overwritten; MailSalon adds a numeric suffix when needed. The default destination is `~/Downloads` and can be changed with `download_dir`, `MAILTUI_DOWNLOAD_DIR`, or `-download-dir`.

While replying, press `Ctrl-O`, enter a file path, and press `Enter` to attach it. Repeat for multiple files. Press `Ctrl-X` to remove the last file before sending.

## Notes

This first version treats the configured Maildir as one mailbox. It does not yet provide a folder tree, compose-new-message workflow, search, archive, or a Sent-folder copy. `msmtp` itself sends mail but does not automatically create a local Sent message; if your mail provider/server does not save sent mail, that would be a useful next feature.
