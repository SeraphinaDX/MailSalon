# Quickstart

[Documentation index](README.md) · [Configuration reference](configuration.md)

This guide takes you from an existing email account to reading and sending mail
in MailSalon. The commands below use Linux paths. You need Git, Go 1.24 or newer,
a terminal, and a configured mail sync/sending tool. Go downloads MailSalon's
library dependencies during the build; you do not install gotui separately.

## 1. Prepare your mailbox

MailSalon reads local files. Set up your sync tool and download your mail before
launching it for the first time.

If you already use MailSalonSync, `mbsync`, or `offlineimap`, keep that setup and
find the directory it populates, such as `~/Maildir`. MailSalon accepts a Maildir
with `cur`, `new`, and `tmp` directly, or a container holding an `INBOX` Maildir
and other folders.

For a new setup, follow [MailSalonSync's setup instructions](https://github.com/SeraphinaDX/MailSalonSync#readme)
first. Its default Linux config is `~/.config/MailSalonSync/config.toml`; it owns
server addresses, login credentials, and synchronization state. MailSalon's
separate config is `~/.config/mailsalon/config.toml` (lowercase `mailsalon`).
Do not copy the sync configuration into MailSalon.

Check that sync works from your terminal:

```sh
MailSalonSync -plain sync
```

This guide uses MailSalonSync JMAP sending. Replace `personal-jmap` below with
the account name from your **MailSalonSync** config. If you receive with another
tool or send through SMTP, use the [transport alternatives](configuration.md#receive-and-send-commands).
Sending requires a configured sender; downloading mail alone does not enable it.

## 2. Build MailSalon

```sh
git clone https://github.com/SeraphinaDX/MailSalon.git
cd MailSalon
go build -o MailSalon ./cmd/MailSalon
./MailSalon -version
```

The last command prints the version without opening the UI. Run subsequent
`./MailSalon` commands from this checkout.

## 3. Create a minimal configuration

Create the configuration directory:

```sh
mkdir -p ~/.config/mailsalon
```

Open `~/.config/mailsalon/config.toml` in your editor and paste the complete
configuration below. If you already have a working file, keep it and compare
these settings instead of replacing it.

```toml
[[accounts]]
name = "personal"
maildir = "~/Maildir"
from = "Your Name <you@example.com>"
receive = "MailSalonSync -plain sync"
send = "MailSalonSync -plain jmap-send -account personal-jmap"

[options]
default_account = "personal"
startup_sync = false
sync_interval = "5m"
auto_add_reply_contacts = true
```

Replace `from`, `maildir`, and the sync account name in `send`. The UI account
name `personal` is your label in MailSalon; it can differ from the sync tool's
`personal-jmap`. The Maildir must be the same directory your sync tool writes.
Both commands must be available on the `PATH` inherited by MailSalon, or use
absolute executable paths.

GnuPG, signatures, themes, and contact/calendar collections are optional.
The [annotated example](../config.toml.example) shows those settings.

For local reading only, omit `receive` and `send`, and set
`sync_interval = "0"` under `[options]`. Sending stays unavailable until a sender
is configured.

## 4. Read your first message

```sh
./MailSalon
```

1. Confirm the expected account appears above the folder list.
2. Click `INBOX`, or select it with arrows and press Enter.
3. Click a message to preview it. `Tab` changes panes; arrows or `j` / `k` move
   through messages or scroll the preview.
4. Press `u` or click **Update Mail** to sync. Press `R` to reload local files
   without running the sync tool.

Opening a message marks it read. MailSalon runs receive commands every five
minutes with this config. Change `sync_interval` or set it to `"0"` to disable
the timer. `startup_sync = false` disables only the initial startup sync.

No messages? Check the Maildir path and sync tool using the
[troubleshooting guide](troubleshooting.md#no-messages-or-the-wrong-mailbox).

## 5. Send a message or reply

Press `c` in Mail to compose. Check **From**, enter a recipient in **To**, and
use `Tab` to reach Subject and Body. You can also click each field. In the body,
Tab inserts a tab; use `Shift+Tab` or a mouse click to leave it.

Press `Ctrl+S` to send, or Esc to cancel. Use `Ctrl+A` to attach a file by path.
To reply to the selected message, press `r`; the body is focused immediately,
and **Reply from** shows the chosen account. Clicking that row changes it.
MailSalon chooses a reply account from matching To/Cc addresses when possible.

If sending fails, the composer stays open and the status shows the error.
Check the account's send command and its own credentials/settings before trying
again. [Composing controls](usage.md#compose-reply-and-forward) cover Cc/Bcc,
attachments, text selection, and contact suggestions.

## 6. Add contacts and calendars when ready

Set up your remote collections in MailSalonSync, then add matching local
`[[collections]]` entries to MailSalon. Follow
[contacts and calendars](contacts-calendar.md#configuration). Use `2` / `3`
or click the top tabs to open them.

Contact suggestions appear in To/Cc/Bcc once contact collections are configured.
By default, opening a reply saves a missing sender to your contacts, even if
you later cancel the draft. To disable that, change this value inside your
existing `[options]` section:

```toml
auto_add_reply_contacts = false
```

If a message contains calendar events, press `i` or click **Calendar attachment**
to review and add one to a calendar. See [calendar attachment import](contacts-calendar.md#incoming-calendar-attachments).

## Optional: install the binary

To run MailSalon outside the checkout:

```sh
mkdir -p ~/.local/bin
cp MailSalon ~/.local/bin/MailSalon
```

Add `~/.local/bin` to your shell's `PATH` if needed. Then run `MailSalon`.
An explicit configuration uses `MailSalon -config=/path/to/config.toml`;
the `-config=...` spelling also works in fish.

## Calendar layouts

When a calendar collection is configured, press **3** or click **Calendar**.
Choose **Month**, **Week**, **Day**, or **Agenda** with the mouse or uppercase
`M`, `W`, `D`, or `G`. Month opens by default. Click a day to see its events;
click **Today** or press `T` to return to today. `[` and `]` move periods, and
`n` starts an event on the selected date. See [calendar details and
preferences](contacts-calendar.md#calendar-view).
