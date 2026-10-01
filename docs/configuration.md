# Configuration

[Documentation index](README.md) · [Quickstart](quickstart.md)

## File location and TOML structure

The default Linux path is `~/.config/mailsalon/config.toml`. If
`XDG_CONFIG_HOME` is set, MailSalon uses `mailsalon/config.toml` beneath it.
Use `-config=/path/to/config.toml` to select another file. Paths such as
`maildir`, `signature_file`, `download_dir`, and collection `local_dir` support
`~/` and environment variables.

This file belongs to **MailSalon**. Your sync tool has its own configuration
with server details and credentials; MailSalonSync's default Linux path is
`~/.config/MailSalonSync/config.toml`.

- `[[accounts]]` adds one independent email account.
- `[accounts.gpg]` applies to the preceding account and is optional.
- `[options]`, `[keybindings]`, and `[theme]` are global tables; define each once.
- `[[collections]]` adds a contact book or calendar. Its `account` refers to a
  MailSalon account name. See [collection configuration](contacts-calendar.md#configuration).

When adapting a snippet, merge fields into the existing matching section.
Do not append a second `[options]` or duplicate collection/account block.
Unrecognized field names and invalid settings are rejected at startup.
Missing optional fields use defaults. If the config file does not exist,
MailSalon uses a local `~/Maildir` account with no transport commands.

The old `[mail]`, `[identity]`, and `[commands]` single-account format remains
readable, but use `[[accounts]]` for new configurations.

## Account settings

| Field | Meaning / default |
| --- | --- |
| `name` | Unique account label used by the UI, `default_account`, and collections; matching is case-insensitive |
| `maildir` | Local mailbox root; defaults to `~/Maildir` for the first account; specify it for each account |
| `from` | Sender identity, for example `Your Name <you@example.com>`; needed for sending |
| `receive` | Sync command; optional for local-only reading |
| `send` | Command receiving the generated MIME message on stdin; needed for sending |
| `signature_file` | Optional text file read at send time |
| `trash_folder` | Local folder used by delete; default `Trash` |
| `archive_folder` | Local folder used by archive; default `Archive` |
| `download_dir` | Incoming attachment destination; default `~/Downloads` |

A Maildir can be an INBOX itself or a container with an `INBOX` child.
Maildir++ folders such as `.Sent` and ordinary nested Maildirs are discovered.
Trash and Archive must exist before their actions can be used; MailSalon does
not create those folders automatically. See [Maildir behavior](usage.md#maildir-behavior).

## Multiple accounts

This is a complete two-account example. Adapt the identities, paths, and
transport account names to your own tools:

```toml
[[accounts]]
name = "personal"
maildir = "~/Maildir"
from = "Your Name <you@example.com>"
receive = "MailSalonSync -plain sync"
send = "MailSalonSync -plain jmap-send -account personal-jmap"

[[accounts]]
name = "work"
maildir = "~/Maildir-work"
from = "Your Name <you@work.example>"
receive = "mbsync work"
send = "msmtp -a work -t"

[options]
default_account = "personal"
startup_sync = false
sync_interval = "5m"
auto_add_reply_contacts = true
```

Account labels must be unique. Switching accounts with `A`, or the mouse over
the account selector, changes the Maildir and visible contact/calendar
collections. The From/Reply from account selects its identity, signature,
sending command, and OpenPGP settings together.

## Receive and send commands

MailSalon runs these command strings through `/bin/sh -c`. Commands must be
available on its inherited `PATH`, or use absolute paths. Wrapper scripts work
too, for example `receive = "~/bin/sync-my-mail"`.

Set the fields in the applicable `[[accounts]]` block:

| Tool | `receive` | `send` |
| --- | --- | --- |
| MailSalonSync with JMAP submission | `"MailSalonSync -plain sync"` | `"MailSalonSync -plain jmap-send -account personal-jmap"` |
| mbsync and msmtp | `"mbsync personal"` | `"msmtp -a personal -t"` |
| offlineimap and msmtp | `"offlineimap"` | `"msmtp -t"` |

Configure the chosen tools separately first. `personal-jmap` names an account
in MailSalonSync, while `personal` in the other commands refers to those tools'
configurations. For IMAP mail without JMAP submission, use an SMTP sender such
as msmtp. Contacts/calendars alone do not provide mail sending.

Receive commands update local files; after completion MailSalon reloads them.
Send commands receive the entire RFC 5322/MIME message on standard input.
The selected compose account determines both the From header and send command.
MailSalon reports transport failures in the status line.

## Global options

```toml
[options]
default_account = "personal"
startup_sync = false
sync_interval = "5m"
auto_add_reply_contacts = true
calendar_default_view = "month"
calendar_week_start = "monday"
```

| Option | Default | Behavior |
| --- | --- | --- |
| `default_account` | First account | Account selected when MailSalon starts |
| `startup_sync` | `false` | Run the selected account's receive command once at startup |
| `sync_interval` | `"5m"` | Interval between background receive cycles; `"0"` disables them |
| `auto_add_reply_contacts` | `true` | Save missing senders when opening replies, if a contact collection is configured |
| `calendar_default_view` | `"month"` | Initial layout: `month`, `week`, `day`, or `agenda` |
| `calendar_week_start` | `"monday"` | First weekday: `monday` or `sunday` |

Intervals use Go duration syntax, such as `"30s"`, `"10m"`, or `"1h"`. Enabled
intervals must be at least one second. Background cycles run the accounts'
receive commands and deduplicate identical command strings. The UI stays usable
while they run, and the active Maildir and PIM view are refreshed afterward.
Manual `u` runs only the active account's receive command.

`startup_sync` and `sync_interval` are independent. The command-line
`-no-startup-sync` override does not disable the background timer.

Set `auto_add_reply_contacts = false` in the existing `[options]` table to
disable automatic contact saving. Autocomplete remains available. See
[reply contact saving](contacts-calendar.md#contact-autocomplete-and-reply-saving)
for destination selection and duplicate checks.

## Signatures

`signature_file` is optional and is configured per account:

```toml
signature_file = "~/.signature"
```

MailSalon reads the file at send time. If the file already starts with the
standard `-- ` signature separator, MailSalon preserves it. Otherwise MailSalon
adds the separator automatically. For replies and forwards, the signature is
placed before the quoted/forwarded original message.

## Keybindings

MailSalon action keys can be changed in `config.toml`. The main-view, message-preview, and compose legends are generated from these values, so the UI always shows the keys that are actually active. Single-character bindings are case-sensitive (`a` and `A` are different). Named keys use friendly names such as `Tab`, `Shift+Tab`, `Enter`, `Esc`, `PgUp`, and `PgDn`. `Ctrl+X`-style bindings are supported for single-character control combinations.

```toml
[keybindings]
mail_view = "1"
contacts_view = "2"
calendar_view = "3"
calendar_month = "M"
calendar_week = "W"
calendar_day = "D"
calendar_agenda = "G"
calendar_previous = "["
calendar_next = "]"
calendar_today = "T"
quit = "q"
compose = "c"
sync = "u"
reply = "r"
forward = "f"
archive = "e"
toggle_read = "m"
search = "/"
delete = "d"
save_attachments = "a"
import_calendar = "i"
switch_account = "A"
refresh = "R"

focus_next = "Tab"
focus_left = "h"
focus_right = "l"
move_up = "k"
move_down = "j"
page_up = "PgUp"
page_down = "PgDn"
home = "Home"
end = "End"
open = "Enter"

send = "Ctrl+S"
attach = "Ctrl+A"
pgp_mode = "Ctrl+G"
cancel = "Esc"
next_field = "Tab"
previous_field = "Shift+Tab"
```

Duplicate bindings within the same UI mode are rejected at startup instead of making one action silently unreachable. Mouse controls are unchanged by keyboard remapping.

## Themes

The UI can be themed directly in `config.toml` with an optional `[theme]`
section. MailSalon supports 24-bit `#RRGGBB` colors as well as common color
names such as `red`, `cyan`, `pink`, `grey`, `skyblue`, and `default`.
Omitted theme values use the built-in defaults.

```toml
[theme]
background = "#090d16"
foreground = "#d8dee9"
muted = "#77839a"
border = "#36506b"
active_border = "#64d8cb"
title = "#ff9ecb"
selected_fg = "#071018"
selected_bg = "#ff9ecb"
account = "#d9a7ff"
unread = "#f6c177"
status = "#64d8cb"
error = "#ff6b81"
cursor_fg = "#071018"
cursor_bg = "#ff9ecb"
```

The theme roles are intentionally semantic:

- `background` and `foreground` control the normal pane background/text.
- `border` is the normal pane border; `active_border` marks the focused pane.
- `title` colors pane titles and the account selector border.
- `muted` is used for secondary UI text such as pane-bottom key hints and the
  message-table header.
- `selected_fg` / `selected_bg` style selected folders and messages.
- `account` highlights the currently selected account and From/Reply from row.
- `unread` colors unread message rows.
- `status` colors normal footer status text; `error` is used for failures.
- `cursor_fg` / `cursor_bg` control the compose cursor.

Use `background = "default"` if you prefer MailSalon to inherit the terminal's
normal background instead of painting its own color.

## OpenPGP

OpenPGP is optional and configured per account through `[accounts.gpg]`.
Follow the [OpenPGP guide](openpgp.md) once basic sending works.

## Command-line flags

| Flag | Behavior |
| --- | --- |
| `-config=/path/to/config.toml` | Read the selected config file; use this spelling in fish too |
| `-no-startup-sync` | Skip the configured startup receive for this run; periodic sync remains enabled |
| `-version` | Print the version and exit |
| `-h` | Show command-line help |

Example:

```sh
./MailSalon -config=./config.toml -no-startup-sync
```
