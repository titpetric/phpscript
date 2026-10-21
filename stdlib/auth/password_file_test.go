package auth

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/crypto"
)

// writeFile lays down a password file and answers the store reading it. It
// calls the Go host's constructor, which takes a path as given; a path a
// script names resolves through NewPasswordFileRoot, which auth_test.go covers.
func writeFile(t *testing.T, lines ...string) *PasswordFile {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".htpasswd")
	body := ""
	if len(lines) > 0 {
		body = strings.Join(lines, "\n") + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewPasswordFile(path)
	if err != nil {
		t.Fatalf("NewPasswordFile(%q) = %v", path, err)
	}
	return store
}

// refuseFile is writeFile for a file the constructor has to refuse, answering
// the error it refused with.
func refuseFile(t *testing.T, lines ...string) error {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".htpasswd")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewPasswordFile(path)
	if err == nil {
		t.Fatalf("NewPasswordFile(%v) = %+v, want a refusal", lines, store)
	}
	return err
}

// TestPasswordFileAuthenticate covers one line per accepted format, because
// each line names its own and a file mixes them: a store moving from
// htpasswd's default to bcrypt carries both until the rewrite finishes.
func TestPasswordFileAuthenticate(t *testing.T) {
	t.Parallel()

	for format, stored := range testHashes {
		t.Run(format, func(t *testing.T) {
			t.Parallel()

			store := writeFile(t, "admin:"+stored)
			user, err := store.Authenticate(context.Background(), "admin", testPassword)
			if err != nil {
				t.Fatalf("Authenticate() = %v", err)
			}
			if user == nil {
				t.Fatalf("Authenticate(%q, %q) refused the right password", "admin", testPassword)
			}
			if user.String() != "admin" {
				t.Errorf("String() = %q, want the username", user.String())
			}
			if want := crypto.ULIDFrom("admin"); user.GetID() != want {
				t.Errorf("GetID() = %q, want %q", user.GetID(), want)
			}
		})
	}
}

// TestPasswordFileRefusesACredential covers the three ways a login fails. All
// three answer a nil user and no error, so nothing a caller can read says
// which, and a login page cannot leak which usernames exist.
func TestPasswordFileRefusesACredential(t *testing.T) {
	t.Parallel()

	store := writeFile(t, "admin:"+testHashes["bcrypt"])
	for _, test := range []struct{ name, key, secret string }{
		{"the wrong password", "admin", "hunter3hunter3"},
		{"an empty password", "admin", ""},
		{"a user that is not in the file", "nobody", testPassword},
		{"the username in another case", "Admin", testPassword},
		{"an empty username", "", testPassword},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			user, err := store.Authenticate(context.Background(), test.key, test.secret)
			if err != nil {
				t.Fatalf("Authenticate() = %v, want no error for a refusal", err)
			}
			if user != nil {
				t.Errorf("Authenticate(%q, %q) = %v, want no user", test.key, test.secret, user)
			}
		})
	}
}

// TestPasswordFileRefusesTheFile covers what construction will not read. Each
// one is a file an operator has to fix, so the message carries the line and
// the parse stops there. A file half of which authenticates hides the other
// half until it locks a user out.
func TestPasswordFileRefusesTheFile(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		lines []string
		want  string
	}{
		{
			name:  "a line with no separator",
			lines: []string{"admin"},
			want:  "line 1: no username:hash separator",
		},
		{
			name:  "a username that is not lowercase",
			lines: []string{"admin:" + testHashes["bcrypt"], "Reader:" + testHashes["bcrypt"]},
			want:  "line 2: username \"Reader\" is not lowercase",
		},
		{
			name:  "the same username twice",
			lines: []string{"admin:" + testHashes["bcrypt"], "admin:" + testHashes["apr1"]},
			want:  "line 2: \"admin\" is already in this file",
		},
		{
			name:  "an empty username",
			lines: []string{":" + testHashes["bcrypt"]},
			want:  "line 1: empty username",
		},
		{
			name:  "a plain text password",
			lines: []string{"admin:hunter2hunter2"},
			want:  "unreadable password hash",
		},
		{
			name:  "no hash at all",
			lines: []string{"admin:"},
			want:  "no password hash",
		},
		{
			name:  "php's md5-crypt in place of apache's",
			lines: []string{"admin:$1$abcdefgh$W3/Qmrm9IyJrlaEZhHu/p."},
			want:  "md5-crypt is php's $1$",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if err := refuseFile(t, test.lines...); !strings.Contains(err.Error(), test.want) {
				t.Errorf("refusal = %v, want a message carrying %q", err, test.want)
			}
		})
	}
}

// TestPasswordFileReadsAroundNothing covers the lines a file carries that are
// not users: apache's `#` comments, blank lines and trailing whitespace, which
// a hand-edited file accumulates.
func TestPasswordFileReadsAroundNothing(t *testing.T) {
	t.Parallel()

	store := writeFile(t,
		"# the operator's note",
		"",
		"   ",
		"  admin:"+testHashes["bcrypt"]+"  ",
		"#reader:"+testHashes["apr1"],
	)

	users, err := store.Users(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].String() != "admin" {
		t.Fatalf("Users() = %v, want admin alone", users)
	}
	user, err := store.Authenticate(context.Background(), "admin", testPassword)
	if err != nil || user == nil {
		t.Fatalf("Authenticate() = %v, %v", user, err)
	}
}

// TestPasswordFileUsers covers the listing a caller rebuilds a table from:
// ordered by username, one record each, and no hash anywhere in it.
func TestPasswordFileUsers(t *testing.T) {
	t.Parallel()

	store := writeFile(t,
		"reader:"+testHashes["apr1"],
		"admin:"+testHashes["bcrypt"],
		"zoe:"+testHashes["sha"],
	)

	users, err := store.Users(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 3 {
		t.Fatalf("Users() = %v, want three", users)
	}
	for i, want := range []string{"admin", "reader", "zoe"} {
		if users[i].String() != want {
			t.Errorf("Users()[%d] = %q, want %q", i, users[i].String(), want)
		}
		if users[i].GetID() != crypto.ULIDFrom(want) {
			t.Errorf("Users()[%d].GetID() = %q, want the id computed from %q", i, users[i].GetID(), want)
		}
	}
}

// TestPasswordFileExists covers the question a default store rests on. A
// file that is not there is no error, because a tree with no user database
// still has to construct one; a directory in its place is, because that is a
// path somebody got wrong.
func TestPasswordFileExists(t *testing.T) {
	t.Parallel()

	store := writeFile(t, "admin:"+testHashes["bcrypt"])
	exists, err := store.Exists()
	if err != nil || !exists {
		t.Errorf("Exists() = %v, %v, want true", exists, err)
	}

	dir := t.TempDir()
	absent, err := NewPasswordFile(filepath.Join(dir, "nothing-here"))
	if err != nil {
		t.Fatalf("NewPasswordFile() on a missing file = %v, want a store", err)
	}
	exists, err = absent.Exists()
	if err != nil || exists {
		t.Errorf("Exists() = %v, %v, want false and no error", exists, err)
	}

	// A missing file authenticates nobody, and says why: this is a
	// misconfiguration, where a refusal is a wrong password.
	if _, err := absent.Authenticate(context.Background(), "admin", testPassword); err == nil {
		t.Error("Authenticate() against a missing file = nil, want an error naming the file")
	}
	// The listing is empty and carries no error, so a caller rebuilding a
	// table writes no rows and its startup job still finishes.
	users, err := absent.Users(context.Background())
	if err != nil || len(users) != 0 {
		t.Errorf("Users() = %v, %v, want none and no error", users, err)
	}

	// The read the constructor runs refuses a directory in place of the
	// file, before anything asks Exists.
	if store, err := NewPasswordFile(dir); err == nil {
		t.Errorf("NewPasswordFile(%q) = %+v, want a refusal", dir, store)
	}
}

// TestPasswordFileExistsOnADirectory covers a path that turns into a directory
// after construction, the one way Exists meets one: the constructor's own read
// refuses it otherwise, so this is a tree that changed under a running server.
func TestPasswordFileExistsOnADirectory(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".htpasswd")
	if err := os.WriteFile(path, []byte("admin:"+testHashes["bcrypt"]+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewPasswordFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	exists, err := store.Exists()
	if err == nil {
		t.Error("Exists() on a directory = nil, want an error")
	}
	if exists {
		t.Error("Exists() on a directory = true")
	}
}

// TestPasswordFileEmptyPath covers the Go host's constructor refusing what it
// cannot resolve. A script never reaches it: an omitted path is the default
// file, which TestNewPasswordFileRoot covers.
func TestPasswordFileEmptyPath(t *testing.T) {
	t.Parallel()

	if store, err := NewPasswordFile(""); err == nil {
		t.Errorf("NewPasswordFile(\"\") = %+v, want an error", store)
	}
}

// TestNewPasswordFileRoot covers the rooted constructor as a Go caller reaches
// it, which stdlib/session does for the store `new Session\Manager` takes by
// default: an omitted path is DefaultPasswordFile in the root, and a named one
// resolves inside it.
//
// auth_test.go covers the confinement itself, through the registered class,
// where a script does the naming.
func TestNewPasswordFileRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, DefaultPasswordFile), []byte("admin:"+testHashes["bcrypt"]+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rt := runner.New(io.Discard, runner.Options{})
	store, err := NewPasswordFileRoot(rt, root)
	if err != nil {
		t.Fatalf("NewPasswordFileRoot(%q) = %v", root, err)
	}
	if exists, err := store.Exists(); err != nil || !exists {
		t.Fatalf("Exists() = %v, %v, want the default file", exists, err)
	}
	user, err := store.Authenticate(context.Background(), "admin", testPassword)
	if err != nil || user == nil {
		t.Fatalf("Authenticate() = %v, %v", user, err)
	}

	// A path the caller names resolves as named, and a tree with no file at
	// that path still constructs.
	named, err := NewPasswordFileRoot(rt, root, "config/users")
	if err != nil {
		t.Fatalf("NewPasswordFileRoot with a named path = %v", err)
	}
	if exists, err := named.Exists(); err != nil || exists {
		t.Errorf("Exists() = %v, %v, want false for a path with no file", exists, err)
	}
}

// TestPasswordFileRereadsTheFile is the property that makes a reload enough to
// add a user: the store caches nothing, so an edited file takes effect on the
// next call and nothing anywhere needs invalidating.
func TestPasswordFileRereadsTheFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".htpasswd")
	if err := os.WriteFile(path, []byte("admin:"+testHashes["bcrypt"]+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewPasswordFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if user, _ := store.Authenticate(context.Background(), "reader", testPassword); user != nil {
		t.Fatal("a user the file does not carry authenticated")
	}

	appended := "admin:" + testHashes["bcrypt"] + "\nreader:" + testHashes["apr1"] + "\n"
	if err := os.WriteFile(path, []byte(appended), 0o600); err != nil {
		t.Fatal(err)
	}

	user, err := store.Authenticate(context.Background(), "reader", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if user == nil {
		t.Error("a user added to the file did not authenticate, so the parse is cached")
	}
}

// TestPasswordFileCancelledContext covers what a read owes the request it
// runs under: a cancelled context ends the call and leaves the filesystem
// alone.
func TestPasswordFileCancelledContext(t *testing.T) {
	t.Parallel()

	store := writeFile(t, "admin:"+testHashes["bcrypt"])
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.Authenticate(ctx, "admin", testPassword); err == nil {
		t.Error("Authenticate() on a cancelled context = nil, want the context error")
	}
	if _, err := store.Users(ctx); err == nil {
		t.Error("Users() on a cancelled context = nil, want the context error")
	}
}
