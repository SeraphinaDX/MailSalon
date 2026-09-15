// Package pgp implements MailSalon's OpenPGP/PGP-MIME integration.
//
// MailSalon does not embed an OpenPGP implementation. Instead it invokes a
// configured GnuPG-compatible executable directly (without a shell) and handles
// the surrounding MIME structure itself. Outgoing messages can be signed,
// encrypted, or signed-then-encrypted. Incoming top-level PGP/MIME wrappers are
// decrypted and/or verified before the ordinary MIME parser sees the message.
//
// The package keeps crypto processing separate from the UI: callers provide raw
// RFC 5322 bytes and receive processed bytes plus Info describing encryption and
// signature status. Failures are generally non-destructive so the original
// message can still be displayed with a useful error summary.
package pgp
