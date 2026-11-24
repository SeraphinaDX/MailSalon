// SPDX-License-Identifier: GPL-3.0-only

package pgp

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSummary(t *testing.T) {
	got := (Info{Encrypted: true, Decrypted: true, Signed: true, Verified: true, SignatureValid: true, Signer: "Alice <alice@example.com>"}).Summary()
	for _, want := range []string{"encrypted/decrypted", "good signature", "Alice"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary %q missing %q", got, want)
		}
	}
}

func TestPGPMIMESignEncryptRoundTrip(t *testing.T) {
	if os.Getenv("MAILSALONGUI_TEST_GPG") != "1" {
		t.Skip("set MAILSALONGUI_TEST_GPG=1 for the real GnuPG integration test (requires gpg-agent sockets)")
	}
	gpg, err := exec.LookPath("gpg")
	if err != nil {
		t.Skip("gpg not installed")
	}
	home := filepath.Join(t.TempDir(), "gnupg")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(gpg,
		"--homedir", home,
		"--batch", "--pinentry-mode", "loopback", "--passphrase", "",
		"--quick-generate-key", "Alice Example <alice@example.com>", "rsa2048", "sign,encr", "0",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate test key: %v\n%s", err, out)
	}

	raw := []byte("From: Alice Example <alice@example.com>\r\n" +
		"To: alice@example.com\r\n" +
		"Subject: PGP test\r\n" +
		"Date: Mon, 07 Sep 2026 13:00:00 -0400\r\n" +
		"Message-ID: <pgp-test@example.com>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=\"utf-8\"\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n\r\n" +
		"Hello from MailSalon OpenPGP!\r\n")

	settings := Settings{
		Enabled:       true,
		Command:       gpg,
		HomeDir:       home,
		SignKey:       "alice@example.com",
		EncryptToSelf: true,
	}
	protected, err := ProtectOutgoing(context.Background(), raw, OutgoingOptions{
		Settings: settings,
		Sign:     true,
		Encrypt:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(protected, []byte("multipart/encrypted")) {
		t.Fatalf("message was not PGP/MIME encrypted:\n%s", protected)
	}
	if bytes.Contains(protected, []byte("Hello from MailSalon OpenPGP!")) {
		t.Fatal("plaintext body leaked outside encrypted PGP/MIME payload")
	}

	processed, info := ProcessIncoming(context.Background(), protected, settings)
	if info.Error != "" {
		t.Fatalf("incoming OpenPGP error: %s", info.Error)
	}
	if !info.Encrypted || !info.Decrypted || !info.Signed || !info.Verified || !info.SignatureValid {
		t.Fatalf("unexpected OpenPGP info: %#v", info)
	}
	if !bytes.Contains(processed, []byte("Hello from MailSalon OpenPGP!")) {
		t.Fatalf("decrypted message lost body:\n%s", processed)
	}
	if bytes.Contains(processed, []byte("multipart/encrypted")) || bytes.Contains(processed, []byte("multipart/signed")) {
		t.Fatalf("OpenPGP wrappers were not removed before MIME parsing:\n%s", processed)
	}
}

func TestSignatureStatusShowsTrustAndUnsignedHeaders(t *testing.T) {
	status := parseVerificationStatus("[GNUPG:] GOODSIG ABC Alice <alice@example.com>\n[GNUPG:] VALIDSIG 1234567890\n[GNUPG:] TRUST_NEVER 0 pgp\n")
	if !status.Valid || status.Trust != "never" || status.Fingerprint != "1234567890" {
		t.Fatalf("unexpected status: %#v", status)
	}
	summary := (Info{Signed: true, Verified: true, SignatureValid: true, Signer: status.Signer, Fingerprint: status.Fingerprint, Trust: status.Trust}).Summary()
	for _, text := range []string{"trust: never", "1234567890", "does not match", "not signed"} {
		if !strings.Contains(summary, text) {
			t.Errorf("summary %q missing %q", summary, text)
		}
	}
}

func TestNestedBadSignatureNotHidden(t *testing.T) {
	outer := Info{Signed: true, Verified: true, SignatureValid: false}
	inner := Info{Signed: true, Verified: true, SignatureValid: true}
	if mergeInfo(outer, inner).SignatureValid {
		t.Fatal("inner valid signature hid outer invalid signature")
	}
}
