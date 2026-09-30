# Troubleshooting

[Documentation index](README.md) · [Quickstart](quickstart.md)

## No messages or the wrong mailbox

Check `maildir` in the active MailSalon account against the directory populated
by your sync tool. A folder must have `cur`, `new`, and `tmp` subdirectories;
MailSalon also accepts a container with Maildirs beneath it. Press `A` to check
whether you selected the wrong account.

Run your sync tool from the terminal, then press `R` in MailSalon to reload the
local files. If mail appears after `R` but not after `u`, check `receive`.
MailSalon cannot fetch remote mail until that command is configured and works.

## Config edits seem to have no effect

Check which file is being read. MailSalon uses `~/.config/mailsalon/config.toml`
on Linux, or `$XDG_CONFIG_HOME/mailsalon/config.toml` when that variable is set.
MailSalonSync uses its separate config; editing it does not change UI options.
Try `./MailSalon -config=/absolute/path/to/config.toml` to choose the file
explicitly, then restart after changing settings.

A missing config file uses defaults rather than raising a missing-file error.
For TOML parse errors, merge fields into existing sections instead of appending
duplicate `[options]`, `[theme]`, or `[keybindings]` tables. Use the exact field
names from the [configuration reference](configuration.md).

## Receive or send command fails

The status line shows the failure. Check the command in the selected account;
MailSalon runs it using `/bin/sh -c` and the environment it inherited.
Commands that depend on variables exported only in another terminal may fail.
Use an absolute executable path if the program is outside `PATH`.

For receive failures, exit MailSalon and run the same sync command in a terminal
to see the tool's full output. Check its server settings, credentials, and
account names. For MailSalonSync JMAP sending, the `-account` value must match
an account in the sync config. For msmtp, check its selected SMTP account.
A failed send leaves the composer open so you can correct the problem.

A configured signature file must exist and be readable at send time. Remove
`signature_file` if you do not want a signature.

## Sync runs even with startup sync disabled

`startup_sync = false` disables the initial sync. Background receive is a
separate option and defaults to five minutes. To stop periodic receive, set
`sync_interval = "0"` inside the existing `[options]` table. The
`-no-startup-sync` flag affects only startup. Manual `u` still runs sync.

## Contacts or calendars are empty or shared unexpectedly

Each collection's `local_dir` must match MailSalonSync's path, and `protocol`
must match the stored files: DAV uses `.vcf` / `.ics`; native JMAP uses `.json`.
Sync the collection first, then press `R` to reload it.

Set each private collection's `account` to the corresponding **MailSalon**
account `name`. An omitted account makes a collection shared across all
accounts. The UI account name need not match the MailSalonSync account name.
See [contacts and calendars](contacts-calendar.md#configuration).

## Reply sender was not added to contacts

Check that `auto_add_reply_contacts` is enabled (its default) and that the reply
account has an associated or shared contact collection. MailSalon checks all
email addresses, including aliases, before adding a sender. A matching existing
email means no new record is created. The status explains local save errors;
replying remains available. Upload occurs on the next sync.

## A contact or event edit fails, or a deleted item comes back

An active sync can hold the collection lock. Let sync finish and retry. If the
item changed since you opened the editor, reopen it to edit the new version;
stale editors cannot overwrite newer downloads. UID changes are rejected.

For local deletions to propagate remotely, enable `propagate_deletes = true`
in the corresponding **MailSalonSync** collection. Otherwise sync can restore
deleted tracked files. Local backups are retained in the collection's
`.mss-trash` directory. See [editing and deletion](contacts-calendar.md#new-contacts-and-events).

## Archive or delete is unavailable

The configured Archive/Trash Maildir must already exist. MailSalon does not
create those folders automatically. Check `archive_folder` / `trash_folder`
against the discovered folder names; a plain `Archive` or Maildir++ `.Archive`
works. Deleting a message already in Trash removes it permanently.

## Tab or text selection behaves differently in the body

In the message body, Tab inserts a tab; use Shift+Tab or click another field to
leave it. In recipient fields, Tab accepts a contact suggestion first.

Some terminals intercept Shift+arrow combinations. Use Ctrl+Space followed by
arrows to select, or drag with the mouse. Alt+A selects all text in the active
editable field. Ctrl+A attaches a file in the composer, and Ctrl+C quits.
See [text selection](usage.md#selecting-text).

## OpenPGP errors

Install GnuPG if enabling `[accounts.gpg]`. Ensure the chosen homedir/keyring
contains your private key for signing/decryption and recipients' public keys
for encryption. Check `sign_key`, key trust, and `gpg-agent`/pinentry access.

MailSalon keeps encryption failures visible and does not silently send the
message in plaintext. Verification failures appear in the preview.
See the [OpenPGP guide](openpgp.md).
