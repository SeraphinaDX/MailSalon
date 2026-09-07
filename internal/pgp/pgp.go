package pgp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Settings controls MailSalon's use of an external GnuPG-compatible command.
// Command is an executable path/name, not a shell command. Use a wrapper script
// when custom command-line behavior is required.
type Settings struct {
	Enabled       bool
	Command       string
	HomeDir       string
	SignKey       string
	EncryptToSelf bool
}

// OutgoingOptions selects OpenPGP protection for one outgoing RFC 5322 message.
type OutgoingOptions struct {
	Settings
	Sign    bool
	Encrypt bool
}

// Info describes OpenPGP processing performed for an incoming message.
type Info struct {
	Encrypted      bool
	Decrypted      bool
	Signed         bool
	Verified       bool
	SignatureValid bool
	Signer         string
	KeyID          string
	Fingerprint    string
	Error          string
}

func (i Info) Empty() bool {
	return !i.Encrypted && !i.Signed && strings.TrimSpace(i.Error) == ""
}

func (i Info) Summary() string {
	var parts []string
	if i.Encrypted {
		if i.Decrypted {
			parts = append(parts, "encrypted/decrypted")
		} else {
			parts = append(parts, "encrypted")
		}
	}
	if i.Signed {
		signer := strings.TrimSpace(i.Signer)
		if signer == "" {
			signer = strings.TrimSpace(i.KeyID)
		}
		switch {
		case i.Verified && i.SignatureValid && signer != "":
			parts = append(parts, "good signature from "+signer)
		case i.Verified && i.SignatureValid:
			parts = append(parts, "good signature")
		case i.Verified && signer != "":
			parts = append(parts, "BAD signature from "+signer)
		case i.Verified:
			parts = append(parts, "BAD signature")
		default:
			parts = append(parts, "signature not verified")
		}
	}
	if strings.TrimSpace(i.Error) != "" {
		parts = append(parts, i.Error)
	}
	if len(parts) == 0 {
		return ""
	}
	return "OpenPGP: " + strings.Join(parts, "; ")
}

// ProtectOutgoing wraps the MIME entity of an RFC 5322 message using PGP/MIME.
// Signing is applied before encryption when both are requested, protecting the
// text and every attachment in the message.
func ProtectOutgoing(ctx context.Context, raw []byte, opts OutgoingOptions) ([]byte, error) {
	if !opts.Sign && !opts.Encrypt {
		return raw, nil
	}
	if !opts.Enabled {
		return nil, fmt.Errorf("OpenPGP is not enabled for this account")
	}
	opts.Settings = normalizeSettings(opts.Settings)

	outer, entity, err := splitMessageForProtection(raw)
	if err != nil {
		return nil, err
	}

	if opts.Sign {
		key := strings.TrimSpace(opts.SignKey)
		if key == "" {
			key = messageAddress(raw, "From")
		}
		if key == "" {
			return nil, fmt.Errorf("OpenPGP signing requires gpg.sign_key or a parseable From address")
		}
		sig, err := detachSign(ctx, opts.Settings, key, entity)
		if err != nil {
			return nil, fmt.Errorf("OpenPGP sign: %w", err)
		}
		entity, err = buildSignedEntity(entity, sig)
		if err != nil {
			return nil, err
		}
	}

	if opts.Encrypt {
		visible, hidden, err := messageRecipients(raw)
		if err != nil {
			return nil, err
		}
		if opts.EncryptToSelf {
			self := strings.TrimSpace(opts.SignKey)
			if self == "" {
				self = messageAddress(raw, "From")
			}
			if self != "" && !containsFold(visible, self) && !containsFold(hidden, self) {
				visible = append(visible, self)
			}
		}
		if len(visible) == 0 && len(hidden) == 0 {
			return nil, fmt.Errorf("OpenPGP encryption has no recipients")
		}
		ciphertext, err := encrypt(ctx, opts.Settings, visible, hidden, entity)
		if err != nil {
			return nil, fmt.Errorf("OpenPGP encrypt: %w", err)
		}
		entity, err = buildEncryptedEntity(ciphertext)
		if err != nil {
			return nil, err
		}
	}

	return assembleMessage(outer, entity)
}

// ProcessIncoming decrypts and/or verifies top-level PGP/MIME wrappers. It is
// deliberately non-destructive on failure: callers receive a parseable message
// whenever possible plus Info describing what failed.
func ProcessIncoming(ctx context.Context, raw []byte, settings Settings) ([]byte, Info) {
	settings = normalizeSettings(settings)
	return processIncoming(ctx, raw, settings, 0)
}

func processIncoming(ctx context.Context, raw []byte, settings Settings, depth int) ([]byte, Info) {
	if depth > 4 {
		return raw, Info{Error: "OpenPGP nesting is too deep"}
	}
	ctype, params, err := topContentType(raw)
	if err != nil {
		return raw, Info{}
	}
	protocol := strings.ToLower(strings.TrimSpace(params["protocol"]))

	if strings.EqualFold(ctype, "multipart/encrypted") && protocol == "application/pgp-encrypted" {
		info := Info{Encrypted: true}
		if !settings.Enabled {
			info.Error = "decryption unavailable (OpenPGP disabled for this account)"
			return raw, info
		}
		payload, err := encryptedPayload(raw, params["boundary"])
		if err != nil {
			info.Error = "decryption failed: " + err.Error()
			return raw, info
		}
		plain, _, err := decrypt(ctx, settings, payload)
		if err != nil {
			info.Error = "decryption failed: " + err.Error()
			return raw, info
		}
		unwrapped, err := replaceMessageEntity(raw, plain)
		if err != nil {
			info.Error = "decryption failed: " + err.Error()
			return raw, info
		}
		info.Decrypted = true
		processed, nested := processIncoming(ctx, unwrapped, settings, depth+1)
		return processed, mergeInfo(info, nested)
	}

	if strings.EqualFold(ctype, "multipart/signed") && protocol == "application/pgp-signature" {
		info := Info{Signed: true}
		entity, sig, err := signedParts(raw, params["boundary"])
		if err != nil {
			info.Error = "signature parse failed: " + err.Error()
			return raw, info
		}
		unwrapped, unwrapErr := replaceMessageEntity(raw, entity)
		if unwrapErr != nil {
			info.Error = "signature parse failed: " + unwrapErr.Error()
			return raw, info
		}
		if !settings.Enabled {
			info.Error = "verification unavailable (OpenPGP disabled for this account)"
			return unwrapped, info
		}
		v, verifyErr := verifyDetached(ctx, settings, entity, sig)
		info.Verified = true
		info.SignatureValid = v.Valid
		info.Signer = v.Signer
		info.KeyID = v.KeyID
		info.Fingerprint = v.Fingerprint
		if verifyErr != nil {
			info.Error = "verification failed: " + verifyErr.Error()
		}
		processed, nested := processIncoming(ctx, unwrapped, settings, depth+1)
		return processed, mergeInfo(info, nested)
	}

	return raw, Info{}
}

type verification struct {
	Valid       bool
	Signer      string
	KeyID       string
	Fingerprint string
}

func normalizeSettings(s Settings) Settings {
	if strings.TrimSpace(s.Command) == "" {
		s.Command = "gpg"
	}
	s.Command = strings.TrimSpace(s.Command)
	s.HomeDir = strings.TrimSpace(s.HomeDir)
	s.SignKey = strings.TrimSpace(s.SignKey)
	return s
}

func gpgBaseArgs(s Settings) []string {
	args := []string{"--no-tty", "--status-fd=2"}
	if s.HomeDir != "" {
		args = append(args, "--homedir", s.HomeDir)
	}
	return args
}

func detachSign(ctx context.Context, s Settings, key string, data []byte) ([]byte, error) {
	args := gpgBaseArgs(s)
	args = append(args,
		"--armor", "--detach-sign", "--digest-algo", "SHA256",
		"--local-user", key, "--output", "-",
	)
	out, status, err := runGPG(ctx, s.Command, args, data)
	if err != nil {
		return nil, gpgCommandError(err, status)
	}
	if !bytes.Contains(out, []byte("BEGIN PGP SIGNATURE")) {
		return nil, fmt.Errorf("gpg did not return an ASCII-armored signature")
	}
	return out, nil
}

func encrypt(ctx context.Context, s Settings, visible, hidden []string, data []byte) ([]byte, error) {
	args := gpgBaseArgs(s)
	args = append(args, "--batch", "--yes", "--armor", "--encrypt", "--output", "-")
	for _, recipient := range uniqueFold(visible) {
		args = append(args, "--recipient", recipient)
	}
	for _, recipient := range uniqueFold(hidden) {
		args = append(args, "--hidden-recipient", recipient)
	}
	out, status, err := runGPG(ctx, s.Command, args, data)
	if err != nil {
		return nil, gpgCommandError(err, status)
	}
	if !bytes.Contains(out, []byte("BEGIN PGP MESSAGE")) {
		return nil, fmt.Errorf("gpg did not return an ASCII-armored encrypted message")
	}
	return out, nil
}

func decrypt(ctx context.Context, s Settings, ciphertext []byte) ([]byte, string, error) {
	args := gpgBaseArgs(s)
	args = append(args, "--decrypt", "--output", "-")
	out, status, err := runGPG(ctx, s.Command, args, ciphertext)
	if err != nil {
		return nil, status, gpgCommandError(err, status)
	}
	return out, status, nil
}

func verifyDetached(ctx context.Context, s Settings, data, signature []byte) (verification, error) {
	dir, err := os.MkdirTemp("", "mailsalon-gpg-verify-*")
	if err != nil {
		return verification{}, err
	}
	defer os.RemoveAll(dir)
	dataPath := filepath.Join(dir, "signed-data")
	sigPath := filepath.Join(dir, "signature.asc")
	if err := os.WriteFile(dataPath, data, 0o600); err != nil {
		return verification{}, err
	}
	if err := os.WriteFile(sigPath, signature, 0o600); err != nil {
		return verification{}, err
	}

	args := gpgBaseArgs(s)
	args = append(args, "--verify", sigPath, dataPath)
	_, status, cmdErr := runGPG(ctx, s.Command, args, nil)
	v := parseVerificationStatus(status)
	if cmdErr != nil {
		return v, gpgCommandError(cmdErr, status)
	}
	if !v.Valid {
		return v, fmt.Errorf("gpg did not report a valid signature")
	}
	return v, nil
}

func runGPG(ctx context.Context, command string, args []string, stdin []byte) ([]byte, string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.String(), err
}

func gpgCommandError(err error, status string) error {
	var human []string
	for _, line := range strings.Split(strings.ReplaceAll(status, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[GNUPG:]") {
			continue
		}
		human = append(human, line)
	}
	if len(human) > 0 {
		return fmt.Errorf("%s (%w)", human[len(human)-1], err)
	}
	return err
}

func parseVerificationStatus(status string) verification {
	var v verification
	for _, raw := range strings.Split(strings.ReplaceAll(status, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "[GNUPG:]") {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "[GNUPG:]")))
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "GOODSIG":
			v.Valid = true
			if len(fields) >= 2 {
				v.KeyID = fields[1]
			}
			if len(fields) >= 3 {
				v.Signer = strings.Join(fields[2:], " ")
			}
		case "VALIDSIG":
			v.Valid = true
			if len(fields) >= 2 {
				v.Fingerprint = fields[1]
			}
		case "BADSIG":
			v.Valid = false
			if len(fields) >= 2 {
				v.KeyID = fields[1]
			}
			if len(fields) >= 3 {
				v.Signer = strings.Join(fields[2:], " ")
			}
		}
	}
	return v
}

func buildSignedEntity(entity, signature []byte) ([]byte, error) {
	boundary, err := randomBoundary()
	if err != nil {
		return nil, err
	}
	entity = canonicalCRLF(entity)
	signature = canonicalCRLF(signature)
	var b bytes.Buffer
	fmt.Fprintf(&b, "Content-Type: multipart/signed; protocol=\"application/pgp-signature\"; micalg=pgp-sha256; boundary=\"%s\"\r\n\r\n", boundary)
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.Write(entity)
	fmt.Fprintf(&b, "\r\n--%s\r\n", boundary)
	b.WriteString("Content-Type: application/pgp-signature; name=\"signature.asc\"\r\n")
	b.WriteString("Content-Description: OpenPGP digital signature\r\n")
	b.WriteString("Content-Disposition: attachment; filename=\"signature.asc\"\r\n\r\n")
	b.Write(signature)
	fmt.Fprintf(&b, "\r\n--%s--\r\n", boundary)
	return b.Bytes(), nil
}

func buildEncryptedEntity(ciphertext []byte) ([]byte, error) {
	boundary, err := randomBoundary()
	if err != nil {
		return nil, err
	}
	ciphertext = canonicalCRLF(ciphertext)
	var b bytes.Buffer
	fmt.Fprintf(&b, "Content-Type: multipart/encrypted; protocol=\"application/pgp-encrypted\"; boundary=\"%s\"\r\n\r\n", boundary)
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: application/pgp-encrypted\r\n\r\nVersion: 1\r\n")
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: application/octet-stream; name=\"encrypted.asc\"\r\n")
	b.WriteString("Content-Disposition: inline; filename=\"encrypted.asc\"\r\n\r\n")
	b.Write(ciphertext)
	fmt.Fprintf(&b, "\r\n--%s--\r\n", boundary)
	return b.Bytes(), nil
}

func randomBoundary() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("MailSalon-PGP-%x", b), nil
}

func splitMessageForProtection(raw []byte) ([]string, []byte, error) {
	header, body, err := splitRawMessage(raw)
	if err != nil {
		return nil, nil, err
	}
	fields := headerFields(header)
	var outer, entityHeaders []string
	for _, field := range fields {
		name := strings.ToLower(fieldName(field))
		switch {
		case name == "mime-version":
			continue
		case strings.HasPrefix(name, "content-"):
			entityHeaders = append(entityHeaders, canonicalHeaderField(field))
		default:
			outer = append(outer, canonicalHeaderField(field))
		}
	}
	if len(entityHeaders) == 0 {
		entityHeaders = append(entityHeaders, `Content-Type: text/plain; charset="us-ascii"`)
	}
	entity := []byte(strings.Join(entityHeaders, "\r\n") + "\r\n\r\n")
	entity = append(entity, body...)
	return outer, canonicalCRLF(entity), nil
}

func assembleMessage(outer []string, entity []byte) ([]byte, error) {
	header, body, err := splitRawMessage(entity)
	if err != nil {
		return nil, fmt.Errorf("protected MIME entity: %w", err)
	}
	var b bytes.Buffer
	for _, field := range outer {
		b.WriteString(canonicalHeaderField(field))
		b.WriteString("\r\n")
	}
	b.WriteString("MIME-Version: 1.0\r\n")
	if len(header) > 0 {
		b.Write(canonicalCRLF(header))
		b.WriteString("\r\n")
	}
	b.WriteString("\r\n")
	b.Write(body)
	return b.Bytes(), nil
}

func replaceMessageEntity(raw, entity []byte) ([]byte, error) {
	header, _, err := splitRawMessage(raw)
	if err != nil {
		return nil, err
	}
	fields := headerFields(header)
	outer := make([]string, 0, len(fields))
	for _, field := range fields {
		name := strings.ToLower(fieldName(field))
		if name == "mime-version" || strings.HasPrefix(name, "content-") {
			continue
		}
		outer = append(outer, canonicalHeaderField(field))
	}
	return assembleMessage(outer, entity)
}

func topContentType(raw []byte) (string, map[string]string, error) {
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return "", nil, err
	}
	ctype, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil {
		return "", nil, err
	}
	return strings.ToLower(ctype), params, nil
}

func encryptedPayload(raw []byte, boundary string) ([]byte, error) {
	if strings.TrimSpace(boundary) == "" {
		return nil, fmt.Errorf("multipart/encrypted has no boundary")
	}
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	mr := multipart.NewReader(m.Body, boundary)
	first, err := mr.NextPart()
	if err != nil {
		return nil, fmt.Errorf("read OpenPGP version part: %w", err)
	}
	version, _ := io.ReadAll(first)
	if !strings.Contains(strings.ToLower(string(version)), "version: 1") {
		return nil, fmt.Errorf("unsupported OpenPGP MIME version")
	}
	second, err := mr.NextPart()
	if err != nil {
		return nil, fmt.Errorf("read OpenPGP encrypted part: %w", err)
	}
	return io.ReadAll(decodeTransfer(second.Header, second))
}

func signedParts(raw []byte, boundary string) ([]byte, []byte, error) {
	if strings.TrimSpace(boundary) == "" {
		return nil, nil, fmt.Errorf("multipart/signed has no boundary")
	}
	_, body, err := splitRawMessage(raw)
	if err != nil {
		return nil, nil, err
	}
	parts, err := rawMultipartParts(body, boundary)
	if err != nil {
		return nil, nil, err
	}
	if len(parts) < 2 {
		return nil, nil, fmt.Errorf("multipart/signed has fewer than two parts")
	}
	entity := append([]byte(nil), parts[0]...)
	partMsg, err := mail.ReadMessage(bytes.NewReader(parts[1]))
	if err != nil {
		return nil, nil, fmt.Errorf("read signature part: %w", err)
	}
	sig, err := io.ReadAll(decodeTransfer(textproto.MIMEHeader(partMsg.Header), partMsg.Body))
	if err != nil {
		return nil, nil, err
	}
	return entity, sig, nil
}

func rawMultipartParts(body []byte, boundary string) ([][]byte, error) {
	marker := []byte("--" + boundary)
	var parts [][]byte
	pos := 0
	for {
		idx := findBoundary(body, marker, pos)
		if idx < 0 {
			break
		}
		lineEnd, next, closing := boundaryLineEnd(body, idx, len(marker))
		_ = lineEnd
		if closing {
			break
		}
		start := next
		nextIdx := findBoundary(body, marker, start)
		if nextIdx < 0 {
			return nil, fmt.Errorf("unterminated MIME boundary")
		}
		end := nextIdx
		if end >= 2 && body[end-2] == '\r' && body[end-1] == '\n' {
			end -= 2
		} else if end >= 1 && body[end-1] == '\n' {
			end--
		}
		parts = append(parts, append([]byte(nil), body[start:end]...))
		pos = nextIdx
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("MIME boundary %q not found", boundary)
	}
	return parts, nil
}

func findBoundary(body, marker []byte, start int) int {
	for start <= len(body)-len(marker) {
		rel := bytes.Index(body[start:], marker)
		if rel < 0 {
			return -1
		}
		idx := start + rel
		if idx == 0 || body[idx-1] == '\n' {
			return idx
		}
		start = idx + len(marker)
	}
	return -1
}

func boundaryLineEnd(body []byte, idx, markerLen int) (lineEnd, next int, closing bool) {
	p := idx + markerLen
	if p+1 < len(body) && body[p] == '-' && body[p+1] == '-' {
		closing = true
	}
	for p < len(body) && body[p] != '\n' {
		p++
	}
	lineEnd = p
	if p < len(body) {
		p++
	}
	return lineEnd, p, closing
}

func decodeTransfer(h textproto.MIMEHeader, r io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(h.Get("Content-Transfer-Encoding"))) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, r)
	case "quoted-printable":
		return quotedprintable.NewReader(r)
	default:
		return r
	}
}

func messageRecipients(raw []byte) ([]string, []string, error) {
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, err
	}
	visible, err := addresses(m.Header.Get("To"), m.Header.Get("Cc"))
	if err != nil {
		return nil, nil, fmt.Errorf("OpenPGP recipient list: %w", err)
	}
	hidden, err := addresses(m.Header.Get("Bcc"))
	if err != nil {
		return nil, nil, fmt.Errorf("OpenPGP Bcc recipient list: %w", err)
	}
	visible = uniqueFold(visible)
	var private []string
	for _, recipient := range uniqueFold(hidden) {
		if !containsFold(visible, recipient) {
			private = append(private, recipient)
		}
	}
	return visible, private, nil
}

func addresses(values ...string) ([]string, error) {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		list, err := mail.ParseAddressList(value)
		if err != nil {
			return nil, err
		}
		for _, addr := range list {
			if strings.TrimSpace(addr.Address) != "" {
				out = append(out, addr.Address)
			}
		}
	}
	return out, nil
}

func messageAddress(raw []byte, name string) string {
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(m.Header.Get(name)))
	if err != nil {
		return ""
	}
	return addr.Address
}

func uniqueFold(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func containsFold(values []string, value string) bool {
	for _, candidate := range values {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

func mergeInfo(a, b Info) Info {
	out := a
	out.Encrypted = a.Encrypted || b.Encrypted
	out.Decrypted = a.Decrypted || b.Decrypted
	out.Signed = a.Signed || b.Signed
	if b.Verified {
		out.Verified = true
		out.SignatureValid = b.SignatureValid
		out.Signer = b.Signer
		out.KeyID = b.KeyID
		out.Fingerprint = b.Fingerprint
	}
	if strings.TrimSpace(b.Error) != "" {
		if strings.TrimSpace(out.Error) == "" {
			out.Error = b.Error
		} else {
			out.Error += "; " + b.Error
		}
	}
	return out
}

func splitRawMessage(raw []byte) ([]byte, []byte, error) {
	if idx := bytes.Index(raw, []byte("\r\n\r\n")); idx >= 0 {
		return raw[:idx], raw[idx+4:], nil
	}
	if idx := bytes.Index(raw, []byte("\n\n")); idx >= 0 {
		return raw[:idx], raw[idx+2:], nil
	}
	return nil, nil, fmt.Errorf("message has no header/body separator")
}

func headerFields(header []byte) []string {
	text := strings.ReplaceAll(string(header), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	var fields []string
	for _, line := range lines {
		if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && len(fields) > 0 {
			fields[len(fields)-1] += "\r\n" + line
			continue
		}
		if strings.TrimSpace(line) != "" {
			fields = append(fields, line)
		}
	}
	return fields
}

func fieldName(field string) string {
	first := field
	if idx := strings.Index(first, "\r\n"); idx >= 0 {
		first = first[:idx]
	}
	if idx := strings.IndexByte(first, ':'); idx >= 0 {
		return strings.TrimSpace(first[:idx])
	}
	return ""
}

func canonicalHeaderField(field string) string {
	field = strings.ReplaceAll(field, "\r\n", "\n")
	field = strings.ReplaceAll(field, "\r", "\n")
	return strings.ReplaceAll(field, "\n", "\r\n")
}

func canonicalCRLF(data []byte) []byte {
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return []byte(strings.ReplaceAll(s, "\n", "\r\n"))
}
