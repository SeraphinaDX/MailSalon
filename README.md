# MailSalon

MailSalon is a terminal app for email, contacts, and calendars, written in Go
with gotui v5. Read your local mail, compose and reply, look up contacts, and
manage calendar items using the keyboard or mouse.

MailSalon works with local Maildirs and contact/calendar files.
[MailSalonSync](https://github.com/SeraphinaDX/MailSalonSync) handles IMAP/JMAP
mail and CardDAV, CalDAV, and JMAP contacts/calendars. You can also use `mbsync`,
`offlineimap`, `msmtp`, or your own transport commands.

**New here? Start with the [quickstart guide](docs/quickstart.md).** It walks
through building MailSalon, connecting an existing mailbox, and sending your
first message. Current version: **0.6.0**.

## Get started

With Go 1.24 or newer and Git installed:

```sh
git clone https://github.com/SeraphinaDX/MailSalon.git
cd MailSalon
go build -o MailSalon ./cmd/MailSalon
mkdir -p ~/.config/mailsalon
cp -i config.toml.example ~/.config/mailsalon/config.toml
```

Edit `~/.config/mailsalon/config.toml`: set your name/email, the Maildir populated
by your sync tool, and your receive/send commands. The example uses
MailSalonSync and a placeholder sync account named `personal-jmap`; replace it
with your own. Configure and run the sync tool first, as described in the
[quickstart](docs/quickstart.md#1-prepare-your-mailbox).

```sh
./MailSalon
```

Already have a configuration? Keep it and run `./MailSalon` directly. Use
`./MailSalon -config=./config.toml` to try a different file.

## Everyday controls

The top bar shows **1 Mail**, **2 Contacts**, and **3 Calendar**. Press a number
or click a tab. Folders or collections stay on the left; the item list and
preview are on the right. The on-screen hints show your current keybindings.

| Action | Default control |
| --- | --- |
| Switch view | `1`, `2`, `3`, or click a tab |
| Switch account | `A`, or click/wheel over the account selector |
| Navigate | Arrow keys, `j` / `k`, mouse click or wheel |
| Change pane | `Tab` |
| Compose / reply / forward | `c` / `r` / `f` in Mail |
| Sync / reload local files | `u` / `R` |
| Search | `/` |
| Send a message / save a contact or event | `Ctrl+S` in the editor |
| Quit | `q` or `Ctrl+C` |

## What you can do

- Use multiple accounts with separate mailboxes, identities, signatures, and
  transport commands.
- Read plain text and HTML mail, search folders, mark mail read/unread, archive,
  and delete with confirmation.
- Compose with To/Cc/Bcc, attachments, contact suggestions, and automatic reply
  account selection. Missing reply senders are saved as contacts by default
  when a contact collection is configured; this can be disabled.
- Browse, search, create, and edit contacts and calendar items. Bind collections
  to an account or explicitly share them across accounts.
- Sync manually or on a configurable background timer.
- Select text in editable fields and customize keys and colors.
- Sign, encrypt, decrypt, and verify PGP/MIME mail with optional GnuPG support.

Calendar browsing currently uses an item list and source preview. Recurrence
data is preserved, but there is no month/week grid, expanded recurring agenda,
reminder notification, or invitation/RSVP workflow yet. See
[contacts and calendars](docs/contacts-calendar.md#calendar-view).

## Documentation

| Guide | Use it for |
| --- | --- |
| [Quickstart](docs/quickstart.md) | Install, configure one account, read and send mail |
| [Configuration](docs/configuration.md) | Accounts, sync interval, signatures, keys, themes, and command-line flags |
| [Using MailSalon](docs/usage.md) | Keyboard/mouse controls, composing, selecting text, and Maildir behavior |
| [Contacts and calendars](docs/contacts-calendar.md) | Collections, autocomplete, reply contact saving, and local editing |
| [OpenPGP](docs/openpgp.md) | Optional signing, encryption, keyring settings, and verification |
| [Troubleshooting](docs/troubleshooting.md) | Empty mailboxes, command failures, configuration errors, and sync issues |
| [Development](docs/development.md) | Build, tests, project structure, and contribution policy |
| [Example configuration](config.toml.example) | Annotated settings to copy and adapt |

MailSalon keeps transport credentials in your sync/sending tools' configuration.
Its `receive` and `send` settings choose the commands to run.
See [LICENSE](LICENSE) for licensing terms.
