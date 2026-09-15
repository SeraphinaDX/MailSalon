// Package maildir provides MailSalon's local Maildir view and file operations.
//
// MailSalon deliberately treats the local Maildir as its working data store;
// network synchronization is handled by an external receive command such as
// MailSalonSync or mbsync. This package discovers standard Maildir and Maildir++
// folders, reads lightweight message summaries, and performs local moves for
// read/unread, archive, and trash operations.
//
// Maildir state is encoded in both directory placement (new/ versus cur/) and
// filename flags such as S (Seen). Operations preserve those conventions so an
// external synchronization tool can observe and propagate the changes later.
// Folder discovery also handles both layouts commonly produced by sync tools:
// an account root that is itself INBOX and a container root with an explicit
// INBOX subdirectory.
package maildir
