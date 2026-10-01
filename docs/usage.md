# Using MailSalon

[Documentation index](README.md) · [Quickstart](quickstart.md)

The tables below show the defaults. Any configurable entry is reflected automatically in the on-screen legend after remapping it under `[keybindings]`.

## Mail view

| Key | Action |
| --- | --- |
| `1` / `2` / `3` | Switch Mail / Contacts / Calendar |
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
| `u` | Sync: run the active account's receive command and rescan |
| `r` | Reply |
| `f` | Forward |
| `e` | Archive the selected message when the configured Archive folder exists |
| `m` | Toggle the selected message between read and unread |
| `/` | Search the current folder; submit an empty search to clear the filter |
| `d`, then `d` | Delete / confirm delete |
| `a` | Save all attachments from the selected message |
| `i` | Review incoming calendar events and choose a calendar to add one |
| `R` | Rescan the active Maildir without running a receive command |
| `q` / `Ctrl+C` | Quit |

## Compose, reply, and forward

The first row is `From` for new messages and forwards, or `Reply from` for
replies. It displays the account name, email identity, and configured signature
file. A four-line legend remains visible at the bottom of the screen, labels
the current mode as `Compose`, `Reply`, or `Forward`, and shows the current
OpenPGP mode.

New messages and forwards initially focus the `To` field. Replies initially
focus the message body so you can start typing above the quoted original text.

`To`, `Cc`, and `Bcc` each accept multiple comma-separated addresses, including
display-name forms such as `Alice <alice@example.com>, Bob <bob@example.com>`.
MailSalon validates each address list before invoking the external send command
and identifies the bad field if parsing fails.

| Key | Action |
| --- | --- |
| `Tab` | Accept a contact suggestion, otherwise next field; in Body, insert a tab |
| `Shift+Tab` | Previous field, including leaving Body |
| Left / Right while From is selected | Select sending account |
| `h` / `l`, `j` / `k`, or Enter on From | Change sending account |
| `Ctrl+A` | Add an attachment by file path |
| `Ctrl+G` | Cycle OpenPGP mode: Off → Sign → Encrypt → Sign+Encrypt |
| `Ctrl+S` | Build/protect the MIME message and run the selected account's send command |
| `Esc` | Close suggestions first, otherwise cancel composition |
| Arrow keys | Move the cursor in normal input/body fields |

## Calendar attachments and invitations

Calendar attachments (`.ics`, `text/calendar`, and common calendar MIME types)
and inline calendar parts without a filename are detected when a message is
opened. Event details appear in the preview. Press `i`, or click the **Calendar
attachment** button, to choose an event and destination. Use Tab/Left/Right to
choose a list and Up/Down to select. Click or Tab to the details pane and use arrows or the mouse wheel
to scroll long descriptions. Press Ctrl+S/Enter or click **Add to
calendar**; Esc returns to mail. The selected account's calendars and explicitly
shared calendars are available.

Import saves locally and uploads on the next sync. Repeating an import keeps an
existing UID unchanged. This does not accept/decline or send an RSVP. See
[calendar import formats and limits](contacts-calendar.md#incoming-calendar-attachments)
for CalDAV/JMAP behavior. `a` still saves the original attachment, including
malformed calendar files that cannot be previewed.

## Contact suggestions

Type a name or part of an email address in To/Cc/Bcc. Up/Down selects a match;
Enter, Tab, or a click inserts it. The next Tab advances the field after an
address is complete. Suggestions use the compose account’s contact collections
and shared books, including aliases. Changing From reloads the contacts.
See [contact saving and collection setup](contacts-calendar.md).

## Selecting text

Text selection works in To/Cc/Bcc/Subject, the message body, search, attachment
paths, contact/calendar creation fields, and the full-source editor. Selection
uses your theme's `selected_fg` and `selected_bg` colors.

| Key or action | Behavior |
| --- | --- |
| Shift+Left/Right | Extend or shrink the selection by a character |
| Shift+Up/Down | Extend the selection across lines in a multiline editor |
| Shift+Home/End | Select to the start/end of the current line |
| Ctrl+Shift+Home/End | Select to the start/end of the entire text |
| Alt+A | Select all text in the current editable field |
| Ctrl+Space, then arrows | Start a keyboard selection; Ctrl+Space again clears it |
| Left mouse drag | Select text; a click places the cursor |
| Type or terminal paste | Replace the selected text |
| Backspace/Delete | Remove the selected text |
| Left/Right without Shift | Collapse the selection to its start/end |

Ctrl+A continues to attach a file in the composer. It also selects all in
editors where it has no configured action, such as search and the attachment
path prompt. Existing configured actions take precedence. Ctrl+C retains its
quit behavior. Selection edits the local field; it does not copy to the system
clipboard. Use Ctrl+Space if your terminal intercepts Shift+arrow shortcuts.

The From/Reply from field selects an account identity and is not a text editor.
Raw contact/calendar source scrolls horizontally without adding wrap newlines.

## Mouse controls

- Click the **1 Mail**, **2 Contacts**, or **3 Calendar** tab to change views.
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
body. When OpenPGP is enabled for the active account, encrypted message bodies
are decrypted as needed for body searching. Press Enter to apply the filter,
Escape to cancel the prompt, or submit an empty search to restore the full
folder.

Archiving uses the active account's configured `archive_folder` (default `Archive`). MailSalon only enables and advertises Archive when that Maildir is actually present; it does not create the folder automatically. A manually created ordinary Maildir named `Archive` works, as does a Maildir++ `.Archive` folder; no server-side Archive role is required. The move preserves the message's Maildir Seen/unread state so the external sync tool can propagate the move.

Deletion uses the active account's configured Trash Maildir. If that folder is
not present, MailSalon refuses to delete instead of guessing a path. Deleting a
message already in Trash permanently removes the file.

For creating and editing contacts or events, use the
[contacts and calendars guide](contacts-calendar.md#controls).
