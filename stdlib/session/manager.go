package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/auth"
)

const defaultCookieName = "session"

// Manager associates an HTTP-only cookie with data in Storage.
// The cookie contains only an opaque, randomly generated session ID.
type Manager struct {
	// SessionCookieName is the mutable name this package reads and writes the session
	// cookie. PHP may change it with `$session->SessionCookieName = "sid"`.
	SessionCookieName string

	storage   Storage
	users     auth.Storage
	sessionID string
}

// NewManager creates a manager backed by storage, authenticating against the
// user database given.
//
// The user database is variadic because most of what a manager does needs
// none: a session is a cookie and a storage, and login() is the one method
// that asks who a credential belongs to. The registered constructor fills both
// with defaults when a script names neither; see session.go.
func NewManager(storage Storage, users ...auth.Storage) (*Manager, error) {
	if storage == nil {
		return nil, errors.New("session storage is nil")
	}
	manager := &Manager{
		SessionCookieName: defaultCookieName,
		storage:           traceStorage(storage),
	}
	if len(users) > 0 {
		manager.users = users[0]
	}
	return manager, nil
}

// SetStorage replaces the storage sessions live in.
//
// This is the setter half of the constructor: a script that took the
// manager's defaults and then needs memory storage says so here, and the rest
// of the manager holds. It wraps the storage for tracing as the constructor
// does, so the trace names whichever backend answered.
func (s *Manager) SetStorage(storage Storage) error {
	if storage == nil {
		return errors.New("session storage is nil")
	}
	s.storage = traceStorage(storage)
	return nil
}

// SetAuth replaces the user database login() authenticates against.
func (s *Manager) SetAuth(users auth.Storage) error {
	if users == nil {
		return errors.New("user database is nil")
	}
	s.users = users
	return nil
}

// Login authenticates $key and $secret against the user database and starts a
// session for the user it names. It reports whether the credential held; a
// wrong password is false and no error.
//
// The session carries the user's primary key, so a row written during the
// session references the user the way any other table does. The username
// stays in the user database, and a caller that needs it reads it there.
func (s *Manager) Login(ctx context.Context, key, secret string) (bool, error) {
	if s.users == nil {
		return false, errors.New("session manager has no user database to authenticate against")
	}

	user, err := s.users.Authenticate(ctx, key, secret)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, nil
	}
	if err := s.Start(ctx, user.GetID()); err != nil {
		return false, err
	}
	return true, nil
}

// Destroy ends the current session: it drops the data behind the cookie and
// expires the cookie on the way out.
//
// A session that is not there is no error. Logging out twice, and logging out
// of a session a prune already took, are the same request to the client, and
// both end with no session and no cookie.
func (s *Manager) Destroy(ctx context.Context) error {
	id, ok := s.currentID(ctx)
	s.sessionID = ""
	if !ok {
		return nil
	}

	if err := s.storage.Delete(ctx, id); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	request, ok := runner.RequestContext(ctx)
	if !ok {
		return nil
	}
	// MaxAge below zero is net/http's spelling of "delete this now", which it
	// writes as Max-Age=0. The rest of the attributes match what Start wrote,
	// because a client applies the deletion to the cookie it matches.
	request.AddResponseHeader("Set-Cookie", (&http.Cookie{
		Name:     s.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}).String())
	return nil
}

// DeleteByUser signs one user out of every client, answering how many sessions
// it took. It clears this manager's own session when the sweep took it, so
// valid() answers false in the same request the account went away in.
func (s *Manager) DeleteByUser(ctx context.Context, userID string) (int, error) {
	if current, err := s.Get(ctx); err == nil && current == userID {
		s.sessionID = ""
	}
	return s.storage.DeleteByUser(ctx, userID)
}

// Start creates a new session, stores userID, and stages its HTTP-only cookie.
func (s *Manager) Start(ctx context.Context, userID any) error {
	request, ok := runner.RequestContext(ctx)
	if !ok {
		return errors.New("session manager requires an HTTP request context")
	}

	idBytes := make([]byte, 32)
	if _, err := rand.Read(idBytes); err != nil {
		return fmt.Errorf("create session ID: %w", err)
	}
	id := hex.EncodeToString(idBytes)
	if err := s.storage.Save(ctx, id, []byte(fmt.Sprint(userID))); err != nil {
		return err
	}

	s.sessionID = id
	request.AddResponseHeader("Set-Cookie", (&http.Cookie{
		Name:     s.SessionCookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}).String())
	return nil
}

// Get returns the user ID stored for the current session cookie.
func (s *Manager) Get(ctx context.Context) (string, error) {
	id, ok := s.currentID(ctx)
	if !ok {
		return "", errors.New("invalid or missing session cookie")
	}
	data, err := s.storage.Load(ctx, id)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Valid reports whether the request has a well-formed cookie whose session is
// present in storage. Missing and malformed cookies are invalid, not errors.
func (s *Manager) Valid(ctx context.Context) (bool, error) {
	id, ok := s.currentID(ctx)
	if !ok {
		return false, nil
	}
	if _, err := s.storage.Load(ctx, id); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Manager) currentID(ctx context.Context) (string, bool) {
	if s.sessionID != "" {
		return s.sessionID, true
	}
	request, ok := runner.RequestContext(ctx)
	if !ok {
		return "", false
	}
	for name, value := range request.HeaderMap() {
		if strings.EqualFold(name, "Authorize") && strings.TrimSpace(value) != "" {
			return validSessionID(strings.TrimSpace(value))
		}
	}
	// CookieMap and not the Cookie field: a request decodes its cookies on the
	// first read through the accessor, so the field is empty until something
	// asks. The same call covers a host that fills the field itself.
	return validSessionID(request.CookieMap()[s.SessionCookieName])
}

func validSessionID(id string) (string, bool) {
	if len(id) != 64 || strings.ToLower(id) != id {
		return "", false
	}
	decoded, err := hex.DecodeString(id)
	return id, err == nil && len(decoded) == 32
}
