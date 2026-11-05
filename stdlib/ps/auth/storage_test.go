package auth

import (
	"context"
	"errors"
	"testing"
)

// stubStorage is a store of one user with no derivation, standing in for the
// driver a host writes against a table of its own. It proves a driver can skip
// this package's file, and the interface is the whole surface Session\Manager
// calls.
type stubStorage struct {
	username string
	secret   string
	absent   bool
	broken   error
}

func (s stubStorage) Authenticate(_ context.Context, key, secret string) (User, error) {
	if s.broken != nil {
		return nil, s.broken
	}
	if key != s.username || secret != s.secret {
		return nil, nil
	}
	return newUser(key), nil
}

func (s stubStorage) Exists() (bool, error) {
	if s.broken != nil {
		return false, s.broken
	}
	return !s.absent, nil
}

var _ Storage = stubStorage{}

// TestStorageContract covers the three answers the interface distinguishes,
// because a caller branches on them: a user, a refusal that is not an error,
// and an error that is not a refusal.
func TestStorageContract(t *testing.T) {
	t.Parallel()

	store := stubStorage{username: "admin", secret: testPassword}

	user, err := store.Authenticate(context.Background(), "admin", testPassword)
	if err != nil || user == nil {
		t.Fatalf("Authenticate() = %v, %v, want the user", user, err)
	}
	if user.String() != "admin" {
		t.Errorf("String() = %q", user.String())
	}

	user, err = store.Authenticate(context.Background(), "admin", "wrong")
	if err != nil {
		t.Errorf("a wrong password returned the error %v, want a nil user", err)
	}
	if user != nil {
		t.Errorf("a wrong password returned %v, want no user", user)
	}

	unreadable := stubStorage{broken: errors.New("no such store")}
	if _, err := unreadable.Authenticate(context.Background(), "admin", testPassword); err == nil {
		t.Error("an unreadable store returned no error")
	}
	if exists, err := unreadable.Exists(); exists || err == nil {
		t.Errorf("Exists() = %v, %v, want false and the error", exists, err)
	}
}

// TestPasswordFileImplementsStorage is the assertion that matters at the
// seam: the registered class is what Session\Manager accepts.
func TestPasswordFileImplementsStorage(t *testing.T) {
	t.Parallel()

	var store Storage = writeFile(t, "admin:"+testHashes["bcrypt"])
	if exists, err := store.Exists(); err != nil || !exists {
		t.Errorf("Exists() = %v, %v", exists, err)
	}
}
