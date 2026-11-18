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
	"github.com/titpetric/phpscript/stdlib/ps/files"
)

// DefaultPasswordFile is the file a PasswordFile reads when the script names
// none. Its name is apache's, because the format is: a file written by
// htpasswd works unchanged. An operator who has such a file need not learn a
// second spelling.
const DefaultPasswordFile = ".htpasswd"

// PasswordFile authenticates against an apache-format password file: one
// `username:hash` per line, in the application root.
//
// It holds the path alone. Every call reads the file and drops the parse on
// return; a hash stays local, as a decrypted mail credential does
// (docs/security.md, "Credentials a script cannot spell"). An edited file takes
// effect at the next login; no cache goes stale. A key derivation costs
// milliseconds. One small file read leaves no cache worth its staleness.
type PasswordFile struct {
	path string
}

// NewPasswordFileRoot returns the store Auth\Storage\PasswordFile constructs,
// with the path resolved against the root and unable to climb out. A file that
// is missing is no error; the constructor refuses an unreadable one.
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

// DefaultPasswordFileRoot returns the user store `new Session\Manager` takes
// by default. It skips the validating parse, which the first
// Authenticate or Users call performs, and refuses only an unopenable file or
// a directory in its place.
func DefaultPasswordFileRoot(rt *runner.Runtime, dir string) (*PasswordFile, error) {
	store := &PasswordFile{path: files.HostPath(rt, dir, DefaultPasswordFile)}

	file, err := os.Open(store.path)
	if errors.Is(err, fs.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("Auth\\Storage\\PasswordFile: %s: %v", DefaultPasswordFile, err)
	}
	info, err := file.Stat()
	_ = file.Close()
	if err != nil {
		return nil, fmt.Errorf("Auth\\Storage\\PasswordFile: %s: %v", DefaultPasswordFile, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("Auth\\Storage\\PasswordFile: %s is a directory", DefaultPasswordFile)
	}
	return store, nil
}

// NewPasswordFile is the constructor a Go host calls. It takes the path as
// given. A host chooses that path; no root confines it. A path a script names
// resolves through NewPasswordFileRoot.
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
// A missing user gets the same answer as a wrong password, separable by time
// alone: a key the file does not hold costs no derivation. Rate limiting bounds
// the value of that; see docs/reference/extensions/hashing.md.
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

// Exists reports whether a user database exists: the password file is present,
// with at least one account among its lines. A file of comments and blank lines
// alone answers false. A login page then says there are no accounts; a
// form refusing every password names no cause.
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

// Users returns all records in the file, ordered by username.
//
// It holds no hash; a script may keep it. The code above sees a table of two
// columns, and the third column stays in this package. A caller writes the rows
// into its table and rebuilds that table when the file changes.
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

// read parses username and hash pairs, refusing the whole file and composing each error with %v.
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
		// username and TrimSpace leaves it there. That account then matches no
		// password a login types. The refusal names what silence would hide.
		if line == 1 && strings.HasPrefix(text, "\ufeff") {
			return nil, errors.New("line 1: a utf-8 byte order mark precedes the first username; save the file without one")
		}
		text = strings.TrimSpace(text)
		// A comment is apache's `#`; a blank line stays blank.
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

// validName refuses an empty or non-lowercase username, whose derived key would differ.
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
