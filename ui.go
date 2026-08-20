package main

import (
	"fmt"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type focusArea int

const (
	focusList focusArea = iota
	focusBody
)

type uiMode int

const (
	modeInbox uiMode = iota
	modeReply
	modeAttachPath
)

type syncedMsg struct {
	messages     []Message
	err          error
	output       string
	maildirFiles int
}

type loadedMsg struct {
	messages []Message
	err      error
}

type sentMsg struct{ err error }

type savedAttachmentsMsg struct {
	paths []string
	err   error
}

type model struct {
	cfg                Config
	messages           []Message
	selected           int
	width              int
	height             int
	focus              focusArea
	mode               uiMode
	viewport           viewport.Model
	composer           textarea.Model
	attachmentInput    textarea.Model
	replyAttachments   []string
	status             string
	busy               bool
	startupSyncStarted bool
	confirmDelete      bool
}

var (
	accent  = lipgloss.Color("#C2185B")
	dim     = lipgloss.Color("#777777")
	good    = lipgloss.Color("#2ECC71")
	bad     = lipgloss.Color("#FF5F5F")
	titleSt = lipgloss.NewStyle().Bold(true).Foreground(accent)
	dimSt   = lipgloss.NewStyle().Foreground(dim)
	goodSt  = lipgloss.NewStyle().Foreground(good)
	badSt   = lipgloss.NewStyle().Foreground(bad)
)

func newModel(cfg Config) model {
	vp := viewport.New(viewport.WithWidth(60), viewport.WithHeight(20))
	vp.SoftWrap = true
	vp.FillHeight = true

	composer := textarea.New()
	composer.Placeholder = "Write your reply…"
	composer.ShowLineNumbers = false
	composer.SetVirtualCursor(true)
	composer.SetWidth(76)
	composer.SetHeight(18)

	attachmentInput := textarea.New()
	attachmentInput.Placeholder = "/path/to/file"
	attachmentInput.ShowLineNumbers = false
	attachmentInput.SetVirtualCursor(true)
	attachmentInput.SetWidth(76)
	attachmentInput.SetHeight(1)

	return model{
		cfg:             cfg,
		viewport:        vp,
		composer:        composer,
		attachmentInput: attachmentInput,
		status:          "loading local Maildir…",
		busy:            true,
	}
}

func (m model) Init() tea.Cmd {
	// Always show the existing local mailbox first. Startup sync can take a
	// while, and the inbox should remain usable while offlineimap is running.
	return loadCmd(m.cfg.Maildir)
}

func loadCmd(maildir string) tea.Cmd {
	return func() tea.Msg {
		messages, err := loadMaildir(maildir)
		return loadedMsg{messages: messages, err: err}
	}
}

func syncCmd(cfg Config) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command(cfg.OfflineIMAP, cfg.OfflineIMAPArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return syncedMsg{err: fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)}
		}
		messages, loadErr := loadMaildir(cfg.Maildir)
		if loadErr != nil {
			return syncedMsg{err: loadErr, output: strings.TrimSpace(string(output))}
		}
		fileCount, countErr := countMaildirMessageFiles(cfg.Maildir)
		if countErr != nil {
			return syncedMsg{err: countErr, output: strings.TrimSpace(string(output))}
		}
		if len(messages) == 0 && fileCount > 0 {
			return syncedMsg{
				err:          fmt.Errorf("reload parsed 0 messages from %d Maildir files in %s", fileCount, cfg.Maildir),
				output:       strings.TrimSpace(string(output)),
				maildirFiles: fileCount,
			}
		}
		return syncedMsg{messages: messages, output: strings.TrimSpace(string(output)), maildirFiles: fileCount}
	}
}

func sendCmd(cfg Config, original Message, body string, attachmentPaths []string) tea.Cmd {
	return func() tea.Msg {
		wire, err := buildReplyWithAttachments(cfg, original, body, attachmentPaths)
		if err == nil {
			err = runMSMTP(cfg, wire)
		}
		return sentMsg{err: err}
	}
}

func saveAttachmentsCmd(msg Message, downloadDir string) tea.Cmd {
	return func() tea.Msg {
		paths, err := saveMessageAttachments(msg, downloadDir)
		return savedAttachmentsMsg{paths: paths, err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		m.refreshPreview()
		return m, nil

	case loadedMsg:
		m.busy = false
		if msg.err != nil {
			m.status = "load failed: " + msg.err.Error()
			return m, nil
		}
		m.messages = msg.messages
		m.clampSelection()
		m.refreshPreview()

		// On startup, display local mail immediately, then sync in the
		// background. syncCmd reloads the Maildir when offlineimap finishes.
		if m.cfg.StartupSync && !m.startupSyncStarted {
			m.startupSyncStarted = true
			m.busy = true
			m.status = fmt.Sprintf("loaded %d messages from %s · syncing with offlineimap…", len(m.messages), m.cfg.Maildir)
			return m, syncCmd(m.cfg)
		}

		m.status = fmt.Sprintf("loaded %d messages from %s", len(m.messages), m.cfg.Maildir)
		return m, nil

	case syncedMsg:
		m.busy = false
		if msg.err != nil {
			m.status = "sync/reload failed: " + msg.err.Error()
			return m, nil
		}
		m.messages = msg.messages
		m.clampSelection()
		m.refreshPreview()
		m.status = fmt.Sprintf("sync complete · %d messages from %s", len(m.messages), m.cfg.Maildir)
		return m, nil

	case sentMsg:
		m.busy = false
		if msg.err != nil {
			m.status = "send failed: " + msg.err.Error()
			return m, nil
		}
		m.mode = modeInbox
		m.composer.Blur()
		m.composer.Reset()
		m.replyAttachments = nil
		m.status = "reply sent with msmtp"
		return m, nil

	case savedAttachmentsMsg:
		m.busy = false
		if msg.err != nil {
			m.status = "attachment save failed: " + msg.err.Error()
			return m, nil
		}
		if len(msg.paths) == 1 {
			m.status = "saved attachment to " + msg.paths[0]
		} else {
			m.status = fmt.Sprintf("saved %d attachments to %s", len(msg.paths), m.cfg.DownloadDir)
		}
		return m, nil
	}

	if m.mode == modeAttachPath {
		return m.updateAttachPath(msg)
	}
	if m.mode == modeReply {
		return m.updateReply(msg)
	}
	return m.updateInbox(msg)
}

func (m model) updateReply(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			if !m.busy {
				m.mode = modeInbox
				m.composer.Blur()
				m.replyAttachments = nil
				m.status = "reply cancelled"
			}
			return m, nil
		case "ctrl+o":
			if m.busy {
				return m, nil
			}
			m.mode = modeAttachPath
			m.composer.Blur()
			m.attachmentInput.Reset()
			cmd := m.attachmentInput.Focus()
			m.status = "attach file · enter add · esc cancel"
			return m, cmd
		case "ctrl+x":
			if len(m.replyAttachments) == 0 {
				m.status = "no reply attachments to remove"
				return m, nil
			}
			removed := m.replyAttachments[len(m.replyAttachments)-1]
			m.replyAttachments = m.replyAttachments[:len(m.replyAttachments)-1]
			m.status = "removed attachment " + removed
			return m, nil
		case "ctrl+s", "alt+s":
			if m.busy || len(m.messages) == 0 {
				return m, nil
			}
			body := strings.TrimSpace(m.composer.Value())
			if body == "" {
				m.status = "reply is empty"
				return m, nil
			}
			m.busy = true
			m.status = "sending with msmtp…"
			return m, sendCmd(m.cfg, m.messages[m.selected], body, append([]string(nil), m.replyAttachments...))
		}
	}
	var cmd tea.Cmd
	m.composer, cmd = m.composer.Update(msg)
	return m, cmd
}

func (m model) updateAttachPath(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			m.mode = modeReply
			m.attachmentInput.Blur()
			cmd := m.composer.Focus()
			m.status = "attachment cancelled"
			return m, cmd
		case "enter":
			path, err := normalizeAttachmentPath(m.attachmentInput.Value())
			if err != nil {
				m.status = "attachment error: " + err.Error()
				return m, nil
			}
			for _, existing := range m.replyAttachments {
				if existing == path {
					m.status = "attachment already added"
					return m, nil
				}
			}
			m.replyAttachments = append(m.replyAttachments, path)
			m.mode = modeReply
			m.attachmentInput.Blur()
			cmd := m.composer.Focus()
			m.status = fmt.Sprintf("attached %s · %d file(s)", path, len(m.replyAttachments))
			return m, cmd
		}
	}
	var cmd tea.Cmd
	m.attachmentInput, cmd = m.attachmentInput.Update(msg)
	return m, cmd
}

func (m model) updateInbox(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	if m.confirmDelete {
		switch key.String() {
		case "y", "enter":
			m.confirmDelete = false
			if len(m.messages) == 0 {
				m.status = "nothing to delete"
				return m, nil
			}

			deleted := m.messages[m.selected]
			if err := deleteMessage(deleted); err != nil {
				m.status = "delete failed: " + err.Error()
				return m, nil
			}

			m.messages = append(m.messages[:m.selected], m.messages[m.selected+1:]...)
			m.clampSelection()
			m.refreshPreview()
			m.status = "message deleted locally · sync to propagate deletion"
			return m, nil
		case "n", "esc", "q":
			m.confirmDelete = false
			m.status = "delete cancelled"
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		default:
			return m, nil
		}
	}

	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "a":
		if m.busy {
			return m, nil
		}
		if len(m.messages) == 0 {
			m.status = "nothing selected"
			return m, nil
		}
		if len(m.messages[m.selected].Attachments) == 0 {
			m.status = "message has no attachments"
			return m, nil
		}
		m.busy = true
		m.status = fmt.Sprintf("saving %d attachment(s) to %s…", len(m.messages[m.selected].Attachments), m.cfg.DownloadDir)
		return m, saveAttachmentsCmd(m.messages[m.selected], m.cfg.DownloadDir)
	case "d", "delete":
		if m.busy {
			return m, nil
		}
		if len(m.messages) == 0 {
			m.status = "nothing to delete"
			return m, nil
		}
		m.confirmDelete = true
		m.status = fmt.Sprintf("delete %q? y/enter yes · n/esc cancel", m.messages[m.selected].Subject)
		return m, nil
	case "s":
		if !m.busy {
			m.busy = true
			m.status = "syncing with offlineimap…"
			return m, syncCmd(m.cfg)
		}
	case "r":
		if len(m.messages) == 0 {
			m.status = "nothing to reply to"
			return m, nil
		}
		if strings.TrimSpace(m.cfg.From) == "" {
			m.status = "reply disabled: configure -from or MAILTUI_FROM"
			return m, nil
		}
		if m.messages[m.selected].ReplyTo == "" {
			m.status = "reply disabled: no valid sender address"
			return m, nil
		}
		m.mode = modeReply
		m.replyAttachments = nil
		m.composer.Reset()
		m.composer.SetValue(replyQuote(m.messages[m.selected]))
		m.composer.MoveToBegin()
		cmd := m.composer.Focus()
		m.status = "compose reply · ctrl+o attach · ctrl+s / alt+s send · esc cancel"
		return m, cmd
	case "tab":
		if m.focus == focusList {
			m.focus = focusBody
			m.status = "message body focused · j/k scroll · tab returns to list"
		} else {
			m.focus = focusList
			m.status = "message list focused"
		}
		return m, nil
	}

	if m.focus == focusList {
		switch key.String() {
		case "j", "down":
			m.moveSelection(1)
			return m, nil
		case "k", "up":
			m.moveSelection(-1)
			return m, nil
		case "g", "home":
			m.selected = 0
			m.afterSelectionChanged()
			return m, nil
		case "G", "end":
			if len(m.messages) > 0 {
				m.selected = len(m.messages) - 1
				m.afterSelectionChanged()
			}
			return m, nil
		case "enter", "right", "l":
			m.focus = focusBody
			m.markSelectedRead()
			return m, nil
		}
	} else {
		switch key.String() {
		case "esc", "left", "h":
			m.focus = focusList
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	listW := m.listWidth()
	previewW := max(20, m.width-listW)
	contentH := max(4, m.height-3)
	m.viewport.SetWidth(max(10, previewW-4))
	m.viewport.SetHeight(max(3, contentH-7))
	m.composer.SetWidth(max(20, m.width-6))
	m.composer.SetHeight(max(6, m.height-10))
	m.attachmentInput.SetWidth(max(20, m.width-18))
}

func (m model) listWidth() int {
	if m.width < 80 {
		return max(24, m.width/2)
	}
	return min(44, max(32, m.width*38/100))
}

func (m *model) moveSelection(delta int) {
	if len(m.messages) == 0 {
		return
	}
	m.selected += delta
	m.clampSelection()
	m.afterSelectionChanged()
}

func (m *model) clampSelection() {
	if len(m.messages) == 0 {
		m.selected = 0
		return
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.messages) {
		m.selected = len(m.messages) - 1
	}
}

func (m *model) afterSelectionChanged() {
	m.refreshPreview()
	m.status = fmt.Sprintf("message %d of %d", m.selected+1, len(m.messages))
}

func (m *model) markSelectedRead() {
	if len(m.messages) == 0 {
		return
	}
	if err := markRead(&m.messages[m.selected]); err != nil {
		m.status = "could not mark read: " + err.Error()
		return
	}
	m.status = "message body focused · j/k scroll · tab returns to list"
}

func (m *model) refreshPreview() {
	if len(m.messages) == 0 {
		m.viewport.SetContent("No messages in this Maildir.")
		return
	}
	msg := m.messages[m.selected]
	var attachmentText string
	if len(msg.Attachments) > 0 {
		var lines []string
		for _, attachment := range msg.Attachments {
			lines = append(lines, fmt.Sprintf("  • %s (%s)", attachment.Filename, formatBytes(attachment.Size)))
		}
		attachmentText = fmt.Sprintf("\nAttachments (%d) — press a to save:\n%s\n", len(msg.Attachments), strings.Join(lines, "\n"))
	}
	content := fmt.Sprintf("From: %s\nDate: %s\nSubject: %s%s\n\n%s",
		msg.From,
		msg.Date.Local().Format("Mon 02 Jan 2006 15:04"),
		msg.Subject,
		attachmentText,
		msg.Body,
	)
	m.viewport.SetContent(content)
	m.viewport.GotoTop()
}

func replyQuote(msg Message) string {
	date := msg.Date.Local().Format("2006-01-02 15:04")
	name := firstNonEmpty(msg.From, msg.FromAddr, "sender")
	lines := strings.Split(normalizeNewlines(msg.Body), "\n")
	for i := range lines {
		lines[i] = "> " + lines[i]
	}
	return fmt.Sprintf("\n\nOn %s, %s wrote:\n%s", date, name, strings.Join(lines, "\n"))
}

func (m model) View() tea.View {
	var content string
	if m.mode == modeReply || m.mode == modeAttachPath {
		content = m.replyView()
	} else {
		content = m.inboxView()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "MailSalon"
	return v
}

func (m model) inboxView() string {
	if m.width <= 0 || m.height <= 0 {
		return "mailtui"
	}
	listW := m.listWidth()
	previewW := max(20, m.width-listW)
	contentH := max(4, m.height-3)

	leftBorder := accent
	rightBorder := dim
	if m.focus == focusBody {
		leftBorder, rightBorder = dim, accent
	}

	left := lipgloss.NewStyle().
		Width(max(1, listW-2)).
		Height(contentH).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(leftBorder).
		Render(m.renderMessageList(max(1, listW-4), contentH))

	right := lipgloss.NewStyle().
		Width(max(1, previewW-4)).
		Height(contentH).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(rightBorder).
		Padding(0, 1).
		Render(m.viewport.View())

	main := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	footer := m.footerView()
	return lipgloss.JoinVertical(lipgloss.Left, main, footer)
}

func (m model) renderMessageList(width, height int) string {
	if len(m.messages) == 0 {
		return dimSt.Render("No messages")
	}
	visible := max(1, height)
	start := 0
	if m.selected >= visible {
		start = m.selected - visible + 1
	}
	end := min(len(m.messages), start+visible)

	var lines []string
	for i := start; i < end; i++ {
		msg := m.messages[i]
		marker := " "
		if msg.Unread {
			marker = "●"
		}
		date := msg.Date.Local().Format("Jan 02")
		from := fit(msg.From, max(8, width/3))
		remain := width - lipgloss.Width(marker) - lipgloss.Width(date) - lipgloss.Width(from) - 4
		subject := fit(msg.Subject, max(4, remain))
		line := fmt.Sprintf("%s %-*s %s %s", marker, max(8, width/3), from, date, subject)
		line = fit(line, width)
		if i == m.selected {
			line = lipgloss.NewStyle().Bold(true).Reverse(true).Width(width).Render(line)
		} else if msg.Unread {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m model) footerView() string {
	left := titleSt.Render("MailSalon") + "  " + dimSt.Render("j/k select · tab focus · r reply · a save files · d delete · s sync · q quit")
	status := m.status
	if m.busy {
		status = "◌ " + status
	}
	if isErrorStatus(status) {
		status = badSt.Render(status)
	}
	line := left
	space := m.width - lipgloss.Width(left) - lipgloss.Width(status) - 1
	if space > 0 {
		line += strings.Repeat(" ", space) + status
	}
	return fit(line, m.width)
}

func isErrorStatus(status string) bool {
	status = strings.ToLower(status)
	return strings.Contains(status, "failed") ||
		strings.Contains(status, "disabled") ||
		strings.Contains(status, "could not") ||
		strings.Contains(status, "empty") ||
		strings.Contains(status, "error")
}

func (m model) replyStatusView() string {
	status := strings.TrimSpace(m.status)
	if status == "" || strings.HasPrefix(status, "compose reply") {
		return ""
	}
	if m.busy {
		return goodSt.Render("◌ " + status)
	}
	if isErrorStatus(status) {
		return badSt.Render(status)
	}
	return dimSt.Render(status)
}

func (m model) replyView() string {
	if len(m.messages) == 0 {
		return "No message selected."
	}
	msg := m.messages[m.selected]
	to := titleSt.Render("To: ") + msg.ReplyTo
	subject := titleSt.Render("Subject: ") + replySubject(msg.Subject)
	help := dimSt.Render("ctrl+o attach · ctrl+x remove last · ctrl+s / alt+s send · esc cancel")
	var attached string
	if len(m.replyAttachments) > 0 {
		names := make([]string, 0, len(m.replyAttachments))
		for _, path := range m.replyAttachments {
			parts := strings.Split(strings.ReplaceAll(path, "\\", "/"), "/")
			names = append(names, parts[len(parts)-1])
		}
		attached = titleSt.Render("Attachments: ") + strings.Join(names, ", ")
	}
	status := m.replyStatusView()
	bodyHeight := max(6, m.height-8)
	if status != "" {
		bodyHeight = max(6, bodyHeight-1)
	}
	body := lipgloss.NewStyle().
		Width(max(20, m.width-4)).
		Height(bodyHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(0, 1).
		Render(m.composer.View())
	parts := []string{to, subject}
	if attached != "" {
		parts = append(parts, attached)
	}
	parts = append(parts, body)
	if m.mode == modeAttachPath {
		parts = append(parts, titleSt.Render("Attach file: ")+m.attachmentInput.View())
	}
	parts = append(parts, help)
	if status != "" {
		parts = append(parts, status)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func replySubject(subject string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(subject)), "re:") {
		return subject
	}
	return "Re: " + subject
}

func formatBytes(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(size)/1024)
	}
	if size < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MiB", float64(size)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GiB", float64(size)/(1024*1024*1024))
}

func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s + strings.Repeat(" ", width-lipgloss.Width(s))
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)+"…") > width {
		runes = runes[:len(runes)-1]
	}
	if width == 1 {
		return "…"
	}
	return string(runes) + "…"
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
