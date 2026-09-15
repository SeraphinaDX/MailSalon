// Package transport is the boundary between MailSalon and external mail tools.
//
// Receiving and sending are intentionally delegated instead of being coupled to
// one protocol implementation. Receive runs the configured synchronization
// command and captures its output. Send writes a complete RFC 5322 message to
// the configured send command's standard input, which supports tools such as
// msmtp and MailSalonSync's JMAP sender.
//
// Commands are user configuration and currently run through /bin/sh -c so
// ordinary shell command lines work as written in TOML. Callers should treat
// command output as diagnostic text, not structured protocol data.
package transport
