# MailSalon

MailSalon is a Maildir-based terminal mail client written in Go using gotui v5.
It deliberately leaves network transport to external programs. MailSalon reads
and modifies local Maildirs, while tools such as `mbsync`, `offlineimap`,
`MailSalonSync`, `msmtp`, or custom wrapper scripts handle receiving and sending.

## Design

The folder pane stays on the left. The message list and preview are stacked on
the right so the message list remains wide enough to keep sender, subject, and
date/time visible.

MailSalon supports multiple independent accounts. A dedicated account selector
is always visible above the folder pane. It shows the active account and its
position, for example `Account 1/2` and `‹ cerberus ›`. Press `A`, click the
account selector, or use the mouse wheel over it to switch accounts.

```text
┌─ Account 1/2 ────────┐┌─ Messages ──────────────────────────────────────────┐
│ ‹ cerberus ›         ││ ●  From             Subject                  Date  │
├─ A/Click switch ─────┤│    Alice Example    Plans for Friday        14:32 │
┌─ Update Mail ────────┐│ ●  Bob Example      Re: project             13:08 │
│ ↻ Update             │├─ Message ───────────────────────────────────────────┤
├─ u/Click update ─────┤│ From: Alice Example <alice@example.com>            │
┌─ Folders ────────────┐│ To: britney@cerberusgames.ca                       │
│ INBOX                ││ Subject: Plans for Friday                          │
│ Archive              ││                                                    │
│ Drafts               ││ Message body...                                    │
│ Sent                 ││                                                    │
│ Trash                │├─ j/k Scroll  r Reply  f Fwd  m Read/Unread  a Save ┤
└──────────────────────┘└────────────────────────────────────────────────────┘
 [cerberus] Ready
 c Compose  u Update mail  r Reply  f Forward  m Read/unread  / Search
 d Delete  a Save attachments  A Switch account  Tab Focus  j/k Move  Enter Open  R Refresh  q Quit
```

## Features

- Local Maildir reader with `new`, `cur`, and Maildir flags.
- Multiple accounts, each with its own Maildir, identity, transport commands,
  signature file, Trash folder, and download directory.
- INBOX, Maildir++ folders such as `.Sent`, and nested Maildirs.
- Left-side folder navigation.
- Wide From / Subject / Date message table.
- Plain-text message preview. HTML-only messages are converted to readable terminal text, preserving paragraphs, lists, and useful link destinations while discarding scripts/styles.
- Contextual keybind hints are displayed directly on the message-preview pane.
- Manual read/unread toggle using the Maildir `S` (Seen) flag.
- Case-insensitive search within the current folder across sender, recipients,
  subject, date, Message-ID, and message body.
- New message composition with separate To, Cc, and Bcc fields. Each field accepts multiple comma-separated RFC-style addresses.
- Reply with `In-Reply-To`, `References`, and quoted original text.
- Selectable **Reply from** account in the compose UI.
- Automatic reply-account selection when a message's To/Cc address matches a
  configured account.
- Forward messages, including their original attachments.
- Add multiple outgoing attachments by file path.
- Save incoming attachments to the active account's download directory.
- Per-account signature files. Signatures are read when the message is sent,
  so changing a signature file does not require restarting MailSalon.
- Delete to the active account's configured Trash folder; deletion is confirmed
  with a second `d` press.
- Generic external receive command per account.
- Generic external send command per account. MailSalon writes the complete RFC
  5322/MIME message to the command's standard input.
- Mouse selection and mouse-wheel scrolling.
- Persistent on-screen key hints, including a mode-aware legend while composing, replying, or forwarding.
- TOML-configurable truecolor UI themes, including pane backgrounds, borders, active focus, titles, selections, unread mail, account identity, status/error text, and compose cursors.
- Always-visible account selector and Update Mail control above the folder list with keyboard and mouse hints.
- Keyboard-only operation remains fully supported.

## Requirements

- Go 1.24 or newer.
- `github.com/metaspartan/gotui/v5`.
- One or more Maildirs.
- Optional external receive program such as `mbsync`, `offlineimap`, or
  MailSalonSync.
- An external sender such as `msmtp`, MailSalonSync JMAP submission, or your
  own compatible command.

## Build

```sh
go build -o MailSalon ./cmd/MailSalon
```

Run it with the default configuration path:

```sh
./MailSalon
```

Or choose a configuration explicitly:

```sh
./MailSalon -config=./config.toml
```

To suppress a configured startup synchronization for one run:

```sh
./MailSalon -no-startup-sync
```

## Configuration

The default configuration path is:

```text
~/.config/mailsalon/config.toml
```

Accounts are TOML array-of-table entries. Each account is intentionally
self-contained so selecting another identity also selects the correct Maildir,
send command, sync command, signature, Trash folder, and attachment directory.

Example for `britney@cerberusgames.ca`:

```toml
[[accounts]]
name = "cerberus"
maildir = "~/Maildir"
from = "Britney Lozza <britney@cerberusgames.ca>"
signature_file = "~/.signature"
trash_folder = "Trash"
download_dir = "~/Downloads"
receive = "MailSalonSync -plain sync"
send = "MailSalonSync -plain jmap-send -account cerberus-jmap"

[options]
default_account = "cerberus"
startup_sync = false
```

### Multiple accounts

Add another `[[accounts]]` block:

```toml
[[accounts]]
name = "cerberus"
maildir = "~/Maildir"
from = "Britney Lozza <britney@cerberusgames.ca>"
signature_file = "~/.signature"
trash_folder = "Trash"
download_dir = "~/Downloads"
receive = "MailSalonSync -plain sync"
send = "MailSalonSync -plain jmap-send -account cerberus-jmap"

[[accounts]]
name = "work"
maildir = "~/Maildir-work"
from = "Britney Lozza <britney@work.example>"
signature_file = "~/.signature-work"
trash_folder = "Trash"
download_dir = "~/Downloads"
receive = "mbsync work"
send = "msmtp -a work -t"

[options]
default_account = "cerberus"
startup_sync = false
```

Account names must be unique. `default_account` chooses the mailbox shown when
MailSalon starts.

The earlier single-account `[mail]`, `[identity]`, and `[commands]` format is
still accepted for compatibility, but `[[accounts]]` is the recommended format.

### Themes

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

### Signatures

`signature_file` is optional and is configured per account:

```toml
signature_file = "~/.signature"
```

MailSalon reads the file at send time. If the file already starts with the
standard `-- ` signature separator, MailSalon preserves it. Otherwise MailSalon
adds the separator automatically. For replies and forwards, the signature is
placed before the quoted/forwarded original message.

### Receiving mail

`receive` is intentionally generic. MailSalon executes the active account's
configured command with `/bin/sh -c`, waits for it to finish, and rescans that
account's Maildir.

Examples:

```toml
receive = "mbsync work"
```

```toml
receive = "offlineimap"
```

```toml
receive = "MailSalonSync -plain sync"
```

A wrapper script works too:

```toml
receive = "~/bin/sync-my-mail"
```

### Sending mail

`send` receives the entire generated RFC 5322/MIME message on standard input.
For msmtp:

```toml
send = "msmtp -t"
```

For MailSalonSync JMAP submission:

```toml
send = "MailSalonSync -plain jmap-send -account cerberus-jmap"
```

A custom wrapper is equally valid:

```toml
send = "~/bin/mail-send-wrapper"
```

The selected From/Reply from account determines both the `From:` header and the
send command. This prevents selecting one identity while accidentally using a
different account's transport configuration.

## Keyboard controls

### Mail view

| Key | Action |
| --- | --- |
| `A` | Switch to the next configured account; the account selector is always visible above Folders |
| `Tab` | Cycle Folders → Messages → Preview |
| `h` / Left | Move focus toward folders |
| `l` / Right | Move focus toward messages/preview |
| `j` / Down | Move selection or scroll preview |
| `k` / Up | Move selection or scroll preview |
| `PageUp` / `PageDown` | Page through the active pane |
| `Home` / `End` | Jump to beginning/end |
| `Enter` | Open selected folder/message |
| `c` | Compose a new message |
| `r` | Reply |
| `f` | Forward |
| `m` | Toggle the selected message between read and unread |
| `/` | Search the current folder; submit an empty search to clear the filter |
| `d`, then `d` | Delete / confirm delete |
| `a` | Save all attachments from the selected message |
| `u` | Update mail: run the active account's receive command and rescan |
| `s` | Alias for Update mail |
| `R` | Rescan the active Maildir without running a receive command |
| `q` / `Ctrl+C` | Quit |

### Compose / reply / forward view

The first row is `From` for new messages and forwards, or `Reply from` for
replies. It displays the account name, email identity, and configured signature
file. A four-line legend remains visible at the bottom of the screen and labels
the current mode as `Compose`, `Reply`, or `Forward`.

New messages and forwards initially focus the `To` field. Replies initially
focus the message body so you can start typing above the quoted original text.

`To`, `Cc`, and `Bcc` each accept multiple comma-separated addresses, including
display-name forms such as `Alice <alice@example.com>, Bob <bob@example.com>`.
MailSalon validates each address list before invoking the external send command
and identifies the bad field if parsing fails.

| Key | Action |
| --- | --- |
| `Tab` | Next field |
| `Shift+Tab` | Previous field |
| Left / Right while From is selected | Select sending account |
| `h` / `l`, `j` / `k`, or Enter on From | Change sending account |
| `Ctrl+A` | Add an attachment by file path |
| `Ctrl+S` | Build the MIME message and run the selected account's send command |
| `Esc` | Cancel composition |
| Arrow keys | Move the cursor in normal input/body fields |

## Mouse controls

- Click a folder to open it.
- Click the dedicated Account selector above Folders to switch to the next account.
- Click the **Update Mail** control to run the active account's receive command and rescan the Maildir.
- Use the mouse wheel over the Account selector to switch backward/forward.
- Click a message to select and preview it.
- Click the preview pane to focus it.
- Use the mouse wheel over folders, messages, or the preview to scroll.
- Click the From/Reply from row to cycle the sending account.
- Use the mouse wheel over the From/Reply from row to cycle backward/forward.
- Compose fields can be focused with the mouse.

## Maildir behavior

Each account can point either at an INBOX Maildir itself or at a Maildir container that contains an `INBOX` child. MailSalon discovers Maildir++ folders such as `.Trash` and ordinary nested Maildirs without manufacturing a duplicate INBOX.
Messages in `new` or without the `S` flag are displayed as unread. Opening a
message moves it from `new` to `cur` when necessary and adds the `S` (seen)
flag. This also works for unread messages that a sync tool has already placed
in `cur`. Press `m` to manually remove/add the Seen flag and mark a message
unread/read.

Press `/` to search the currently open folder. Search is case-insensitive and
checks From, To, Cc, Subject, Date, Message-ID, and the rendered plain-text
body. Press Enter to apply the filter, Escape to cancel the prompt, or submit an
empty search to restore the full folder.

Deletion uses the active account's configured Trash Maildir. If that folder is
not present, MailSalon refuses to delete instead of guessing a path. Deleting a
message already in Trash permanently removes the file.

## Testing

```sh
go test ./...
```

The core packages include tests for configuration parsing, multi-account
configuration, signature loading, Maildir discovery and movement, MIME
construction/parsing with attachments, and external command stdin/stdout
handling.

## Current scope

MailSalon is intentionally a Maildir MUA rather than an IMAP/JMAP client. It
owns the local mail/user-interface behavior and delegates transport to whatever
external tools the user chooses.
