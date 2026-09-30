# Contacts and calendars

The highlighted view bar at the top shows **1 Mail**, **2 Contacts**, and
**3 Calendar**. Press a shortcut or click a tab to switch views. The selected
account stays the same when you switch views.
Contacts and calendars use local files synchronized by MailSalonSync 0.6.0.
Network credentials and sync state stay with the sync tool.

## Configuration

Keep your existing `[[accounts]]` configuration and add top-level collections:

```toml
[[collections]]
name = "Personal contacts"
account = "personal"
protocol = "carddav"
local_dir = "~/PIM/contacts/personal"

[[collections]]
name = "Personal calendar"
account = "personal"
protocol = "caldav"
local_dir = "~/PIM/calendars/personal"
```

`account` must match the `name` in the associated MailSalon `[[accounts]]`
block (not the MailSalonSync account name or JMAP object ID). Set it on each
account’s own address books and calendars. Switching accounts then changes
which collections and items are visible.

Omit `account` only for a collection you intend to share across every account.
These appear with a `Shared:` label and “shared across accounts” in the footer;
they remain visible when switching accounts. Account-bound collections show
“ACCOUNT only” in the footer. `local_dir` must match the corresponding MailSalonSync
collection exactly; `protocol` must match its stored format. Each collection
directory contains individual items rather than one concatenated export.

| Protocol | Mode | Item files |
| --- | --- | --- |
| `carddav` | Contacts | `.vcf` |
| `caldav` | Calendar | `.ics` |
| `jmap-contacts` | Contacts | Native ContactCard/JSContact `.json` |
| `jmap-calendars` | Calendar | Native CalendarEvent/JSCalendar `.json` |

For JMAP, use the same entries with `protocol = "jmap-contacts"` or
`"jmap-calendars"` and point at their JSON directories. There is no conversion
between JSON and DAV file formats. For remote configuration/discovery, see
[MailSalonSync's guide](https://github.com/SeraphinaDX/MailSalonSync/blob/main/docs/contacts-calendar.md).

Set `receive = "MailSalonSync -plain sync"` in the associated `[[accounts]]`
block. Manual `u` and the existing `[options] sync_interval = "5m"` then run
mail/contact/calendar sync together. If you use selectors, include every PIM
collection you want, for example:

```toml
receive = "MailSalonSync -plain sync -account personal-jmap -collection personal-contacts,personal-calendar"
```

## Contact autocomplete and reply saving

To/Cc/Bcc suggest names and all stored email addresses from the compose account's
contact collections plus shared ones. Use Up/Down and Enter/Tab, or click a match.
Only the recipient around the cursor is replaced; existing recipients are kept.
Esc closes the suggestions. Switching the From account reloads its contacts.

By default, opening a reply saves its sender to the first contact collection
associated with the reply account (or the first shared contact collection if
none is associated). The person is identified by email, including aliases,
case-insensitively. Existing contacts are retained unchanged. If saving fails or
no contact collection is configured, the status explains why and the reply
continues. The saved contact remains even if you cancel the draft; the next sync
uploads it. New messages and forwards do not automatically add contacts.

Disable this in the existing `[options]` section:

```toml
auto_add_reply_contacts = false
```

Autocomplete still works when automatic contact saving is disabled.

## Controls

| Key | Action |
| --- | --- |
| `1` / `2` / `3` | Mail / Contacts / Calendar |
| `A` | Switch mail account and its visible collections |
| `Tab`, arrows, `h` / `l` | Focus collection list, item list, or preview |
| `j` / `k`, arrows, PgUp/PgDn, Home/End | Navigate the focused pane |
| `/` | Search names, email, phone, dates, location, and notes |
| `n` | New contact or single event in the selected collection |
| `e` | Edit the complete native source of the selected item |
| `c` | Compose mail to the selected contact's first email address |
| `d`, then `d` again | Delete locally, retaining a `.mss-trash` backup |
| `u` | Run the account's receive/sync command |
| `R` | Reload local files |
| `q` | Quit |

The Mail/Contacts/Calendar bar is clickable. Mouse clicks and wheel navigation
work in the lists and preview. New-item forms
use Tab/Shift+Tab or Enter to move fields. Ctrl+S saves locally; Esc cancels.
Only the active field shows a cursor. Source editing uses the multiline editor
and preserves fields beyond those shown in previews. Its Ctrl+S saves locally;
Esc cancels. UID changes are rejected. Collection locks and comparison against
the original content prevent stale editors from overwriting sync downloads.

In `[keybindings]`, `mail_view`, `contacts_view`, and `calendar_view` change the
mode keys. Existing navigation, compose, archive (`Edit source` in PIM views),
delete, search, sync, refresh, account, send/save, and cancel bindings apply in
these views too. `n` remains the fixed new-item shortcut in PIM views.

## New contacts and events

Contact forms accept name, email, and phone. They create vCard 4.0 or JSContact
1.0 depending on the collection. For additional addresses, organizations,
birthdays, groups, or notes, edit the full source afterward.

Event forms accept title, start, end, optional IANA time zone, and location.
Use `2026-10-01T10:00:00` for timed events or `2026-10-01` for all-day events.
The all-day end date is exclusive: Oct 1 through Oct 2 means one day on Oct 1.
An empty time zone creates a floating timed event. For DAV, a supplied time zone
is converted to UTC instants; JMAP stores the IANA zone and duration. The form
creates an ordinary single event, without attendees or recurrence rules.

Saves remain local until the next sync. Newly created items upload automatically.
To propagate local deletion remotely, set `propagate_deletes = true` on the
corresponding **MailSalonSync** collection. Its default is false, in which case
sync restores locally deleted tracked files. Invalid source edits are rejected
for malformed containers/JSON or missing UID; the server performs detailed
protocol validation when uploading.

## Calendar view in this increment

The calendar mode is a sortable list of stored items and a detail/source preview,
not yet a month/week grid or an occurrence-expanding agenda. Recurring resources
are marked `[recurring]` and their native recurrence data is retained. The list
shows the series' stored start, not every occurrence. Native time zone and
all-day information is shown rather than silently interpreted as local time.
Existing CalDAV VTODO files are readable in the calendar view, but there is no
task creation form. Reminder notifications, invitation handling/RSVP, free/busy,
contact autocomplete in arbitrary recipient fields, and structured editing of
all existing properties remain future increments.

