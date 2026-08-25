package uiapp

import (
	"context"
	"fmt"
	"image"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"

	"git.cerberusgames.ca/Starstreak/MailSalon/internal/config"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/maildir"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/mimeutil"
	"git.cerberusgames.ca/Starstreak/MailSalon/internal/transport"
)

type focus int

const (
	focusFolders focus = iota
	focusMessages
	focusPreview
)

type composeField int

const (
	composeFrom composeField = iota
	composeTo
	composeCc
	composeBcc
	composeSubject
	composeBody
)

type composeState struct {
	from               *widgets.Paragraph
	to                 *widgets.Input
	cc                 *widgets.Input
	bcc                *widgets.Input
	subject            *widgets.Input
	body               *widgets.TextArea
	field              composeField
	attachments        []string
	forwardAttachments []mimeutil.Attachment
	inReplyTo          string
	references         string
	account            int
	isReply            bool

	attachPrompt *widgets.Input
}

type resolvedTheme struct {
	background   ui.Color
	foreground   ui.Color
	muted        ui.Color
	border       ui.Color
	activeBorder ui.Color
	title        ui.Color
	selectedFG   ui.Color
	selectedBG   ui.Color
	account      ui.Color
	unread       ui.Color
	status       ui.Color
	error        ui.Color
	cursorFG     ui.Color
	cursorBG     ui.Color
}

type App struct {
	cfg     config.Config
	theme   resolvedTheme
	account int

	folders  []maildir.Folder
	messages []maildir.Entry
	parsed   *mimeutil.ParsedMessage

	selectedFolder  int
	selectedMessage int
	folderOffset    int
	messageOffset   int
	previewScroll   int
	focus           focus
	status          string
	deleteArmed     bool

	accountBar *widgets.Paragraph
	folderList *widgets.List
	messageTbl *widgets.Table
	preview    *widgets.Paragraph
	footer     *widgets.Paragraph

	compose *composeState
}

func New(cfg config.Config) (*App, error) {
	if len(cfg.Accounts) == 0 {
		return nil, fmt.Errorf("no accounts configured")
	}
	for _, account := range cfg.Accounts {
		if err := maildir.PrepareRoot(account.Maildir); err != nil {
			return nil, fmt.Errorf("prepare maildir for %s: %w", account.Name, err)
		}
	}

	theme, err := resolveTheme(themeWithDefaults(cfg.Theme))
	if err != nil {
		return nil, err
	}

	a := &App{
		cfg:             cfg,
		theme:           theme,
		account:         cfg.DefaultAccountIndex(),
		selectedFolder:  0,
		selectedMessage: 0,
		focus:           focusMessages,
		status:          "Ready",
	}
	a.accountBar = widgets.NewParagraph()
	a.accountBar.Title = "Account"
	a.accountBar.WrapText = false
	a.accountBar.BorderRounded = true

	a.folderList = widgets.NewList()
	a.folderList.Title = "Folders"
	a.folderList.WrapText = false
	a.folderList.BorderRounded = true

	a.messageTbl = widgets.NewTable()
	a.messageTbl.Title = "Messages"
	a.messageTbl.RowSeparator = false
	a.messageTbl.FillRow = true
	a.messageTbl.TextWrap = false
	a.messageTbl.ShowCursor = true

	a.preview = widgets.NewParagraph()
	a.preview.Title = "Message"
	a.preview.TitleBottom = "Wheel/j/k Scroll  PgUp/PgDn Page  r Reply  f Fwd  a Save  d Delete"
	a.preview.WrapText = false
	a.preview.BorderRounded = true

	a.footer = widgets.NewParagraph()
	a.footer.Border = false
	a.footer.WrapText = false

	a.applyStyles()
	if err := a.refreshFolders(); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *App) Run() error {
	a.render()
	if a.cfg.StartupSync && strings.TrimSpace(a.currentAccount().ReceiveCommand) != "" {
		a.runSync()
		a.render()
	}

	events := ui.PollEvents()
	for e := range events {
		if e.Type == ui.ResizeEvent {
			a.render()
			continue
		}
		if a.compose != nil {
			if a.handleComposeEvent(e) {
				return nil
			}
			a.render()
			continue
		}
		if e.Type == ui.MouseEvent {
			a.handleMouse(e)
			a.render()
			continue
		}
		if e.Type != ui.KeyboardEvent {
			continue
		}
		if a.handleKey(e.ID) {
			return nil
		}
		a.render()
	}
	return nil
}

func (a *App) applyStyles() {
	selected := ui.NewStyle(a.theme.selectedFG, a.theme.selectedBG)
	border := ui.NewStyle(a.theme.border, a.theme.background)
	title := ui.NewStyle(a.theme.title, a.theme.background)
	text := ui.NewStyle(a.theme.foreground, a.theme.background)
	muted := ui.NewStyle(a.theme.muted, a.theme.background)

	for _, block := range []*ui.Block{
		&a.accountBar.Block,
		&a.folderList.Block,
		&a.messageTbl.Block,
		&a.preview.Block,
		&a.footer.Block,
	} {
		block.BackgroundColor = a.theme.background
		block.BorderStyle = border
		block.TitleStyle = title
		block.TitleBottomStyle = muted
	}

	a.accountBar.BorderStyle = ui.NewStyle(a.theme.title, a.theme.background)
	a.accountBar.TextStyle = ui.NewStyle(a.theme.account, a.theme.background)

	a.folderList.TextStyle = text
	a.folderList.SelectedStyle = selected

	a.messageTbl.TextStyle = text
	a.messageTbl.SelectedRowStyle = selected
	a.messageTbl.CursorColor = a.theme.selectedBG

	a.preview.TextStyle = text
	a.footer.TextStyle = ui.NewStyle(a.theme.status, a.theme.background)
}

func (a *App) refreshFolders() error {
	folders, err := maildir.DiscoverFolders(a.currentAccount().Maildir)
	if err != nil {
		return fmt.Errorf("discover folders: %w", err)
	}
	a.folders = folders
	if len(a.folders) == 0 {
		a.messages = nil
		a.parsed = nil
		return nil
	}
	if a.selectedFolder >= len(a.folders) {
		a.selectedFolder = len(a.folders) - 1
	}
	if a.selectedFolder < 0 {
		a.selectedFolder = 0
	}
	return a.loadFolder(a.selectedFolder)
}

func (a *App) loadFolder(index int) error {
	if index < 0 || index >= len(a.folders) {
		return nil
	}
	entries, err := maildir.Scan(a.folders[index])
	if err != nil {
		return err
	}
	a.selectedFolder = index
	a.messages = entries
	a.selectedMessage = 0
	a.messageOffset = 0
	a.previewScroll = 0
	a.parsed = nil
	if len(a.messages) > 0 {
		_ = a.openMessage(0)
	}
	return nil
}

func (a *App) openMessage(index int) error {
	if index < 0 || index >= len(a.messages) {
		a.parsed = nil
		return nil
	}
	a.selectedMessage = index
	p, err := mimeutil.ParseFile(a.messages[index].Path)
	if err != nil {
		return err
	}
	if err := maildir.MarkRead(&a.messages[index]); err != nil {
		return err
	}
	a.parsed = p
	a.previewScroll = 0
	return nil
}

func (a *App) handleKey(id string) bool {
	if id != "d" {
		a.deleteArmed = false
	}

	switch id {
	case "q", "<C-c>":
		return true
	case "<Tab>":
		a.focus = (a.focus + 1) % 3
	case "<Left>", "h":
		if a.focus != focusFolders {
			a.focus = focusFolders
		}
	case "<Right>", "l":
		if a.focus == focusFolders {
			a.focus = focusMessages
		} else if a.focus == focusMessages {
			a.focus = focusPreview
		}
	case "<Up>", "k":
		a.moveSelection(-1)
	case "<Down>", "j":
		a.moveSelection(1)
	case "<PageUp>":
		a.page(-1)
	case "<PageDown>":
		a.page(1)
	case "<Home>":
		a.homeEnd(false)
	case "<End>":
		a.homeEnd(true)
	case "<Enter>":
		if a.focus == focusFolders {
			if err := a.loadFolder(a.selectedFolder); err != nil {
				a.setError(err)
			} else {
				a.focus = focusMessages
			}
		} else if a.focus == focusMessages {
			if err := a.openMessage(a.selectedMessage); err != nil {
				a.setError(err)
			} else {
				a.focus = focusPreview
			}
		}
	case "A":
		a.switchAccount(1)
	case "c":
		a.startCompose(nil, false)
	case "r":
		if a.parsed == nil {
			a.status = "No message selected"
		} else {
			a.startCompose(a.parsed, false)
		}
	case "f":
		if a.parsed == nil {
			a.status = "No message selected"
		} else {
			a.startCompose(a.parsed, true)
		}
	case "d":
		if !a.deleteArmed {
			a.deleteArmed = true
			a.status = "Press d again to delete the selected message"
		} else {
			a.deleteSelected()
			a.deleteArmed = false
		}
	case "a":
		a.saveAttachments()
	case "s":
		a.runSync()
	case "R":
		if err := a.refreshFolders(); err != nil {
			a.setError(err)
		} else {
			a.status = "Maildir refreshed"
		}
	}
	return false
}

func (a *App) moveSelection(delta int) {
	switch a.focus {
	case focusFolders:
		if len(a.folders) == 0 {
			return
		}
		a.selectedFolder = clamp(a.selectedFolder+delta, 0, len(a.folders)-1)
	case focusMessages:
		if len(a.messages) == 0 {
			return
		}
		a.selectedMessage = clamp(a.selectedMessage+delta, 0, len(a.messages)-1)
		if err := a.openMessage(a.selectedMessage); err != nil {
			a.setError(err)
		}
	case focusPreview:
		a.previewScroll = max(0, a.previewScroll+delta)
	}
}

func (a *App) page(direction int) {
	_, h := ui.TerminalDimensions()
	step := max(3, h/3)
	switch a.focus {
	case focusFolders:
		if len(a.folders) > 0 {
			a.selectedFolder = clamp(a.selectedFolder+direction*step, 0, len(a.folders)-1)
		}
	case focusMessages:
		if len(a.messages) > 0 {
			a.selectedMessage = clamp(a.selectedMessage+direction*step, 0, len(a.messages)-1)
			if err := a.openMessage(a.selectedMessage); err != nil {
				a.setError(err)
			}
		}
	case focusPreview:
		a.previewScroll = max(0, a.previewScroll+direction*step)
	}
}

func (a *App) homeEnd(end bool) {
	switch a.focus {
	case focusFolders:
		if end {
			a.selectedFolder = max(0, len(a.folders)-1)
		} else {
			a.selectedFolder = 0
		}
	case focusMessages:
		if len(a.messages) == 0 {
			return
		}
		if end {
			a.selectedMessage = len(a.messages) - 1
		} else {
			a.selectedMessage = 0
		}
		if err := a.openMessage(a.selectedMessage); err != nil {
			a.setError(err)
		}
	case focusPreview:
		if end {
			a.previewScroll = 1 << 20
		} else {
			a.previewScroll = 0
		}
	}
}

func (a *App) handleMouse(e ui.Event) {
	m, ok := e.Payload.(ui.Mouse)
	if !ok {
		return
	}
	p := image.Pt(m.X, m.Y)

	switch e.ID {
	case "<MouseLeft>", "MouseLeft":
		switch {
		case p.In(a.accountBar.Rectangle):
			a.switchAccount(1)
		case p.In(a.folderList.Inner):
			a.focus = focusFolders
			row := p.Y - a.folderList.Inner.Min.Y
			idx := a.folderOffset + row
			if idx >= 0 && idx < len(a.folders) {
				a.selectedFolder = idx
				if err := a.loadFolder(idx); err != nil {
					a.setError(err)
				}
			}
		case p.In(a.messageTbl.Inner):
			a.focus = focusMessages
			row := p.Y - a.messageTbl.Inner.Min.Y
			// Row zero is the table header.
			if row > 0 {
				idx := a.messageOffset + row - 1
				if idx >= 0 && idx < len(a.messages) {
					a.selectedMessage = idx
					if err := a.openMessage(idx); err != nil {
						a.setError(err)
					}
				}
			}
		case p.In(a.preview.Inner):
			a.focus = focusPreview
		}
	case "<MouseWheelUp>", "MouseWheelUp":
		a.mouseWheel(p, -3)
	case "<MouseWheelDown>", "MouseWheelDown":
		a.mouseWheel(p, 3)
	}
}

func (a *App) mouseWheel(p image.Point, delta int) {
	switch {
	case p.In(a.accountBar.Rectangle):
		if delta < 0 {
			a.switchAccount(-1)
		} else {
			a.switchAccount(1)
		}
	case p.In(a.folderList.Inner):
		a.focus = focusFolders
		if len(a.folders) > 0 {
			a.selectedFolder = clamp(a.selectedFolder+delta, 0, len(a.folders)-1)
		}
	case p.In(a.messageTbl.Inner):
		a.focus = focusMessages
		if len(a.messages) > 0 {
			a.selectedMessage = clamp(a.selectedMessage+delta, 0, len(a.messages)-1)
			if err := a.openMessage(a.selectedMessage); err != nil {
				a.setError(err)
			}
		}
	case p.In(a.preview.Inner):
		a.focus = focusPreview
		a.previewScroll = max(0, a.previewScroll+delta)
	}
}

func (a *App) deleteSelected() {
	if len(a.messages) == 0 || a.selectedMessage >= len(a.messages) {
		a.status = "No message selected"
		return
	}
	account := a.currentAccount()
	trash, ok := maildir.FindFolder(a.folders, account.TrashFolder)
	if !ok {
		a.status = fmt.Sprintf("Trash folder %q not found; refusing to delete", account.TrashFolder)
		return
	}
	entry := a.messages[a.selectedMessage]
	if err := maildir.Delete(entry, trash); err != nil {
		a.setError(err)
		return
	}
	if err := a.loadFolder(a.selectedFolder); err != nil {
		a.setError(err)
		return
	}
	if a.selectedMessage >= len(a.messages) && len(a.messages) > 0 {
		a.selectedMessage = len(a.messages) - 1
		_ = a.openMessage(a.selectedMessage)
	}
	a.status = "Message deleted"
}

func (a *App) saveAttachments() {
	if a.parsed == nil {
		a.status = "No message selected"
		return
	}
	if len(a.parsed.Attachments) == 0 {
		a.status = "This message has no attachments"
		return
	}
	account := a.currentAccount()
	saved, err := mimeutil.SaveAttachments(a.parsed, account.DownloadDir)
	if err != nil {
		a.setError(err)
		return
	}
	a.status = fmt.Sprintf("Saved %d attachment(s) to %s", len(saved), account.DownloadDir)
}

func (a *App) runSync() {
	account := a.currentAccount()
	if strings.TrimSpace(account.ReceiveCommand) == "" {
		a.status = fmt.Sprintf("No receive command configured for %s", account.Name)
		return
	}
	a.status = fmt.Sprintf("Synchronizing %s...", account.Name)
	a.render()
	ctx := context.Background()
	out, err := transport.Receive(ctx, account.ReceiveCommand)
	if err != nil {
		a.status = commandError("Receive command failed", out, err)
		return
	}
	if err := a.refreshFolders(); err != nil {
		a.setError(err)
		return
	}
	a.status = account.Name + " synchronized"
	if s := strings.TrimSpace(out); s != "" {
		a.status += ": " + lastLine(s)
	}
}

func (a *App) startCompose(source *mimeutil.ParsedMessage, forward bool) {
	accountIndex := a.account
	if source != nil && !forward {
		accountIndex = a.preferredReplyAccount(source)
	}
	c := &composeState{
		from:         widgets.NewParagraph(),
		to:           widgets.NewInput(),
		cc:           widgets.NewInput(),
		bcc:          widgets.NewInput(),
		subject:      widgets.NewInput(),
		body:         widgets.NewTextArea(),
		attachPrompt: widgets.NewInput(),
		account:      accountIndex,
		isReply:      source != nil && !forward,
	}
	if c.isReply {
		c.from.Title = "Reply from"
	} else {
		c.from.Title = "From"
	}
	c.from.BorderRounded = true
	c.to.Title = "To"
	c.cc.Title = "Cc"
	c.bcc.Title = "Bcc"
	c.subject.Title = "Subject"
	c.body.Title = "Body"
	c.body.ShowCursor = true
	c.attachPrompt.Title = "Attach file path"
	a.styleComposeWidgets(c)
	c.field = composeFrom

	if source != nil && forward {
		c.subject.Text = addSubjectPrefix(source.Subject, "Fwd:")
		c.body.Text = forwardBody(source)
		c.forwardAttachments = append([]mimeutil.Attachment(nil), source.Attachments...)
	} else if source != nil {
		c.to.Text = replyAddress(source.From)
		c.subject.Text = addSubjectPrefix(source.Subject, "Re:")
		c.inReplyTo = source.MessageID
		c.references = strings.TrimSpace(strings.TrimSpace(source.References) + " " + strings.TrimSpace(source.MessageID))
		c.body.Text = quoteBody(source)
	}
	c.to.Cursor = utf8.RuneCountInString(c.to.Text)
	c.subject.Cursor = utf8.RuneCountInString(c.subject.Text)
	a.compose = c
	a.updateComposeFrom()
	a.status = "Compose mode"
}

func (a *App) handleComposeEvent(e ui.Event) bool {
	c := a.compose
	if c == nil {
		return false
	}
	if e.Type == ui.MouseEvent {
		a.handleComposeMouse(e)
		return false
	}
	if e.Type != ui.KeyboardEvent {
		return false
	}

	if c.attachPrompt.Text != "" || c.attachPrompt.TitleBottom == "active" {
		return a.handleAttachPrompt(e.ID)
	}

	switch e.ID {
	case "<C-c>":
		return true
	case "<Escape>":
		a.compose = nil
		a.status = "Compose cancelled"
		return false
	case "<C-s>":
		a.sendCompose()
		return false
	case "<C-a>":
		c.attachPrompt.Text = ""
		c.attachPrompt.Cursor = 0
		c.attachPrompt.TitleBottom = "active"
		return false
	case "<Tab>":
		c.field = (c.field + 1) % 6
		return false
	case "<Backtab>", "<S-Tab>":
		c.field = (c.field + 5) % 6
		return false
	}

	if c.field == composeFrom {
		switch e.ID {
		case "<Left>", "h", "<Up>", "k":
			a.cycleComposeAccount(-1)
		case "<Right>", "l", "<Down>", "j", "<Enter>", " ":
			a.cycleComposeAccount(1)
		}
		return false
	}
	if c.field == composeBody {
		a.editTextArea(c.body, e.ID)
	} else {
		a.editInput(a.activeInput(), e.ID)
	}
	return false
}

func (a *App) handleAttachPrompt(id string) bool {
	c := a.compose
	switch id {
	case "<C-c>":
		return true
	case "<Escape>":
		c.attachPrompt.Text = ""
		c.attachPrompt.TitleBottom = ""
	case "<Enter>":
		path := expandUserPath(strings.TrimSpace(c.attachPrompt.Text))
		st, err := os.Stat(path)
		if err != nil {
			a.status = "Attachment: " + err.Error()
			return false
		}
		if st.IsDir() {
			a.status = "Attachment path is a directory"
			return false
		}
		c.attachments = append(c.attachments, path)
		c.attachPrompt.Text = ""
		c.attachPrompt.TitleBottom = ""
		a.status = "Attached " + filepath.Base(path)
	default:
		a.editInput(c.attachPrompt, id)
	}
	return false
}

func (a *App) handleComposeMouse(e ui.Event) {
	m, ok := e.Payload.(ui.Mouse)
	if !ok {
		return
	}
	p := image.Pt(m.X, m.Y)
	c := a.compose
	if c.attachPrompt.TitleBottom == "active" && p.In(c.attachPrompt.Inner) {
		return
	}
	if p.In(c.from.Inner) {
		switch e.ID {
		case "<MouseWheelUp>", "MouseWheelUp":
			c.field = composeFrom
			a.cycleComposeAccount(-1)
			return
		case "<MouseWheelDown>", "MouseWheelDown":
			c.field = composeFrom
			a.cycleComposeAccount(1)
			return
		}
	}
	if e.ID != "<MouseLeft>" && e.ID != "MouseLeft" {
		return
	}
	switch {
	case p.In(c.from.Inner):
		c.field = composeFrom
		a.cycleComposeAccount(1)
	case p.In(c.to.Inner):
		c.field = composeTo
	case p.In(c.cc.Inner):
		c.field = composeCc
	case p.In(c.bcc.Inner):
		c.field = composeBcc
	case p.In(c.subject.Inner):
		c.field = composeSubject
	case p.In(c.body.Inner):
		c.field = composeBody
	}
}

func (a *App) activeInput() *widgets.Input {
	if a.compose == nil {
		return nil
	}
	switch a.compose.field {
	case composeTo:
		return a.compose.to
	case composeCc:
		return a.compose.cc
	case composeBcc:
		return a.compose.bcc
	case composeSubject:
		return a.compose.subject
	default:
		return nil
	}
}

func (a *App) editInput(in *widgets.Input, id string) {
	if in == nil {
		return
	}
	switch id {
	case "<Backspace>", "<Backspace2>":
		in.Backspace()
	case "<Left>":
		in.MoveCursorLeft()
	case "<Right>":
		in.MoveCursorRight()
	case "<Home>":
		in.Cursor = 0
	case "<End>":
		in.Cursor = utf8.RuneCountInString(in.Text)
	case "<Delete>":
		deleteInputRune(in)
	default:
		if r, ok := printableRune(id); ok {
			in.InsertRune(r)
		}
	}
}

func (a *App) editTextArea(ta *widgets.TextArea, id string) {
	switch id {
	case "<Backspace>", "<Backspace2>":
		textAreaBackspace(ta)
	case "<Delete>":
		ta.DeleteRune()
	case "<Enter>":
		ta.InsertNewline()
	case "<Left>":
		ta.MoveCursor(-1, 0)
	case "<Right>":
		ta.MoveCursor(1, 0)
	case "<Up>":
		ta.MoveCursor(0, -1)
	case "<Down>":
		ta.MoveCursor(0, 1)
	default:
		if r, ok := printableRune(id); ok {
			ta.InsertRune(r)
		}
	}
}

func (a *App) sendCompose() {
	c := a.compose
	if c == nil {
		return
	}
	account := a.composeAccount()
	if strings.TrimSpace(account.SendCommand) == "" {
		a.status = fmt.Sprintf("No send command configured for %s", account.Name)
		return
	}
	if strings.TrimSpace(account.From) == "" {
		a.status = fmt.Sprintf("No from address configured for %s", account.Name)
		return
	}
	signature, err := config.ReadSignature(account)
	if err != nil {
		a.setError(err)
		return
	}

	draft := mimeutil.Draft{
		From:              account.From,
		To:                strings.TrimSpace(c.to.Text),
		Cc:                strings.TrimSpace(c.cc.Text),
		Bcc:               strings.TrimSpace(c.bcc.Text),
		Subject:           strings.TrimSpace(c.subject.Text),
		Body:              applySignature(c.body.Text, signature),
		InReplyTo:         c.inReplyTo,
		References:        c.references,
		Attachments:       append([]string(nil), c.attachments...),
		MemoryAttachments: append([]mimeutil.Attachment(nil), c.forwardAttachments...),
	}
	raw, err := mimeutil.Build(draft)
	if err != nil {
		a.setError(err)
		return
	}
	a.status = fmt.Sprintf("Sending from %s...", account.Name)
	a.render()
	out, err := transport.Send(context.Background(), account.SendCommand, raw)
	if err != nil {
		a.status = commandError("Send command failed", out, err)
		return
	}
	a.compose = nil
	a.status = fmt.Sprintf("Message sent from %s", account.Name)
	if s := strings.TrimSpace(out); s != "" {
		a.status += ": " + lastLine(s)
	}
}

func (a *App) render() {
	w, h := ui.TerminalDimensions()
	if w < 60 || h < 24 {
		p := widgets.NewParagraph()
		p.Title = "MailSalon"
		p.Text = fmt.Sprintf("Terminal is too small (%dx%d).\nPlease resize to at least 60x24.", w, h)
		p.BackgroundColor = a.theme.background
		p.TextStyle = ui.NewStyle(a.theme.foreground, a.theme.background)
		p.BorderStyle = ui.NewStyle(a.theme.border, a.theme.background)
		p.TitleStyle = ui.NewStyle(a.theme.title, a.theme.background)
		p.SetRect(0, 0, max(1, w), max(1, h))
		ui.Render(p)
		return
	}
	if a.compose != nil {
		a.renderCompose(w, h)
		return
	}

	folderW := clamp(w/5, 18, 30)
	footerY := h - 3
	rightX := folderW
	listH := max(8, (h-1)*45/100)
	if listH > h-7 {
		listH = h - 7
	}

	accountH := 3
	a.accountBar.SetRect(0, 0, folderW, accountH)
	a.folderList.SetRect(0, accountH, folderW, footerY)
	a.messageTbl.SetRect(rightX, 0, w, listH)
	a.preview.SetRect(rightX, listH, w, footerY)
	a.footer.SetRect(0, footerY, w, h)

	a.populateAccountBar()
	a.folderList.Title = "Folders"
	a.populateFolderList()
	a.populateMessageTable()
	a.populatePreview()
	a.footer.Text = a.footerText()

	a.updateFocusStyles()
	ui.Render(a.accountBar, a.folderList, a.messageTbl, a.preview, a.footer)
}

func (a *App) populateAccountBar() {
	account := a.currentAccount()
	if len(a.cfg.Accounts) > 1 {
		a.accountBar.Title = safeUI(fmt.Sprintf("Account %d/%d", a.account+1, len(a.cfg.Accounts)))
		a.accountBar.Text = safeUI("‹ " + account.Name + " ›")
		a.accountBar.TitleBottom = "A/Click switch"
	} else {
		a.accountBar.Title = "Account"
		a.accountBar.Text = safeUI(account.Name)
		a.accountBar.TitleBottom = "only account"
	}
}

func (a *App) populateFolderList() {
	visible := max(1, a.folderList.Inner.Dy())
	a.folderOffset = keepVisible(a.selectedFolder, a.folderOffset, visible, len(a.folders))
	end := min(len(a.folders), a.folderOffset+visible)
	rows := make([]string, 0, max(0, end-a.folderOffset))
	for _, f := range a.folders[a.folderOffset:end] {
		rows = append(rows, safeUI(f.Name))
	}
	a.folderList.Rows = rows
	a.folderList.SelectedRow = a.selectedFolder - a.folderOffset
	if len(a.folders) == 0 {
		a.folderList.Rows = []string{"(no folders)"}
		a.folderList.SelectedRow = -1
	}
}

func (a *App) populateMessageTable() {
	visible := max(1, a.messageTbl.Inner.Dy()-1)
	a.messageOffset = keepVisible(a.selectedMessage, a.messageOffset, visible, len(a.messages))
	end := min(len(a.messages), a.messageOffset+visible)
	rows := make([][]string, 0, visible+1)
	rows = append(rows, []string{"", "From", "Subject", "Date"})
	a.messageTbl.RowStyles = map[int]ui.Style{
		0: ui.NewStyle(a.theme.muted, a.theme.background),
	}
	for i, m := range a.messages[a.messageOffset:end] {
		mark := " "
		if m.Unread {
			mark = "●"
			a.messageTbl.RowStyles[i+1] = ui.NewStyle(a.theme.unread, a.theme.background)
		}
		rows = append(rows, []string{
			mark,
			safeUI(m.From),
			safeUI(emptySubject(m.Subject)),
			formatDate(m.Date),
		})
	}
	a.messageTbl.Rows = rows
	a.messageTbl.SelectedRow = a.selectedMessage - a.messageOffset + 1
	if len(a.messages) == 0 {
		a.messageTbl.Rows = [][]string{{"", "From", "Subject", "Date"}, {"", "", "(no messages)", ""}}
		a.messageTbl.SelectedRow = -1
	}

	innerW := max(20, a.messageTbl.Inner.Dx())
	dateW := 16
	markW := 2
	fromW := clamp(innerW/4, 14, 28)
	subjectW := max(12, innerW-markW-fromW-dateW-3)
	a.messageTbl.ColumnWidths = []int{markW, fromW, subjectW, dateW}
}

func (a *App) populatePreview() {
	if a.parsed == nil {
		a.preview.Text = "No message selected."
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\n", a.parsed.From)
	fmt.Fprintf(&b, "To: %s\n", a.parsed.To)
	if strings.TrimSpace(a.parsed.Cc) != "" {
		fmt.Fprintf(&b, "Cc: %s\n", a.parsed.Cc)
	}
	fmt.Fprintf(&b, "Date: %s\n", a.parsed.Date)
	fmt.Fprintf(&b, "Subject: %s\n", emptySubject(a.parsed.Subject))
	if len(a.parsed.Attachments) > 0 {
		names := make([]string, 0, len(a.parsed.Attachments))
		for _, at := range a.parsed.Attachments {
			names = append(names, at.Filename)
		}
		fmt.Fprintf(&b, "Attachments: %s\n", strings.Join(names, ", "))
	}
	b.WriteString("\n")
	b.WriteString(a.parsed.Body)

	width := max(10, a.preview.Inner.Dx())
	lines := wrapLines(safeUI(b.String()), width)
	visible := max(1, a.preview.Inner.Dy())
	maxScroll := max(0, len(lines)-visible)
	a.previewScroll = clamp(a.previewScroll, 0, maxScroll)
	end := min(len(lines), a.previewScroll+visible)
	a.preview.Text = strings.Join(lines[a.previewScroll:end], "\n")
	if len(lines) == 0 {
		a.preview.Text = "(empty message)"
	}
}

func (a *App) updateFocusStyles() {
	active := ui.NewStyle(a.theme.activeBorder, a.theme.background)
	inactive := ui.NewStyle(a.theme.border, a.theme.background)
	a.folderList.BorderStyle = inactive
	a.messageTbl.BorderStyle = inactive
	a.preview.BorderStyle = inactive
	switch a.focus {
	case focusFolders:
		a.folderList.BorderStyle = active
	case focusMessages:
		a.messageTbl.BorderStyle = active
	case focusPreview:
		a.preview.BorderStyle = active
	}
	a.updateFooterStyle()
}

func (a *App) footerText() string {
	account := a.currentAccount()
	line1 := fmt.Sprintf(" [%s] %s", account.Name, a.status)
	line2 := " c Compose  r Reply  f Fwd  d Delete  a Save  s Sync  A Switch account"
	line3 := " Tab Focus  j/k Move  Enter Open  PgUp/PgDn Page  R Refresh  q Quit"
	return safeUI(line1 + "\n" + line2 + "\n" + line3)
}

func (a *App) renderCompose(w, h int) {
	c := a.compose
	footerY := h - 3
	inputH := 3
	y := 0
	c.from.SetRect(0, y, w, y+inputH)
	y += inputH
	c.to.SetRect(0, y, w, y+inputH)
	y += inputH
	c.cc.SetRect(0, y, w, y+inputH)
	y += inputH
	c.bcc.SetRect(0, y, w, y+inputH)
	y += inputH
	c.subject.SetRect(0, y, w, y+inputH)
	y += inputH
	attachY := footerY - 3
	if attachY <= y+3 {
		attachY = footerY
	}
	c.body.SetRect(0, y, w, attachY)

	attachments := widgets.NewParagraph()
	attachments.Title = "Attachments"
	attachments.BorderRounded = true
	attachments.BackgroundColor = a.theme.background
	attachments.TextStyle = ui.NewStyle(a.theme.foreground, a.theme.background)
	attachments.BorderStyle = ui.NewStyle(a.theme.border, a.theme.background)
	attachments.TitleStyle = ui.NewStyle(a.theme.title, a.theme.background)
	attachments.TitleBottomStyle = ui.NewStyle(a.theme.muted, a.theme.background)
	attachments.SetRect(0, attachY, w, footerY)
	if len(c.attachments) == 0 && len(c.forwardAttachments) == 0 {
		attachments.Text = "(none)"
	} else {
		names := make([]string, 0, len(c.attachments)+len(c.forwardAttachments))
		for _, p := range c.attachments {
			names = append(names, filepath.Base(p))
		}
		for _, at := range c.forwardAttachments {
			names = append(names, at.Filename+" (forwarded)")
		}
		attachments.Text = safeUI(strings.Join(names, ", "))
	}

	a.footer.SetRect(0, footerY, w, h)
	fromHint := "From row shows the sending identity"
	if len(a.cfg.Accounts) > 1 {
		fromHint = "From row: ←/→ change sending account"
	}
	a.footer.Text = safeUI(" " + a.status + "\n Ctrl+S Send  Ctrl+A Attach  Tab/Shift+Tab Fields  Esc Cancel\n" + fromHint)
	a.updateFooterStyle()

	a.highlightComposeField()
	items := []ui.Drawable{c.from, c.to, c.cc, c.bcc, c.subject, c.body, attachments, a.footer}
	if c.attachPrompt.TitleBottom == "active" {
		promptW := clamp(w-10, 40, 90)
		x := (w - promptW) / 2
		py := max(1, h/2-2)
		c.attachPrompt.SetRect(x, py, x+promptW, py+3)
		items = append(items, c.attachPrompt)
	}
	ui.Render(items...)
}

func (a *App) highlightComposeField() {
	c := a.compose
	inactive := ui.NewStyle(a.theme.border, a.theme.background)
	active := ui.NewStyle(a.theme.activeBorder, a.theme.background)
	c.from.BorderStyle = inactive
	for _, w := range []*widgets.Input{c.to, c.cc, c.bcc, c.subject} {
		w.BorderStyle = inactive
	}
	c.body.BorderStyle = inactive
	switch c.field {
	case composeFrom:
		c.from.BorderStyle = active
	case composeTo:
		c.to.BorderStyle = active
	case composeCc:
		c.cc.BorderStyle = active
	case composeBcc:
		c.bcc.BorderStyle = active
	case composeSubject:
		c.subject.BorderStyle = active
	case composeBody:
		c.body.BorderStyle = active
	}
}

func (a *App) styleComposeWidgets(c *composeState) {
	border := ui.NewStyle(a.theme.border, a.theme.background)
	title := ui.NewStyle(a.theme.title, a.theme.background)
	text := ui.NewStyle(a.theme.foreground, a.theme.background)
	muted := ui.NewStyle(a.theme.muted, a.theme.background)
	cursor := ui.NewStyle(a.theme.cursorFG, a.theme.cursorBG)

	c.from.BackgroundColor = a.theme.background
	c.from.BorderStyle = border
	c.from.TitleStyle = title
	c.from.TitleBottomStyle = muted
	c.from.TextStyle = ui.NewStyle(a.theme.account, a.theme.background)

	for _, w := range []*widgets.Input{c.to, c.cc, c.bcc, c.subject, c.attachPrompt} {
		w.BorderRounded = true
		w.BackgroundColor = a.theme.background
		w.BorderStyle = border
		w.TitleStyle = title
		w.TitleBottomStyle = muted
		w.TextStyle = text
		w.CursorStyle = cursor
	}
	c.body.BorderRounded = true
	c.body.BackgroundColor = a.theme.background
	c.body.BorderStyle = border
	c.body.TitleStyle = title
	c.body.TitleBottomStyle = muted
	c.body.TextStyle = text
	c.body.CursorStyle = cursor
}

func (a *App) updateFooterStyle() {
	statusColor := a.theme.status
	lower := strings.ToLower(a.status)
	if strings.HasPrefix(lower, "error:") || strings.Contains(lower, " failed") || strings.HasPrefix(lower, "failed") {
		statusColor = a.theme.error
	}
	a.footer.BackgroundColor = a.theme.background
	a.footer.TextStyle = ui.NewStyle(statusColor, a.theme.background)
}

func themeWithDefaults(t config.Theme) config.Theme {
	d := config.DefaultTheme()
	set := func(dst *string, src string) {
		if strings.TrimSpace(src) != "" {
			*dst = src
		}
	}
	set(&d.Background, t.Background)
	set(&d.Foreground, t.Foreground)
	set(&d.Muted, t.Muted)
	set(&d.Border, t.Border)
	set(&d.ActiveBorder, t.ActiveBorder)
	set(&d.Title, t.Title)
	set(&d.SelectedFG, t.SelectedFG)
	set(&d.SelectedBG, t.SelectedBG)
	set(&d.Account, t.Account)
	set(&d.Unread, t.Unread)
	set(&d.Status, t.Status)
	set(&d.Error, t.Error)
	set(&d.CursorFG, t.CursorFG)
	set(&d.CursorBG, t.CursorBG)
	return d
}

func resolveTheme(t config.Theme) (resolvedTheme, error) {
	parse := func(name, value string) (ui.Color, error) {
		c, err := parseColor(value)
		if err != nil {
			return ui.ColorClear, fmt.Errorf("theme.%s: %w", name, err)
		}
		return c, nil
	}
	var out resolvedTheme
	var err error
	if out.background, err = parse("background", t.Background); err != nil {
		return out, err
	}
	if out.foreground, err = parse("foreground", t.Foreground); err != nil {
		return out, err
	}
	if out.muted, err = parse("muted", t.Muted); err != nil {
		return out, err
	}
	if out.border, err = parse("border", t.Border); err != nil {
		return out, err
	}
	if out.activeBorder, err = parse("active_border", t.ActiveBorder); err != nil {
		return out, err
	}
	if out.title, err = parse("title", t.Title); err != nil {
		return out, err
	}
	if out.selectedFG, err = parse("selected_fg", t.SelectedFG); err != nil {
		return out, err
	}
	if out.selectedBG, err = parse("selected_bg", t.SelectedBG); err != nil {
		return out, err
	}
	if out.account, err = parse("account", t.Account); err != nil {
		return out, err
	}
	if out.unread, err = parse("unread", t.Unread); err != nil {
		return out, err
	}
	if out.status, err = parse("status", t.Status); err != nil {
		return out, err
	}
	if out.error, err = parse("error", t.Error); err != nil {
		return out, err
	}
	if out.cursorFG, err = parse("cursor_fg", t.CursorFG); err != nil {
		return out, err
	}
	if out.cursorBG, err = parse("cursor_bg", t.CursorBG); err != nil {
		return out, err
	}
	return out, nil
}

func parseColor(value string) (ui.Color, error) {
	s := strings.ToLower(strings.TrimSpace(value))
	if len(s) == 7 && s[0] == '#' {
		r, err := strconv.ParseUint(s[1:3], 16, 8)
		if err != nil {
			return ui.ColorClear, fmt.Errorf("invalid color %q", value)
		}
		g, err := strconv.ParseUint(s[3:5], 16, 8)
		if err != nil {
			return ui.ColorClear, fmt.Errorf("invalid color %q", value)
		}
		b, err := strconv.ParseUint(s[5:7], 16, 8)
		if err != nil {
			return ui.ColorClear, fmt.Errorf("invalid color %q", value)
		}
		return ui.NewColorRGB(int32(r), int32(g), int32(b)), nil
	}
	colors := map[string]ui.Color{
		"default": ui.ColorClear, "clear": ui.ColorClear,
		"black": ui.ColorBlack, "red": ui.ColorRed, "green": ui.ColorGreen,
		"yellow": ui.ColorYellow, "blue": ui.ColorBlue, "magenta": ui.ColorMagenta,
		"cyan": ui.ColorCyan, "white": ui.ColorWhite, "grey": ui.ColorGrey,
		"gray": ui.ColorGrey, "darkgrey": ui.ColorDarkGrey, "darkgray": ui.ColorDarkGrey,
		"lightgrey": ui.ColorLightGrey, "lightgray": ui.ColorLightGrey, "silver": ui.ColorSilver,
		"orange": ui.ColorOrange, "purple": ui.ColorPurple, "pink": ui.ColorPink,
		"coral": ui.ColorCoral, "crimson": ui.ColorCrimson, "gold": ui.ColorGold,
		"teal": ui.ColorTeal, "turquoise": ui.ColorTurquoise, "indigo": ui.ColorIndigo,
		"violet": ui.ColorViolet, "olive": ui.ColorOlive, "navy": ui.ColorNavy,
		"aliceblue": ui.ColorAliceBlue, "beige": ui.ColorBeige, "brown": ui.ColorBrown,
		"darkblue": ui.ColorDarkBlue, "darkcyan": ui.ColorDarkCyan, "darkgreen": ui.ColorDarkGreen,
		"darkred": ui.ColorDarkRed, "hotpink": ui.ColorHotPink, "lightblue": ui.ColorLightBlue,
		"lightcyan": ui.ColorLightCyan, "lightgreen": ui.ColorLightGreen, "lime": ui.ColorLime,
		"maroon": ui.ColorMaroon, "mintcream": ui.ColorMintCream, "mistyrose": ui.ColorMistyRose,
		"orchid": ui.ColorOrchid, "plum": ui.ColorPlum, "salmon": ui.ColorSalmon,
		"seagreen": ui.ColorSeaGreen, "skyblue": ui.ColorSkyBlue, "slateblue": ui.ColorSlateBlue,
		"tan": ui.ColorTan, "tomato": ui.ColorTomato, "wheat": ui.ColorWheat,
	}
	if c, ok := colors[s]; ok {
		return c, nil
	}
	return ui.ColorClear, fmt.Errorf("unsupported color %q", value)
}

func (a *App) currentAccount() config.Account {
	if len(a.cfg.Accounts) == 0 {
		return config.Account{}
	}
	a.account = clamp(a.account, 0, len(a.cfg.Accounts)-1)
	return a.cfg.Accounts[a.account]
}

func (a *App) composeAccount() config.Account {
	if a.compose == nil || len(a.cfg.Accounts) == 0 {
		return a.currentAccount()
	}
	a.compose.account = clamp(a.compose.account, 0, len(a.cfg.Accounts)-1)
	return a.cfg.Accounts[a.compose.account]
}

func (a *App) switchAccount(delta int) {
	if len(a.cfg.Accounts) < 2 {
		a.status = "Only one account is configured"
		return
	}
	a.account = wrapIndex(a.account+delta, len(a.cfg.Accounts))
	a.selectedFolder = 0
	a.selectedMessage = 0
	a.folderOffset = 0
	a.messageOffset = 0
	a.previewScroll = 0
	if err := a.refreshFolders(); err != nil {
		a.setError(err)
		return
	}
	a.status = "Switched to " + a.currentAccount().Name
}

func (a *App) cycleComposeAccount(delta int) {
	if a.compose == nil || len(a.cfg.Accounts) < 2 {
		return
	}
	a.compose.account = wrapIndex(a.compose.account+delta, len(a.cfg.Accounts))
	a.updateComposeFrom()
	a.status = "From: " + a.composeAccount().From
}

func (a *App) updateComposeFrom() {
	if a.compose == nil {
		return
	}
	account := a.composeAccount()
	text := fmt.Sprintf("%s — %s", account.Name, account.From)
	if account.SignatureFile != "" {
		text += "   signature: " + account.SignatureFile
	}
	a.compose.from.Text = safeUI(text)
	if len(a.cfg.Accounts) > 1 {
		a.compose.from.TitleBottom = "←/→ select"
	} else {
		a.compose.from.TitleBottom = ""
	}
}

func (a *App) preferredReplyAccount(p *mimeutil.ParsedMessage) int {
	if p == nil {
		return a.account
	}
	recipients := parseAddressSet(p.To + "," + p.Cc)
	for i, account := range a.cfg.Accounts {
		addr, err := mail.ParseAddress(account.From)
		if err == nil && recipients[strings.ToLower(addr.Address)] {
			return i
		}
	}
	return a.account
}

func (a *App) setError(err error) {
	a.status = "Error: " + err.Error()
}

func safeUI(s string) string {
	// gotui supports inline style markup. Mail content is untrusted display data,
	// so break the style introducers rather than letting a message restyle the UI.
	replacer := strings.NewReplacer(
		"](fg:", "] (fg:",
		"](bg:", "] (bg:",
		"](mod:", "] (mod:",
	)
	return replacer.Replace(s)
}

func printableRune(id string) (rune, bool) {
	if strings.HasPrefix(id, "<") || utf8.RuneCountInString(id) != 1 {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(id)
	if r < 0x20 || r == 0x7f {
		return 0, false
	}
	return r, true
}

func deleteInputRune(in *widgets.Input) {
	runes := []rune(in.Text)
	if in.Cursor < 0 || in.Cursor >= len(runes) {
		return
	}
	in.Text = string(append(runes[:in.Cursor], runes[in.Cursor+1:]...))
}

func textAreaBackspace(ta *widgets.TextArea) {
	lines := strings.Split(ta.Text, "\n")
	y := clamp(ta.Cursor.Y, 0, max(0, len(lines)-1))
	if len(lines) == 0 {
		return
	}
	runes := []rune(lines[y])
	x := clamp(ta.Cursor.X, 0, len(runes))
	if x > 0 {
		lines[y] = string(append(runes[:x-1], runes[x:]...))
		ta.Text = strings.Join(lines, "\n")
		ta.Cursor = image.Pt(x-1, y)
		return
	}
	if y == 0 {
		return
	}
	prev := []rune(lines[y-1])
	newX := len(prev)
	lines[y-1] += lines[y]
	lines = append(lines[:y], lines[y+1:]...)
	ta.Text = strings.Join(lines, "\n")
	ta.Cursor = image.Pt(newX, y-1)
}

func parseAddressSet(raw string) map[string]bool {
	out := make(map[string]bool)
	addrs, err := mail.ParseAddressList(strings.Trim(raw, " ,"))
	if err != nil {
		return out
	}
	for _, addr := range addrs {
		out[strings.ToLower(addr.Address)] = true
	}
	return out
}

func applySignature(body, signature string) string {
	signature = strings.TrimRight(signature, "\r\n")
	if strings.TrimSpace(signature) == "" {
		return body
	}
	block := signature
	trimmed := strings.TrimLeft(signature, " \t\r\n")
	if !strings.HasPrefix(trimmed, "-- ") && trimmed != "--" {
		block = "-- \n" + signature
	}

	markers := []string{"\n\nOn ", "\n\n---------- Forwarded message ----------"}
	cut := -1
	for _, marker := range markers {
		if i := strings.Index(body, marker); i >= 0 && (cut < 0 || i < cut) {
			cut = i
		}
	}
	if cut >= 0 {
		lead := strings.TrimRight(body[:cut], " \t\r\n")
		if lead == "" {
			return block + body[cut:]
		}
		return lead + "\n\n" + block + body[cut:]
	}
	lead := strings.TrimRight(body, "\r\n")
	if lead == "" {
		return block + "\n"
	}
	return lead + "\n\n" + block + "\n"
}

func wrapIndex(i, n int) int {
	if n <= 0 {
		return 0
	}
	i %= n
	if i < 0 {
		i += n
	}
	return i
}

func replyAddress(raw string) string {
	addr, err := mail.ParseAddress(raw)
	if err == nil {
		return addr.String()
	}
	return raw
}

func addSubjectPrefix(subject, prefix string) string {
	subject = strings.TrimSpace(subject)
	if strings.HasPrefix(strings.ToLower(subject), strings.ToLower(prefix)) {
		return subject
	}
	if subject == "" {
		return prefix
	}
	return prefix + " " + subject
}

func quoteBody(p *mimeutil.ParsedMessage) string {
	var b strings.Builder
	b.WriteString("\n\nOn ")
	b.WriteString(p.Date)
	b.WriteString(", ")
	b.WriteString(p.From)
	b.WriteString(" wrote:\n")
	for _, line := range strings.Split(p.Body, "\n") {
		b.WriteString("> ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func forwardBody(p *mimeutil.ParsedMessage) string {
	return fmt.Sprintf("\n\n---------- Forwarded message ----------\nFrom: %s\nDate: %s\nSubject: %s\nTo: %s\n\n%s\n",
		p.From, p.Date, p.Subject, p.To, p.Body)
}

func commandError(prefix, output string, err error) string {
	msg := prefix + ": " + err.Error()
	if s := strings.TrimSpace(output); s != "" {
		msg += " — " + lastLine(s)
	}
	return msg
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) == 0 {
		return ""
	}
	line := strings.TrimSpace(lines[len(lines)-1])
	if len(line) > 120 {
		line = line[:120] + "…"
	}
	return line
}

func emptySubject(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(no subject)"
	}
	return s
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	now := time.Now()
	if now.Year() == t.Year() && now.YearDay() == t.YearDay() {
		return t.Format("15:04")
	}
	if now.Year() == t.Year() {
		return t.Format("Jan 02 15:04")
	}
	return t.Format("2006-01-02")
}

func wrapLines(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		runes := []rune(line)
		if len(runes) == 0 {
			out = append(out, "")
			continue
		}
		for len(runes) > width {
			cut := width
			for i := width; i > width/2; i-- {
				if runes[i-1] == ' ' || runes[i-1] == '\t' {
					cut = i
					break
				}
			}
			out = append(out, string(runes[:cut]))
			runes = runes[cut:]
			for len(runes) > 0 && (runes[0] == ' ' || runes[0] == '\t') {
				runes = runes[1:]
			}
		}
		out = append(out, string(runes))
	}
	return out
}

func keepVisible(selected, offset, visible, total int) int {
	if total <= 0 || visible <= 0 {
		return 0
	}
	selected = clamp(selected, 0, total-1)
	maxOffset := max(0, total-visible)
	offset = clamp(offset, 0, maxOffset)
	if selected < offset {
		offset = selected
	} else if selected >= offset+visible {
		offset = selected - visible + 1
	}
	return clamp(offset, 0, maxOffset)
}

func expandUserPath(path string) string {
	path = os.ExpandEnv(path)
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
