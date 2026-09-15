// Package mimeutil parses incoming Internet mail and builds outgoing messages.
//
// Incoming messages are reduced to the information the terminal UI needs:
// decoded headers, a readable text body, threading headers, and attachments.
// Multipart MIME is walked recursively. Plain-text parts are preferred; when a
// message is HTML-only, MailSalon converts the useful structure and links to a
// conservative plain-text representation rather than rendering HTML.
//
// Outgoing Draft values are serialized as RFC 5322/MIME messages. Attachments
// may come from files selected during composition or from in-memory attachments
// preserved while forwarding. Header values are validated before writing so
// user-entered text cannot inject additional mail headers.
package mimeutil
