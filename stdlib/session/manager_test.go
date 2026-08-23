package session_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/session"
)

func sessionContext(t *testing.T, cookies map[string]string) (context.Context, runner.Context) {
	t.Helper()
	rt := runner.New(nil, runner.Options{})
	request := runner.NewContext()
	request.Cookie = cookies
	request.Register(rt)
	return rt.Context(), request
}

func TestSessionManagerLifecycle(t *testing.T) {
	storage := session.NewStorageMemory()
	manager, err := session.NewManager(storage)
	if err != nil {
		t.Fatal(err)
	}
	if manager.SessionCookieName != "session" {
		t.Fatalf("SessionCookieName = %q, want session", manager.SessionCookieName)
	}
	manager.SessionCookieName = "sid"
	ctx, request := sessionContext(t, nil)

	valid, err := manager.Valid(ctx)
	if err != nil || valid {
		t.Fatalf("Valid without cookie = %v, %v; want false, nil", valid, err)
	}
	if err := manager.Start(ctx, "user-42"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got, err := manager.Get(ctx); err != nil || got != "user-42" {
		t.Fatalf("Get after Start = %q, %v; want user-42, nil", got, err)
	}
	if valid, err := manager.Valid(ctx); err != nil || !valid {
		t.Fatalf("Valid after Start = %v, %v; want true, nil", valid, err)
	}

	setCookies := request.ResponseHeaders().Values("Set-Cookie")
	if len(setCookies) != 1 {
		t.Fatalf("Set-Cookie count = %d, want 1", len(setCookies))
	}
	response := &http.Response{Header: http.Header{"Set-Cookie": setCookies}}
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("parsed cookie count = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "sid" || !cookie.HttpOnly || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie = %#v; want sid, HttpOnly, Path=/, SameSite=Lax", cookie)
	}

	next, err := session.NewManager(storage)
	if err != nil {
		t.Fatal(err)
	}
	next.SessionCookieName = "sid"
	nextCtx, _ := sessionContext(t, map[string]string{"sid": cookie.Value})
	if got, err := next.Get(nextCtx); err != nil || got != "user-42" {
		t.Fatalf("Get from cookie = %q, %v; want user-42, nil", got, err)
	}
}

func TestSessionManagerUsesAuthorizeHeader(t *testing.T) {
	storage := session.NewStorageMemory()
	id := strings.Repeat("a", 64)
	if err := storage.Save(t.Context(), id, []byte("header-user")); err != nil {
		t.Fatal(err)
	}
	manager, err := session.NewManager(storage)
	if err != nil {
		t.Fatal(err)
	}
	ctx, request := sessionContext(t, map[string]string{"session": "malformed"})
	request.Headers["authorize"] = "  " + id + "  "
	if valid, err := manager.Valid(ctx); err != nil || !valid {
		t.Fatalf("Valid with Authorize header = %v, %v; want true, nil", valid, err)
	}
	if got, err := manager.Get(ctx); err != nil || got != "header-user" {
		t.Fatalf("Get with Authorize header = %q, %v; want header-user, nil", got, err)
	}
}

func TestSessionManagerRejectsInvalidCookies(t *testing.T) {
	storage := session.NewStorageMemory()
	for name, value := range map[string]string{
		"missing":   "",
		"short":     "abc123",
		"non-hex":   strings.Repeat("z", 64),
		"uppercase": strings.Repeat("A", 64),
		"unknown":   strings.Repeat("a", 64),
	} {
		t.Run(name, func(t *testing.T) {
			manager, err := session.NewManager(storage)
			if err != nil {
				t.Fatal(err)
			}
			ctx, _ := sessionContext(t, map[string]string{"session": value})
			valid, err := manager.Valid(ctx)
			if err != nil || valid {
				t.Fatalf("Valid = %v, %v; want false, nil", valid, err)
			}
		})
	}
}

func TestNewSessionManagerValidatesConfiguration(t *testing.T) {
	if _, err := session.NewManager(nil); err == nil {
		t.Fatal("nil storage was accepted")
	}
}
