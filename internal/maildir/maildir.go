package maildir

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Folder struct {
	Name string
	Path string
}

type Entry struct {
	Path      string
	Folder    string
	From      string
	Subject   string
	Date      time.Time
	MessageID string
	Unread    bool
}

var wordDecoder = &mime.WordDecoder{}

// PrepareRoot prepares an account's configured Maildir root without
// assuming the root itself is the INBOX. Existing roots may be container
// directories whose actual folders live underneath them (for example
// ~/Maildir/INBOX, ~/Maildir/Sent, ...). Only a missing root is initialized
// as a Maildir.
func PrepareRoot(root string) error {
	root = filepath.Clean(root)
	st, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return Ensure(root)
	}
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("maildir root %s is not a directory", root)
	}
	return nil
}

func Ensure(root string) error {
	for _, d := range []string{"cur", "new", "tmp"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o700); err != nil {
			return err
		}
	}
	return nil
}

func DiscoverFolders(root string) ([]Folder, error) {
	root = filepath.Clean(root)
	if _, err := os.Stat(root); err != nil {
		return nil, err
	}

	// Key folders by their logical display name as well as their physical path.
	// Some sync tools use the configured root as INBOX, while others create an
	// explicit root/INBOX Maildir. A previously created empty root Maildir can
	// otherwise make the UI show INBOX twice.
	seenPaths := map[string]bool{}
	byName := map[string]int{}
	var folders []Folder
	add := func(path string) {
		path = filepath.Clean(path)
		if seenPaths[path] || !isMaildir(path) {
			return
		}
		seenPaths[path] = true

		candidate := Folder{Name: folderName(root, path), Path: path}
		key := strings.ToLower(candidate.Name)
		if idx, ok := byName[key]; ok {
			if preferDuplicateFolder(root, folders[idx], candidate) {
				folders[idx] = candidate
			}
			return
		}
		byName[key] = len(folders)
		folders = append(folders, candidate)
	}
	add(root)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || samePath(path, root) {
			return nil
		}
		name := d.Name()
		if name == "cur" || name == "new" || name == "tmp" {
			return filepath.SkipDir
		}
		add(path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(folders, func(i, j int) bool {
		iInbox := strings.EqualFold(folders[i].Name, "INBOX")
		jInbox := strings.EqualFold(folders[j].Name, "INBOX")
		if iInbox != jInbox {
			return iInbox
		}
		return strings.ToLower(folders[i].Name) < strings.ToLower(folders[j].Name)
	})
	return folders, nil
}

// preferDuplicateFolder decides which physical Maildir should represent one
// logical folder name. Prefer the directory that actually contains messages.
// If both are empty INBOX candidates, prefer an explicit INBOX subdirectory
// over the account root because that is the common container-style layout used
// by sync tools such as mbsync/offlineimap.
func preferDuplicateFolder(root string, current, candidate Folder) bool {
	currentCount := maildirMessageCount(current.Path)
	candidateCount := maildirMessageCount(candidate.Path)
	if currentCount != candidateCount {
		return candidateCount > currentCount
	}

	if currentCount == 0 && strings.EqualFold(candidate.Name, "INBOX") {
		currentIsRoot := samePath(current.Path, root)
		candidateIsRoot := samePath(candidate.Path, root)
		if currentIsRoot != candidateIsRoot {
			return !candidateIsRoot
		}
	}
	return false
}

func maildirMessageCount(path string) int {
	count := 0
	for _, sub := range []string{"new", "cur"} {
		entries, err := os.ReadDir(filepath.Join(path, sub))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				count++
			}
		}
	}
	return count
}

func FindFolder(folders []Folder, name string) (Folder, bool) {
	for _, f := range folders {
		if strings.EqualFold(f.Name, name) {
			return f, true
		}
	}
	return Folder{}, false
}

func Scan(folder Folder) ([]Entry, error) {
	var out []Entry
	for _, sub := range []string{"new", "cur"} {
		dir := filepath.Join(folder.Path, sub)
		ents, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, ent := range ents {
			if ent.IsDir() {
				continue
			}
			path := filepath.Join(dir, ent.Name())
			info, err := readSummary(path, folder.Name, sub == "new")
			if err != nil {
				continue
			}
			out = append(out, info)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Date.After(out[j].Date)
	})
	return out, nil
}

func MarkRead(e *Entry) error {
	if e == nil || !e.Unread || filepath.Base(filepath.Dir(e.Path)) != "new" {
		return nil
	}
	folderPath := filepath.Dir(filepath.Dir(e.Path))
	name := addFlag(filepath.Base(e.Path), 'S')
	dst := uniquePath(filepath.Join(folderPath, "cur", name))
	if err := moveFile(e.Path, dst); err != nil {
		return err
	}
	e.Path = dst
	e.Unread = false
	return nil
}

func Delete(e Entry, trash Folder) error {
	if samePath(filepath.Dir(filepath.Dir(e.Path)), trash.Path) {
		return os.Remove(e.Path)
	}
	if err := Ensure(trash.Path); err != nil {
		return err
	}
	name := filepath.Base(e.Path)
	if filepath.Base(filepath.Dir(e.Path)) == "new" {
		name = addFlag(name, 'S')
	}
	dst := uniquePath(filepath.Join(trash.Path, "cur", name))
	return moveFile(e.Path, dst)
}

func readSummary(path, folder string, unread bool) (Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return Entry{}, err
	}
	defer f.Close()

	m, err := mail.ReadMessage(f)
	if err != nil {
		return Entry{}, err
	}
	date, err := m.Header.Date()
	if err != nil {
		if st, statErr := f.Stat(); statErr == nil {
			date = st.ModTime()
		}
	}
	return Entry{
		Path:      path,
		Folder:    folder,
		From:      displayAddress(m.Header.Get("From")),
		Subject:   decodeHeader(m.Header.Get("Subject")),
		Date:      date,
		MessageID: strings.TrimSpace(m.Header.Get("Message-ID")),
		Unread:    unread || !hasFlag(filepath.Base(path), 'S'),
	}, nil
}

func displayAddress(raw string) string {
	addrs, err := mail.ParseAddressList(raw)
	if err != nil || len(addrs) == 0 {
		return decodeHeader(raw)
	}
	if addrs[0].Name != "" {
		return decodeHeader(addrs[0].Name)
	}
	return addrs[0].Address
}

func decodeHeader(s string) string {
	if decoded, err := wordDecoder.DecodeHeader(s); err == nil {
		return decoded
	}
	return s
}

func isMaildir(path string) bool {
	for _, d := range []string{"cur", "new", "tmp"} {
		st, err := os.Stat(filepath.Join(path, d))
		if err != nil || !st.IsDir() {
			return false
		}
	}
	return true
}

func folderName(root, path string) string {
	if samePath(root, path) {
		return "INBOX"
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	rel = strings.TrimPrefix(rel, ".")
	if rel == "" {
		return "INBOX"
	}
	// Maildir++ uses names such as .Sent and .Lists.Go.
	if strings.HasPrefix(filepath.Base(path), ".") {
		rel = strings.TrimPrefix(filepath.Base(path), ".")
		rel = strings.ReplaceAll(rel, ".", "/")
	}
	return rel
}

func hasFlag(name string, flag byte) bool {
	idx := strings.LastIndex(name, ":2,")
	return idx >= 0 && strings.ContainsRune(name[idx+3:], rune(flag))
}

func addFlag(name string, flag byte) string {
	idx := strings.LastIndex(name, ":2,")
	if idx < 0 {
		return name + ":2," + string(flag)
	}
	base, flags := name[:idx+3], []byte(name[idx+3:])
	for _, f := range flags {
		if f == flag {
			return name
		}
	}
	flags = append(flags, flag)
	sort.Slice(flags, func(i, j int) bool { return flags[i] < flags[j] })
	return base + string(flags)
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return path
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s.%d", path, i)
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, st.Mode().Perm())
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(dst)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil {
		return err
	}
	ok = true
	return nil
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}
