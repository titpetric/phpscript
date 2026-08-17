package ps

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
)

const sessionCookieName = "session"

// SessionManager associates an HTTP-only cookie with data in SessionStorage.
// The cookie contains only an opaque, randomly generated session ID.
type SessionManager struct {
	// SessionCookieName is the mutable name used to read and write the session
	// cookie. PHP may change it with `$session->SessionCookieName = "sid"`.
	SessionCookieName string

	storage   SessionStorage
	sessionID string
}

// NewSessionManager creates a manager backed by storage.
func NewSessionManager(storage SessionStorage) (*SessionManager, error) {
	if storage == nil {
		return nil, errors.New("session storage is nil")
	}
	return &SessionManager{
		SessionCookieName: sessionCookieName,
		storage:           traceSessionStorage(storage),
	}, nil
}

// Start creates a new session, stores userID, and stages its HTTP-only cookie.
func (s *SessionManager) Start(ctx context.Context, userID any) error {
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
func (s *SessionManager) Get(ctx context.Context) (string, error) {
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
func (s *SessionManager) Valid(ctx context.Context) (bool, error) {
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

func (s *SessionManager) currentID(ctx context.Context) (string, bool) {
	if s.sessionID != "" {
		return s.sessionID, true
	}
	request, ok := runner.RequestContext(ctx)
	if !ok {
		return "", false
	}
	for name, value := range request.Headers {
		if strings.EqualFold(name, "Authorize") && strings.TrimSpace(value) != "" {
			return validSessionID(strings.TrimSpace(value))
		}
	}
	return validSessionID(request.Cookie[s.SessionCookieName])
}

func validSessionID(id string) (string, bool) {
	if len(id) != 64 || strings.ToLower(id) != id {
		return "", false
	}
	decoded, err := hex.DecodeString(id)
	return id, err == nil && len(decoded) == 32
}
