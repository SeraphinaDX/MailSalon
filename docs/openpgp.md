# OpenPGP

[Documentation index](README.md) · [Configuration](configuration.md)

OpenPGP is optional and configured independently for each account. MailSalon
uses the user's existing GnuPG keyring; it does not maintain its own key store.
The `command` setting is an executable name/path rather than a shell command.
If custom GnuPG arguments are required, point it at a wrapper script.

Place `[accounts.gpg]` directly after the account it applies to. The example
below shows that placement; edit an existing account rather than adding a
duplicate account block.

```toml
[[accounts]]
name = "personal"
maildir = "~/Maildir"
from = "Your Name <you@example.com>"
send = "MailSalonSync -plain jmap-send -account personal-jmap"

[accounts.gpg]
enabled = true
command = "gpg"
# homedir = "~/.gnupg"
sign_key = "you@example.com"
auto_sign = false
auto_encrypt = false
encrypt_to_self = true
```

Settings:

- `enabled` enables GPG integration for the account.
- `command` defaults to `gpg` and may be an executable path/name.
- `homedir` optionally selects a different GnuPG home/keyring.
- `sign_key` selects the signing key by email, fingerprint, or key ID. When it
  is omitted, MailSalon tries the account's `From` address.
- `auto_sign` and `auto_encrypt` select the initial mode in the compose UI.
- `encrypt_to_self` defaults to `true` so the sender can normally decrypt mail
  from the Sent folder later.

In Compose/Reply/Forward, press `Ctrl+G` to cycle:

```text
Off -> Sign -> Encrypt -> Sign+Encrypt -> Off
```

Signing and encryption use PGP/MIME rather than inline armored text. When both
are enabled, MailSalon signs the complete MIME entity first and then encrypts
it, so message text and attachments are protected together. All To/Cc
recipients are passed to GnuPG as normal recipients; Bcc recipients use GnuPG's
hidden-recipient mode so their key IDs are not deliberately exposed as normal
recipient packets.

Encryption is fail-closed. If GnuPG cannot find or trust a required recipient
key, MailSalon reports the error and does **not** fall back to sending the
message in plaintext.

When opening PGP/MIME mail, MailSalon automatically attempts decryption and
signature verification using the active account's GnuPG settings. The message
preview displays a line such as:

```text
OpenPGP: encrypted/decrypted; good signature from Alice <alice@example.com>
```

A bad or unverifiable signature is shown explicitly. If OpenPGP is disabled or
the private key is unavailable, MailSalon leaves encrypted content protected
and displays the decryption error instead of pretending it is readable.

As with normal PGP/MIME, envelope/message headers such as From, To, Date, and
Subject remain outside the encrypted MIME body. OpenPGP protects the MIME
content and attachments, not those outer headers.

GnuPG may use `gpg-agent`/pinentry when a private key requires a passphrase.
Keys that are already unlocked in the agent provide the smoothest TUI
experience.

For key or agent errors, see [troubleshooting](troubleshooting.md#openpgp-errors).
