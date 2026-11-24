// SPDX-License-Identifier: GPL-3.0-only

// Package gui owns the Fyne interface. Mutable interface state belongs to the
// Fyne event goroutine; workers capture inputs and return results through Do.
package gui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/SeraphinaDX/MailSalonGUI/internal/assets"
	"github.com/SeraphinaDX/MailSalonGUI/internal/version"

	"github.com/SeraphinaDX/MailSalonGUI/internal/config"
	"github.com/SeraphinaDX/MailSalonGUI/internal/maildir"
	"github.com/SeraphinaDX/MailSalonGUI/internal/mailtime"
	"github.com/SeraphinaDX/MailSalonGUI/internal/mimeutil"
	"github.com/SeraphinaDX/MailSalonGUI/internal/pgp"
	"github.com/SeraphinaDX/MailSalonGUI/internal/transport"
)

const Version = version.GUI

type App struct {
	Fyne                               fyne.App
	Window                             fyne.Window
	cfg                                config.Config
	configPath                         string
	ctx                                context.Context
	cancel                             context.CancelFunc
	account                            int
	folders                            []maildir.Folder
	folderList                         *widget.List
	mailDrag                           *mailDrag
	folderHover                        *folderRow
	folderHoverPath                    string
	folderManager                      *folderManager
	folderBusy                         bool
	folderCommand                      func(context.Context, config.Account, transport.FolderRequest) (transport.FolderResponse, error)
	messageList                        *messageList
	previewScroll                      *container.Scroll
	pendingSelectionRestore            *messageSelectionSnapshot
	pendingPreviewOffset               *fyne.Position
	messageMenu                        *widget.PopUpMenu
	messageMenuItems                   map[string]*fyne.MenuItem
	all, messages                      []maildir.Entry
	folderPath                         string
	selected                           int
	parsed                             *mimeutil.ParsedMessage
	status, summary, headers, security *widget.Label
	body                               *widget.RichText
	bodyText                           *widget.Label
	formattedView                      *widget.Check
	attachments                        *fyne.Container
	search                             *widget.Entry
	tabs                               *container.AppTabs
	contacts, calendar                 *collectionView
	syncButton                         *widget.Button
	progress                           *widget.ProgressBarInfinite
	syncing, changing                  bool
	closing                            bool
	changingRead                       bool
	pendingDelete                      func()
	generation, reading                uint64
	composers                          map[*composer]bool
	log                                string
	draftDir                           string
	// dispatch is replaceable by synchronous dispatch in software-driver tests.
	dispatch func(func())
}

func New(f fyne.App, cfg config.Config, path, draftDir string) *App {
	ctx, cancel := context.WithCancel(context.Background())
	a := &App{Fyne: f, cfg: cfg, configPath: path, ctx: ctx, cancel: cancel, selected: -1, composers: map[*composer]bool{}, draftDir: draftDir, dispatch: fyne.Do}
	a.account = cfg.AccountIndex(cfg.DefaultAccount)
	if a.account < 0 {
		a.account = 0
	}
	f.Settings().SetTheme(salonTheme{Theme: theme.DefaultTheme(), colors: cfg.Theme})
	f.SetIcon(assets.Icon)
	a.Window = f.NewWindow("MailSalonGUI")
	a.Window.Resize(fyne.NewSize(1280, 820))
	a.status = widget.NewLabel("Ready")
	a.status.Truncation = fyne.TextTruncateEllipsis
	a.summary = widget.NewLabel("Choose a folder")
	a.headers = widget.NewLabel("Select a message to read it")
	a.headers.Wrapping = fyne.TextWrapWord
	a.headers.Selectable = true
	a.security = widget.NewLabel("")
	a.security.Wrapping = fyne.TextWrapWord
	a.security.Selectable = true
	a.body = widget.NewRichTextWithText("")
	a.body.Wrapping = fyne.TextWrapWord
	// RichText supports links and formatting, but not selecting text. Keep a
	// separate selectable view as the default so copying part of a message works.
	a.body.Hide()
	a.bodyText = widget.NewLabel("")
	a.bodyText.Wrapping = fyne.TextWrapWord
	a.bodyText.Selectable = true
	a.formattedView = widget.NewCheck("Formatted view (links)", func(formatted bool) {
		if formatted {
			a.bodyText.Hide()
			a.body.Show()
		} else {
			a.body.Hide()
			a.bodyText.Show()
		}
	})
	a.attachments = container.NewVBox()
	a.search = widget.NewEntry()
	a.search.SetPlaceHolder("Search this folder — sender or subject")
	a.search.OnChanged = func(string) { a.filterMessages() }
	a.folderList = widget.NewList(func() int { return len(a.folders) }, func() fyne.CanvasObject { return newFolderRow(a) }, func(id widget.ListItemID, o fyne.CanvasObject) {
		row := o.(*folderRow)
		row.id = -1
		if id >= 0 && id < len(a.folders) {
			row.id = id
			row.label.SetText(a.folders[id].Name)
			row.path = a.folders[id].Path
		}
		if a.folderHover == row && row.path != a.folderHoverPath {
			a.hoverFolder(nil)
		}
	})
	a.folderList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(a.folders) {
			a.loadFolder(a.folders[id])
		}
	}
	a.messageList = newMessageList(func() int { return len(a.messages) }, func() fyne.CanvasObject {
		row := newMessageRow(a.selectMessage, a.showMessageMenu)
		row.onDragStart, row.onDragMove, row.onDragEnd = a.startMailDrag, a.updateMailDrag, a.finishMailDrag
		return row
	}, func(id int, o fyne.CanvasObject) {
		row := o.(*messageRow)
		row.id = -1
		row.selected = false
		if id < 0 || id >= len(a.messages) {
			row.Refresh()
			return
		}
		row.id = id
		row.selected = a.messageList.selection[id]
		row.Refresh()
		e := a.messages[id]
		row.subject.TextStyle = fyne.TextStyle{Bold: e.Unread}
		text := e.Subject
		if text == "" {
			text = "(no subject)"
		}
		if e.Unread {
			text = "●  " + text
		}
		row.subject.SetText(text)
		row.meta.SetText(e.From + "  ·  " + mailtime.InLocation(e.Date, a.cfg.MailLocation()).Format("Jan 02 15:04"))
	}, a.deleteMessage)
	a.messageList.onSelected = a.openMessage
	a.messageList.onRune = a.handleMessageRune
	a.messageList.onCancelDrag = a.cancelMailDrag
	a.messageList.onShortcut = func(shortcut fyne.Shortcut) {
		if canvas, ok := a.Window.Canvas().(fyne.Shortcutable); ok {
			canvas.TypedShortcut(shortcut)
		}
	}
	a.messageList.onSelectionChanged = func() {
		a.cancelMailDrag()
		a.hideMessageMenu()
		a.updateSummary()
		if len(a.messageList.selection) == 0 {
			a.clearPreview()
		}
	}
	readActions := container.NewHBox(
		widget.NewButtonWithIcon("Reply", theme.MailReplyIcon(), func() { a.reply(false, false) }),
		widget.NewButton("Reply all", func() { a.reply(false, true) }),
		widget.NewButtonWithIcon("Forward", theme.MailForwardIcon(), func() { a.reply(true, false) }),
		widget.NewButton("Read / unread", a.toggleRead),
	)
	manageActions := container.NewHBox(widget.NewButton("Archive", a.archive), widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), a.deleteMessage), a.formattedView, widget.NewButton("Copy body", func() {
		if a.parsed != nil {
			a.Window.Clipboard().SetContent(a.parsed.DisplayText())
		}
	}), widget.NewButton("Source", a.showSource))
	a.previewScroll = container.NewVScroll(container.NewVBox(a.bodyText, a.body, a.attachments))
	preview := container.NewBorder(container.NewVBox(readActions, manageActions, widget.NewSeparator(), a.headers, a.security), nil, nil, nil, a.previewScroll)
	reader := container.NewHSplit(container.NewBorder(a.search, a.summary, nil, nil, a.messageList), preview)
	reader.Offset = 0.40
	folderHeader := container.NewBorder(nil, nil, widget.NewLabelWithStyle("Folders", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() { a.showFolderManager(true) }))
	mailPane := container.NewHSplit(container.NewBorder(folderHeader, widget.NewButton("Manage folders…", func() { a.showFolderManager(false) }), nil, nil, a.folderList), reader)
	mailPane.Offset = 0.17
	a.contacts = a.newCollectionView(true)
	a.calendar = a.newCollectionView(false)
	a.tabs = container.NewAppTabs(container.NewTabItemWithIcon("Mail", theme.MailComposeIcon(), mailPane), container.NewTabItemWithIcon("Contacts", theme.AccountIcon(), a.contacts.content), container.NewTabItemWithIcon("Calendar", theme.CalendarIcon(), a.calendar.content))
	a.tabs.OnSelected = func(item *container.TabItem) {
		a.cancelMailDrag()
		switch item.Text {
		case "Contacts":
			a.contacts.reloadCollections()
		case "Calendar":
			a.calendar.reloadCollections()
		}
	}
	names := []string{}
	for _, acc := range cfg.Accounts {
		names = append(names, acc.Name)
	}
	accounts := widget.NewSelect(names, nil)
	accounts.SetSelectedIndex(a.account)
	accounts.OnChanged = func(name string) {
		idx := cfg.AccountIndex(name)
		if idx >= 0 && idx != a.account {
			a.account = idx
			a.reload()
			a.contacts.reloadCollections()
			a.calendar.reloadCollections()
		}
	}
	a.syncButton = widget.NewButtonWithIcon("Sync", theme.ViewRefreshIcon(), a.sync)
	brand := widget.NewLabelWithStyle("MailSalonGUI", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	toolbar := container.NewBorder(nil, nil, container.NewHBox(brand, accounts), nil, container.NewHBox(
		widget.NewButtonWithIcon("Compose", theme.MailComposeIcon(), func() { a.compose(mimeutil.Draft{}, a.account, "") }), a.syncButton,
		widget.NewButton("Reload", func() { a.reload(); a.contacts.reloadCollections(); a.calendar.reloadCollections() }), widget.NewButton("Drafts", a.showDrafts), widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), a.settings),
	))
	a.progress = widget.NewProgressBarInfinite()
	a.progress.Hide()
	a.Window.SetContent(container.NewBorder(container.NewVBox(toolbar, widget.NewSeparator()), container.NewVBox(a.progress, a.status), nil, nil, a.tabs))
	a.Window.SetMainMenu(fyne.NewMainMenu(fyne.NewMenu("File", fyne.NewMenuItem("Compose", func() { a.compose(mimeutil.Draft{}, a.account, "") }), fyne.NewMenuItem("Saved drafts", a.showDrafts), fyne.NewMenuItem("Settings", a.settings), fyne.NewMenuItemSeparator(), fyne.NewMenuItem("Quit", a.requestClose)), fyne.NewMenu("Help", fyne.NewMenuItem("About", func() {
		dialog.ShowInformation("MailSalonGUI "+Version, "A Go/Fyne mail, contacts and calendar client.\nLocal Maildirs • External sync/send • GPL-3.0\n\nConfiguration: "+a.configPath, a.Window)
	}), fyne.NewMenuItem("Keyboard shortcuts", a.showKeyboardShortcuts), fyne.NewMenuItem("Keyboard diagnostic", func() {
		newKeyboardInspectorWindow(a.Fyne, a.messageList.keypadDeleteScanCode)
	}))))
	a.Window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyN, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) { a.compose(mimeutil.Draft{}, a.account, "") })
	a.Window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) { a.reply(false, false) })
	a.Window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) { a.Window.Canvas().Focus(a.search); a.tabs.SelectIndex(0) })
	a.Window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF5}, func(fyne.Shortcut) { a.sync() })
	a.Window.SetCloseIntercept(a.requestClose)
	return a
}

func (a *App) Start() {
	a.reload()
	a.contacts.reloadCollections()
	a.calendar.reloadCollections()
	if a.cfg.StartupSync {
		a.sync()
	}
	if a.cfg.SyncInterval > 0 {
		go func() {
			ticker := time.NewTicker(a.cfg.SyncInterval)
			defer ticker.Stop()
			for {
				select {
				case <-a.ctx.Done():
					return
				case <-ticker.C:
					a.post(a.sync)
				}
			}
		}()
	}
}
func (a *App) post(fn func()) {
	if a.ctx.Err() == nil {
		a.dispatch(func() {
			if a.ctx.Err() == nil {
				fn()
			}
		})
	}
}
func (a *App) fail(err error) {
	if err != nil {
		a.status.SetText("Error: " + err.Error())
		dialog.ShowError(err, a.Window)
	}
}
func (a *App) clearPreview() {
	a.hideMessageMenu()
	a.reading++
	a.selected = -1
	a.parsed = nil
	a.headers.SetText("Select a message to read it")
	a.security.SetText("")
	a.body.ParseMarkdown("")
	a.bodyText.SetText("")
	a.attachments.Objects = nil
	a.attachments.Refresh()
}
func (a *App) reload() {
	selection := a.snapshotMessageSelection()
	a.pendingSelectionRestore = nil
	a.pendingPreviewOffset = nil
	a.cancelMailDrag()
	a.hoverFolder(nil)
	a.generation++
	gen := a.generation
	acc := a.cfg.Accounts[a.account]
	previous := a.folderPath
	a.clearPreview()
	a.folders = nil
	a.all = nil
	a.messages = nil
	a.folderList.UnselectAll()
	a.messageList.UnselectAll()
	a.folderList.Refresh()
	a.messageList.Refresh()
	a.status.SetText("Loading folders…")
	go func() {
		folders, err := maildir.DiscoverFolders(acc.Maildir)
		a.post(func() {
			if gen != a.generation {
				return
			}
			if err != nil {
				a.status.SetText("Mailbox unavailable — open Settings to check " + acc.Maildir)
				return
			}
			a.folders = folders
			a.folderList.Refresh()
			if len(folders) == 0 {
				a.summary.SetText("No Maildir folders found")
				a.status.SetText("Run your sync tool to populate " + acc.Maildir)
				return
			}
			index := 0
			for i, f := range folders {
				if f.Path == previous {
					index = i
				}
			}
			if previous == folders[index].Path {
				a.pendingSelectionRestore = selection
			}
			a.folderList.Select(index)
		})
	}()
}
func (a *App) loadFolder(folder maildir.Folder) {
	selection := a.pendingSelectionRestore
	a.pendingSelectionRestore = nil
	if selection != nil && selection.folderPath != folder.Path {
		selection = nil
	}
	a.cancelMailDrag()
	a.generation++
	gen := a.generation
	a.folderPath = folder.Path
	a.clearPreview()
	a.all = nil
	a.messages = nil
	a.messageList.UnselectAll()
	a.messageList.Refresh()
	a.summary.SetText("Loading…")
	go func() {
		entries, err := maildir.Scan(folder)
		a.post(func() {
			if gen != a.generation {
				return
			}
			if err != nil {
				a.fail(err)
				a.summary.SetText("Unable to load folder")
				return
			}
			a.all = entries
			a.filterMessages()
			a.restoreMessageSelection(selection)
			a.status.SetText("Ready — " + folder.Name)
		})
	}()
}
func (a *App) filterMessages() {
	selectedPaths := map[string]bool{}
	for _, e := range a.selectedEntries() {
		selectedPaths[e.Path] = true
	}
	oldPath := ""
	if a.selected >= 0 && a.selected < len(a.messages) {
		oldPath = a.messages[a.selected].Path
	}
	q := strings.ToLower(strings.TrimSpace(a.search.Text))
	a.messages = nil
	for _, e := range a.all {
		if q == "" || strings.Contains(strings.ToLower(e.From+" "+e.Subject), q) {
			a.messages = append(a.messages, e)
		}
	}
	a.messageList.UnselectAll()
	a.clearPreview()
	a.messageList.Refresh()
	a.updateSummary()
	active := -1
	for i, e := range a.messages {
		if selectedPaths[e.Path] {
			a.messageList.selection[i] = true
			if active < 0 || e.Path == oldPath {
				active = i
			}
		}
	}
	if active >= 0 {
		a.messageList.anchor = active
		a.messageList.focusSelection(active)
	}
}
func (a *App) openMessage(id int) { a.loadMessage(id, true) }
func (a *App) loadMessage(id int, autoRead bool) {
	if id < 0 || id >= len(a.messages) || (a.changing && !a.changingRead) {
		return
	}
	a.hideMessageMenu()
	restoreOffset := a.pendingPreviewOffset
	a.pendingPreviewOffset = nil
	a.reading++
	token := a.reading
	gen := a.generation
	acc := a.cfg.Accounts[a.account]
	account := a.account
	e := a.messages[id]
	a.selected = id
	a.parsed = nil
	a.headers.SetText("Loading message…")
	a.body.Segments = nil
	a.body.Refresh()
	a.bodyText.SetText("")
	a.attachments.Objects = nil
	a.attachments.Refresh()
	// No automatic mutation while parsing: a stale result must not mark a message
	// read after the user has changed folders or selected another message.
	go func() {
		raw, err := mimeutil.ReadMessageFile(e.Path)
		var p *mimeutil.ParsedMessage
		var info pgp.Info
		if err == nil {
			ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
			defer cancel()
			raw, info = pgp.ProcessIncoming(ctx, raw, pgpSettings(acc))
			p, err = mimeutil.ParseBytes(raw)
		}
		a.post(func() {
			if token != a.reading || gen != a.generation {
				return
			}
			if err != nil {
				a.fail(err)
				return
			}
			a.parsed = p
			a.headers.SetText(fmt.Sprintf("%s\nFrom: %s\nTo: %s\nCc: %s\nDate: %s", p.Subject, p.From, p.To, p.Cc, mailtime.DisplayHeaderDate(p.Date, a.cfg.MailLocation())))
			a.security.SetText(info.Summary())
			a.body.Segments = messageBodySegments(p)
			a.body.Refresh()
			a.bodyText.SetText(p.DisplayText())
			for _, attachment := range p.Attachments {
				att := attachment
				if card := a.calendarAttachment(att, p, account); card != nil {
					a.attachments.Add(card)
				}
				a.attachments.Add(widget.NewButtonWithIcon(fmt.Sprintf("Save %s (%d bytes)", att.Filename, len(att.Data)), theme.DownloadIcon(), func() { a.saveAttachment(att, acc.DownloadDir) }))
			}
			a.Window.Content().Refresh()
			if restoreOffset != nil && a.previewScroll != nil {
				a.previewScroll.ScrollToOffset(*restoreOffset)
			}
			if autoRead && e.Unread && len(a.messageList.selection) == 1 && a.messageList.selection[id] {
				a.changeRead(e, true)
			}
			a.updateMessageMenu()
		})
	}()
}
func (a *App) changeRead(e maildir.Entry, read bool) {
	if a.changing {
		return
	}
	a.changing = true
	a.changingRead = true
	a.updateMessageMenu()
	gen := a.generation
	old := e.Path
	go func() {
		var err error
		if read {
			err = maildir.MarkRead(&e)
		} else {
			err = maildir.MarkUnread(&e)
		}
		a.post(func() {
			a.changing = false
			a.changingRead = false
			pending := a.pendingDelete
			a.pendingDelete = nil
			defer a.updateMessageMenu()
			if err != nil {
				a.fail(err)
				return
			}
			if gen != a.generation {
				return
			}
			for i := range a.all {
				if a.all[i].Path == old {
					a.all[i] = e
				}
			}
			for i := range a.messages {
				if a.messages[i].Path == old {
					a.messages[i] = e
				}
			}
			a.messageList.Refresh()
			a.updateSummary()
			if pending != nil {
				pending()
			}
		})
	}()
}
func (a *App) currentEntry() (maildir.Entry, bool) {
	if a.changing || a.selected < 0 || a.selected >= len(a.messages) {
		return maildir.Entry{}, false
	}
	return a.messages[a.selected], true
}
func (a *App) reply(forward, all bool) {
	if a.parsed == nil {
		return
	}
	idx := a.account
	if !forward {
		idx = preferredAccount(a.cfg.Accounts, idx, a.parsed)
	}
	p := a.parsed
	a.compose(replyDraftInLocation(p, forward, all, a.cfg.Accounts, a.cfg.MailLocation()), idx, "")
	if !forward && a.cfg.AutoAddReplyContacts {
		a.saveReplyContact(p.From, idx)
	}
}
func (a *App) sync() {
	if a.closing {
		return
	}
	a.syncThen(nil)
}

func (a *App) syncThen(after func(bool)) {
	if a.syncing || a.folderBusy {
		return
	}
	commands := []string{}
	seen := map[string]bool{}
	for _, acc := range a.cfg.Accounts {
		cmd := strings.TrimSpace(acc.ReceiveCommand)
		if cmd != "" && !seen[cmd] {
			seen[cmd] = true
			commands = append(commands, cmd)
		}
	}
	if len(commands) == 0 {
		a.status.SetText("No receive command configured — reloading local files")
		a.reload()
		if after != nil {
			after(true)
		}
		return
	}
	a.syncing = true
	a.syncButton.Disable()
	a.progress.Show()
	a.progress.Start()
	a.status.SetText("Syncing all accounts…")
	go func() {
		var failures []string
		var logs []string
		for _, cmd := range commands {
			ctx, cancel := context.WithTimeout(a.ctx, 10*time.Minute)
			out, err := transport.Receive(ctx, cmd)
			cancel()
			logs = append(logs, cmd+"\n"+out)
			if err != nil {
				failures = append(failures, err.Error())
			}
			if a.ctx.Err() != nil {
				return
			}
		}
		a.post(func() {
			a.syncing = false
			a.syncButton.Enable()
			a.progress.Stop()
			a.progress.Hide()
			a.log = strings.Join(logs, "\n\n")
			a.reload()
			a.contacts.reloadCollections()
			a.calendar.reloadCollections()
			if len(failures) > 0 {
				dialog.ShowInformation("Sync failed", strings.Join(failures, "\n")+"\n\n"+a.log, a.Window)
			}
			if after != nil {
				after(len(failures) == 0)
			}
		})
	}()
}
func (a *App) requestClose() {
	if a.closing {
		return
	}
	for c := range a.composers {
		if c.sending {
			dialog.ShowInformation("Sending in progress", "Wait for delivery to finish before closing MailSalonGUI.", a.Window)
			return
		}
	}
	if len(a.composers) > 0 {
		dialog.ShowConfirm("Close MailSalonGUI?", "Save all open compositions as local drafts and quit?", func(ok bool) {
			if ok {
				for c := range a.composers {
					if !c.save() {
						return
					}
				}
				a.close()
			}
		}, a.Window)
		return
	}
	a.close()
}
func (a *App) close() {
	if a.closing {
		return
	}
	a.closing = true
	waiting := dialog.NewCustomWithoutButtons("Syncing before exit", widget.NewLabel("Waiting for pending work and syncing all accounts…"), a.Window)
	waiting.Show()
	var finish func()
	finish = func() {
		if a.syncing || a.changing || a.folderBusy {
			time.AfterFunc(50*time.Millisecond, func() { a.post(finish) })
			return
		}
		a.syncThen(func(ok bool) {
			waiting.Hide()
			if !ok {
				a.closing = false
				return
			}
			a.finishClose()
		})
	}
	finish()
}

func (a *App) finishClose() {
	a.cancel()
	for c := range a.composers {
		c.window.SetCloseIntercept(nil)
		c.window.Close()
	}
	a.Window.SetCloseIntercept(nil)
	a.Window.Close()
}
func (a *App) saveAttachment(att mimeutil.Attachment, dir string) {
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil {
			a.fail(err)
			return
		}
		if w == nil {
			return
		}
		go func() {
			_, err := w.Write(att.Data)
			ce := w.Close()
			if err == nil {
				err = ce
			}
			a.post(func() {
				if err != nil {
					a.fail(err)
				} else {
					a.status.SetText("Attachment saved")
				}
			})
		}()
	}, a.Window)
	name := filepath.Base(strings.ReplaceAll(att.Filename, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		name = "attachment"
	}
	d.SetFileName(name)
	setDialogDirectory(d, dir)
	d.Show()
}
func (a *App) showSource() {
	e, ok := a.currentEntry()
	if !ok {
		return
	}
	go func() {
		raw, err := mimeutil.ReadMessageFile(e.Path)
		a.post(func() {
			if err != nil {
				a.fail(err)
				return
			}
			w := a.Fyne.NewWindow("Message source")
			text := widget.NewMultiLineEntry()
			text.SetText(string(raw))
			text.Disable()
			w.SetContent(container.NewBorder(nil, widget.NewButton("Copy source", func() { w.Clipboard().SetContent(string(raw)) }), nil, nil, text))
			w.Resize(fyne.NewSize(800, 600))
			showWindow(w)
		})
	}()
}
