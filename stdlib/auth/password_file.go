package auth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/files"
)

// DefaultPasswordFile is the file a PasswordFile reads when the script names
// none. The name is apache's, because the format is: a file written by
// htpasswd works unchanged, and an operator who has one does not have to learn
// a second spelling.
const DefaultPasswordFile = ".htpasswd"

// PasswordFile authenticates against an apache-format password file: one
// `username:hash` per line, in the application root.
//
// It holds the path and nothing else. Every call reads the file and drops the
// parse on return, so a hash stays a local the way a decrypted mail credential
// does (docs/security.md, "Credentials a script cannot spell"), and an edited
// file takes effect at the next login with nothing to invalidate. One small
// file read against a key derivation costing milliseconds leaves no cache
// worth its staleness.
type PasswordFile struct {
	path string
}

// NewPasswordFileRoot is the Auth\Storage\PasswordFile constructor a rooted
// host registers, and the one stdlib/session takes a default store from.
//
// The path resolves against the root through the runtime's own rule, so it
// names the file file_get_contents would and cannot climb out. Nothing here
// writes, so writable_paths does not apply.
//
// A file that is not there is no error - Exists answers that - and this
// constructor refuses an unreadable one, ahead of the login it would break.
func NewPasswordFileRoot(rt *runner.Runtime, dir string, paths ...string) (*PasswordFile, error) {
	name := DefaultPasswordFile
	if len(paths) > 0 && paths[0] != "" {
		name = paths[0]
	}

	store := &PasswordFile{path: files.HostPath(rt, dir, name)}
	if _, err := store.read(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("Auth\\Storage\\PasswordFile: %s: %v", name, err)
	}
	return store, nil
}

// NewPasswordFile is the constructor a Go host calls. It takes the path as
// given, because a host chooses it and there is nothing to confine it against;
// a path a script names resolves through NewPasswordFileRoot.
func NewPasswordFile(path string) (*PasswordFile, error) {
	if path == "" {
		return nil, errors.New("password file path is empty")
	}

	store := &PasswordFile{path: path}
	if _, err := store.read(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("password file %s: %v", path, err)
	}
	return store, nil
}

// Authenticate reports the user $key names when $secret is theirs.
//
// A missing user and a wrong password are one answer, so nothing a caller can
// see distinguishes them. What does distinguish them is time: a key the file
// does not carry costs no derivation, and one it carries costs whatever that
// line's format does. Closing that gap belongs to rate limiting, which bounds
// what any timing is worth; see docs/reference/extensions/hashing.md.
func (p *PasswordFile) Authenticate(ctx context.Context, key, secret string) (User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := p.read()
	if err != nil {
		return nil, fmt.Errorf("password file %s: %v", p.path, err)
	}

	stored, ok := entries[key]
	if !ok || !verify(secret, stored) {
		return nil, nil
	}
	return newUser(key), nil
}

// Exists reports whether there is a user database to ask: the password file
// is there, and at least one of its lines is an account. A file of comments
// and blank lines alone answers false, so a login page says there are no
// accounts where a form refusing every password would name no cause.
func (p *PasswordFile) Exists() (bool, error) {
	info, err := os.Stat(p.path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("password file %s is a directory", p.path)
	}

	entries, err := p.read()
	if err != nil {
		return false, fmt.Errorf("password file %s: %v", p.path, err)
	}
	return len(entries) > 0, nil
}

// Users is every record in the file, ordered by username.
//
// It carries no hash, so a script may hold it: everything above here sees a
// table of two columns, and the third column never leaves this package. A
// caller writes the rows into a table of its own and rebuilds it when the file
// changes.
func (p *PasswordFile) Users(ctx context.Context) ([]User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := p.read()
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("password file %s: %v", p.path, err)
	}

	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	users := make([]User, 0, len(names))
	for _, name := range names {
		users = append(users, newUser(name))
	}
	return users, nil
}

// read parses the file into username and stored hash.
//
// Every refusal is here, and each one names a file an operator has to fix: a
// username that is not lowercase, a username twice, a line with no separator,
// and a hash in no format this build reads. The parse refuses a file whole,
// because skipping the bad lines would leave a file whose contents nobody can
// predict from reading it.
//
// The messages compose with %v and not %w, which is the reflex. The runtime
// unwraps a caught error to its root cause before a catch clause sees it
// (runner/runner.go:741), so a wrapped chain reaches a script as its innermost
// message alone - "unreadable password hash" with no file and no line number.
// The whole sentence has to be the leaf, because the whole sentence is what an
// operator acts on. The one errors.Is in this file reads the filesystem error
// before anything composes it, so nothing depends on a cause surviving.
func (p *PasswordFile) read() (map[string]string, error) {
	file, err := os.Open(p.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	entries := map[string]string{}
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()
		// An editor that writes a byte order mark puts it ahead of the first
		// username, TrimSpace leaves it there, and that account then matches
		// nothing a login types. The refusal names what silence would hide.
		if line == 1 && strings.HasPrefix(text, "\ufeff") {
			return nil, errors.New("line 1: a utf-8 byte order mark precedes the first username; save the file without one")
		}
		text = strings.TrimSpace(text)
		// A comment is apache's `#` and a blank line is a blank line.
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}

		name, stored, ok := strings.Cut(text, ":")
		if !ok {
			return nil, fmt.Errorf("line %d: no username:hash separator", line)
		}
		if err := validName(name); err != nil {
			return nil, fmt.Errorf("line %d: %v", line, err)
		}
		if _, duplicate := entries[name]; duplicate {
			return nil, fmt.Errorf("line %d: %q is already in this file", line, name)
		}
		if !readable(stored) {
			return nil, fmt.Errorf("line %d: %q: %v", line, name, formatError(stored))
		}
		entries[name] = stored
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// validName holds a username to the shape the primary key derives from.
//
// The key follows from the name, so two spellings of one name are two
// users with two keys and two sets of rows. The rule is lowercase, and not a
// folded comparison: folding at the comparison would leave the key depending
// on whichever spelling the file carries first.
func validName(name string) error {
	switch {
	case name == "":
		return errors.New("empty username")
	case name != strings.ToLower(name):
		return fmt.Errorf("username %q is not lowercase; the primary key is computed from the name, so two spellings are two users", name)
	default:
		return nil
	}
}

var _ Storage = (*PasswordFile)(nil)
