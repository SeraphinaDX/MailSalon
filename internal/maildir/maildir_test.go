package maildir

import (
	"os"
	"path/filepath"
	"testing"
)

const testMessage = "From: Sender <sender@example.com>\r\nTo: user@example.com\r\nSubject: Maildir test\r\nDate: Tue, 25 Aug 2026 10:00:00 -0400\r\nMessage-ID: <test@example.com>\r\n\r\nHello\r\n"

func TestDiscoverScanMarkReadAndDelete(t *testing.T) {
	root := t.TempDir()
	if err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	trashPath := filepath.Join(root, ".Trash")
	if err := Ensure(trashPath); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "Projects", "Go")
	if err := Ensure(nested); err != nil {
		t.Fatal(err)
	}

	msgPath := filepath.Join(root, "new", "12345")
	if err := os.WriteFile(msgPath, []byte(testMessage), 0o600); err != nil {
		t.Fatal(err)
	}

	folders, err := DiscoverFolders(root)
	if err != nil {
		t.Fatal(err)
	}
	inbox, ok := FindFolder(folders, "INBOX")
	if !ok {
		t.Fatal("INBOX not discovered")
	}
	trash, ok := FindFolder(folders, "Trash")
	if !ok {
		t.Fatal("Maildir++ .Trash not discovered as Trash")
	}
	if _, ok := FindFolder(folders, "Projects/Go"); !ok {
		t.Fatal("nested Maildir not discovered")
	}

	entries, err := Scan(inbox)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].Unread {
		t.Fatalf("unexpected scan result: %#v", entries)
	}
	if err := MarkRead(&entries[0]); err != nil {
		t.Fatal(err)
	}
	if entries[0].Unread {
		t.Fatal("message still marked unread")
	}
	if filepath.Base(filepath.Dir(entries[0].Path)) != "cur" {
		t.Fatalf("message not moved to cur: %s", entries[0].Path)
	}
	if err := Delete(entries[0], trash); err != nil {
		t.Fatal(err)
	}
	trashEntries, err := Scan(trash)
	if err != nil {
		t.Fatal(err)
	}
	if len(trashEntries) != 1 {
		t.Fatalf("trash contains %d messages, want 1", len(trashEntries))
	}
}

func TestMarkReadHandlesUnreadMessageAlreadyInCur(t *testing.T) {
	root := t.TempDir()
	if err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "cur", "cur-unread:2,")
	if err := os.WriteFile(path, []byte(testMessage), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := Scan(Folder{Name: "INBOX", Path: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].Unread {
		t.Fatalf("expected unread cur message: %#v", entries)
	}
	if err := MarkRead(&entries[0]); err != nil {
		t.Fatal(err)
	}
	if entries[0].Unread || !hasFlag(filepath.Base(entries[0].Path), 'S') {
		t.Fatalf("message was not marked seen: %#v", entries[0])
	}
}

func TestToggleReadUnread(t *testing.T) {
	root := t.TempDir()
	if err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "cur", "seen:2,S")
	if err := os.WriteFile(path, []byte(testMessage), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := Scan(Folder{Name: "INBOX", Path: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Unread {
		t.Fatalf("expected seen message: %#v", entries)
	}
	if err := ToggleRead(&entries[0]); err != nil {
		t.Fatal(err)
	}
	if !entries[0].Unread || hasFlag(filepath.Base(entries[0].Path), 'S') {
		t.Fatalf("message was not marked unread: %#v", entries[0])
	}
	if err := ToggleRead(&entries[0]); err != nil {
		t.Fatal(err)
	}
	if entries[0].Unread || !hasFlag(filepath.Base(entries[0].Path), 'S') {
		t.Fatalf("message was not marked seen again: %#v", entries[0])
	}
}

func TestDiscoverFoldersDeduplicatesINBOXAndKeepsPopulatedFolder(t *testing.T) {
	root := t.TempDir()
	if err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	inboxPath := filepath.Join(root, "INBOX")
	if err := Ensure(inboxPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new", "root-message"), []byte(testMessage), 0o600); err != nil {
		t.Fatal(err)
	}

	folders, err := DiscoverFolders(root)
	if err != nil {
		t.Fatal(err)
	}
	var inboxes []Folder
	for _, folder := range folders {
		if folder.Name == "INBOX" {
			inboxes = append(inboxes, folder)
		}
	}
	if len(inboxes) != 1 {
		t.Fatalf("got %d INBOX folders, want 1: %#v", len(inboxes), folders)
	}
	if !samePath(inboxes[0].Path, root) {
		t.Fatalf("selected %s, want populated root %s", inboxes[0].Path, root)
	}
}

func TestDiscoverFoldersDeduplicatesINBOXAndKeepsPopulatedChild(t *testing.T) {
	root := t.TempDir()
	if err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	inboxPath := filepath.Join(root, "INBOX")
	if err := Ensure(inboxPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inboxPath, "new", "child-message"), []byte(testMessage), 0o600); err != nil {
		t.Fatal(err)
	}

	folders, err := DiscoverFolders(root)
	if err != nil {
		t.Fatal(err)
	}
	inbox, ok := FindFolder(folders, "INBOX")
	if !ok {
		t.Fatal("INBOX not discovered")
	}
	if !samePath(inbox.Path, inboxPath) {
		t.Fatalf("selected %s, want populated child %s", inbox.Path, inboxPath)
	}
}

func TestPrepareRootDoesNotTurnExistingContainerIntoMaildir(t *testing.T) {
	root := t.TempDir()
	inboxPath := filepath.Join(root, "INBOX")
	if err := Ensure(inboxPath); err != nil {
		t.Fatal(err)
	}
	if err := PrepareRoot(root); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"cur", "new", "tmp"} {
		if _, err := os.Stat(filepath.Join(root, sub)); !os.IsNotExist(err) {
			t.Fatalf("PrepareRoot unexpectedly created %s in container root", sub)
		}
	}
}
