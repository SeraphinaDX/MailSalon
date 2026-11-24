# MailSalon and MailSalonGUI

One Go codebase with two interfaces: **MailSalon**, the gotui terminal client
(version **0.10.0**), and **MailSalonGUI**, the Fyne desktop client (version **0.10.0**).
Both use the same Maildir, MIME, TOML, transport, OpenPGP and contacts/calendar
packages. Each retains its own executable, configuration path and interface.

```sh
make build-tui   # MailSalon; pure Go, no graphics dependencies
make build-gui   # MailSalonGUI; needs the desktop dependencies below
make build-all   # Both executables
```

`make build` continues to build the GUI. Terminal setup and controls are in
[the terminal guide](docs/terminal/README.md); repository/config migration is
explained in [Consolidation](docs/consolidation.md).

![MailSalonGUI mail view](docs/screenshots/mail.png)

## Build and try it

Install Go 1.26 or newer and the desktop dependencies described by
[Fyne's quick start](https://docs.fyne.io/started/quick/).
Fyne's desktop graphics driver needs cgo and a C compiler.

On CachyOS / Arch:

```sh
sudo pacman -S --needed go base-devel pkgconf libglvnd libxcursor libxrandr libxinerama libxi libxkbcommon wayland
```

On Debian / Ubuntu:

```sh
sudo apt install build-essential pkg-config libgl1-mesa-dev xorg-dev libxkbcommon-dev libwayland-dev
```

From the extracted `MailSalonGUI` directory:

```sh
make build
./MailSalonGUI -demo
```

The build script automatically prefers `zgo` when it is installed. To require
it explicitly, use `make build ZGO=zgo`. It otherwise uses Go and your configured
C compiler. The `Makefile` invokes the script with `sh`, so an archive that loses
executable permissions does not break the build.

Without Make:

```sh
zgo build -buildvcs=false -o MailSalonGUI ./cmd/MailSalonGUI
# Or with your normal cgo compiler:
go build -buildvcs=false -o MailSalonGUI ./cmd/MailSalonGUI
```

`-demo` creates an offline temporary Maildir, two contacts and one event. It
never reads your real mailbox and has no send or receive commands. Demo drafts,
settings and messages are removed on normal exit. Saved attachments go to the
location you choose in the file dialog.

## Desktop icon and launcher

The envelope icon is embedded in the app, including compose and other windows.
Rebuild and restart to use it when launching the binary directly.

For a Linux application-menu entry and an icon you can pin to your taskbar:

```sh
make install
```

This builds and installs for your user under `~/.local`, with no sudo required.
Launch **MailSalonGUI** from your application menu, then pin that launcher.
The launcher references the installed icon directly. On KDE, installation
refreshes the application cache when `kbuildsycoca6` or `kbuildsycoca5` is
available. X11/XWayland windows identify their desktop launcher explicitly,
including compose and other windows.
If KDE still shows an old paper or generic-X icon, close the app, remove its
old taskbar pin, run `make install`, then launch and pin the application-menu
entry again.
Desktop environments control the final taskbar display; native Wayland
behavior depends on the Fyne/GLFW backend and compositor.

`make uninstall` removes these installed files. For another prefix, use
`make install PREFIX=/usr/local` with the necessary filesystem permissions.
`DATADIR` overrides the icon/launcher directory (for example your
`XDG_DATA_HOME`), and `DESTDIR` supports staged package installation.
The launcher uses an absolute executable path, so `~/.local/bin` need not be
in your PATH. Normal configuration lookup remains the same.

Fyne packaging metadata is included in `cmd/MailSalonGUI/FyneApp.toml` for
platform packages made with the Fyne CLI.

## Connect your mail

Reuse an existing MailSalon configuration directly:

```sh
./MailSalonGUI -config=~/.config/mailsalon/config.toml
```

Or create a separate GUI configuration:

```sh
./MailSalonGUI -init-config
./MailSalonGUI
```

Open **Settings**, replace the example identity and sync account, then restart
the app. The settings editor validates TOML before replacing the file.
`-init-config` refuses to overwrite an existing file.

The default path comes from Go's `os.UserConfigDir()`:

| Platform | Default configuration |
| --- | --- |
| Linux | `$XDG_CONFIG_HOME/mailsalongui/config.toml`, normally `~/.config/mailsalongui/config.toml` |
| Windows | `%AppData%\mailsalongui\config.toml` |
| macOS | `~/Library/Application Support/mailsalongui/config.toml` |

Transport commands run through `/bin/sh -c` on Unix and `cmd.exe /C` on Windows.
Use commands appropriate to the machine. The current release was built and
checked on Linux; Windows/macOS native builds have not been validated.

See [Configuration](docs/configuration.md) for accounts, collections and themes,
and [Usage](docs/usage.md) for everyday workflows.

## Desktop features

- Resizable folder, message and preview panes; multiple account selector.
- Local Maildir and Maildir++ discovery, including container-style INBOX layouts.
- Selectable message headers and body text for mouse selection and clipboard copying, plus a toggle for formatted HTML previews with clickable links and image-button labels.
- Remote images remain unloaded; scripts and embedded web content are excluded.
- Sender/subject search, read/unread state, archive and two-stage Trash deletion.
- Multiple message selection with Ctrl-click, Shift-click and Ctrl+A.
- Drag a message or selected group onto an existing folder to move it.
- Create remote folders and manage subscriptions through MailSalonSync 0.7.0+, retaining cached mail on unsubscribe.
- Bulk read/unread, archive and confirmed deletion from the toolbar, keyboard and context menu.
- Message-list letter shortcuts: `r` reply, `Shift+r` reply all, `e` archive,
  `f` forward and more; `?` or Help → Keyboard shortcuts shows the keys.
- Separate compose windows with From, To, Cc, Bcc, signatures and attachments.
- Reply, reply all, Reply-To handling, threading headers and forward attachments.
- Automatic reply identity selection and a contact picker for recipient fields.
- Private local drafts that can be saved, reopened or deleted.
- Manual sync and a configurable periodic timer; duplicate receive commands run once per sync.
- Contacts and calendars: browse, search, create, edit original source and delete.
- Month, week, day and agenda calendar views, Today/date navigation, local/UTC
  display, overlapping appointments and recurring event expansion.
- CardDAV/CalDAV local vCard/iCalendar and JMAP JSContact/JSCalendar files.
- Calendar attachment and inline invitation previews; Add to Calendar for local
  CalDAV collections, and reviewed Accept/Decline reply drafts.
- Optional GnuPG PGP/MIME signing, encryption, decryption and verification status.
- TOML settings editor and configurable colors.

Receiving and sending are handled by **MailSalonSync**, `mbsync`, `msmtp`, or your
chosen external command. MailSalonGUI itself does not log into IMAP, JMAP,
CardDAV or CalDAV servers. Your sync tool handles remote changes and credentials.

Calendar views expand supported daily, weekly, monthly and yearly recurrences,
including exclusions and moved/cancelled instances. Events whose timezones or
recurrence forms cannot be displayed remain accessible through **Unplaced events**.
Source editing and deletion act on the entire stored series. Dragging/resizing
appointments, an occurrence-only editor and reminders are not implemented yet.
Accept/Decline prepares an iCalendar reply for the invited account;
sending the draft and adding the event locally are separate actions. iCalendar
imports require a CalDAV collection; conversion to JMAP calendar JSON and
automatic cancellation/delegation handling are not supported. Contact/event
editing uses the native source so fields not
shown in the simple creation form are retained. Mail search currently matches
sender and subject; contact suggestions use the picker rather than inline
completion. Terminal `[keybindings]` settings are accepted for config
compatibility; GUI shortcuts are fixed as documented.

The terminal client retains its keyboard/mouse controls, inline contact
completion, OpenPGP, invitation import and month/week/day/agenda calendar views.
Its import chooser can convert supported simple invitations to JMAP; complex
invitations require CalDAV to retain their source. Interface features can differ
while the underlying parser, recurrence engine and storage operations are shared.

## Checks

```sh
make test
make check
make test-tui  # Core and terminal tests with CGO_ENABLED=0
# Optional real GnuPG integration test, on a host that supports gpg-agent sockets:
MAILSALONGUI_TEST_GPG=1 go test ./internal/pgp
```

The normal tests use Fyne's software driver (`-tags ci`), so no display is needed.
They cover Maildir mutations, MIME and attachments, recipient handling, configs,
drafts, both interfaces, stale worker results and collection formats.
The real GnuPG round trip is opt-in because some sandboxed environments forbid
its agent sockets. No live mail accounts are contacted by the tests.

## Codebase

```text
cmd/MailSalonGUI/    Entry point and flags
internal/gui/       Fyne windows, mail view, compose, collections and settings
internal/config/    TOML loading, validation and embedded example
internal/maildir/   Local folder discovery, headers and Maildir operations
internal/mimeutil/  MIME parsing, structured HTML previews and message construction
internal/transport/ External sync/send commands
internal/pim/       Contact/calendar parsing and guarded local editing
internal/pgp/       Optional external GnuPG integration
internal/drafts/    Private local composition files
internal/demo/      Disposable demonstration data
```

See [Development](docs/development.md) and [Validation](docs/validation.md) for
worker ownership and the exact checks and limits of this release.

Backend code was adapted into this project from MailSalon; it is not a runtime
or module dependency. See [NOTICE](NOTICE) for the exact source commit.
License: [GNU GPL version 3](LICENSE).
