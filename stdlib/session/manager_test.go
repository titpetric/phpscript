package session_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/auth"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestSessionManagerReadsAServedRequestsCookie is the manager against the
// context a server builds, and not one a host filled in.
//
// A request decodes its cookies on the first read through Context.CookieMap, so
// the Cookie field is empty until something asks. Reading the field instead made
// every served request anonymous - valid() answered false for a cookie the same
// manager had issued one request earlier - while $_COOKIE held it, because the
// superglobal resolves through the accessor. Issue #118.
func TestSessionManagerReadsAServedRequestsCookie(t *testing.T) {
	t.Parallel()
	storage := session.NewStorageMemory()
	manager, err := session.NewManager(storage)
	if err != nil {
		t.Fatal(err)
	}

	// The first request has no cookie and starts a session.
	first, firstRequest := servedContext(t, "")
	if err := manager.Start(first, "row-1"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	staged := &http.Response{Header: http.Header{
		"Set-Cookie": firstRequest.ResponseHeaders().Values("Set-Cookie"),
	}}
	issued := staged.Cookies()
	if len(issued) != 1 {
		t.Fatalf("Set-Cookie count = %d, want 1", len(issued))
	}

	// The second request carries it, and a manager of its own answers it,
	// because a served request runs on a runtime that holds nothing from the
	// last one.
	next, err := session.NewManager(storage)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := servedContext(t, issued[0].Value)
	if valid, err := next.Valid(second); err != nil || !valid {
		t.Fatalf("Valid with the issued cookie = %v, %v; want true, nil", valid, err)
	}
	if got, err := next.Get(second); err != nil || got != "row-1" {
		t.Fatalf("Get with the issued cookie = %q, %v; want row-1, nil", got, err)
	}
}

// servedContext builds the request context a served request has: decoded from an
// *http.Request, with nothing read off it yet.
func servedContext(t *testing.T, cookie string) (context.Context, runner.Context) {
	t.Helper()
	rt := runner.New(nil, runner.Options{})
	r := httptest.NewRequest(http.MethodGet, "/probe", nil)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: "session", Value: cookie})
	}
	request := runner.FromRequest(r)
	request.Register(rt)
	return rt.Context(), request
}

func TestNewSessionManagerValidatesConfiguration(t *testing.T) {
	t.Parallel()
	if _, err := session.NewManager(nil); err == nil {
		t.Fatal("nil storage was accepted")
	}
}

// users is a user database of one account, standing in for the password file.
// The manager makes one call per login and no other, so these two methods are
// the whole interface.
type users struct {
	username string
	secret   string
	err      error
}

func (u users) Authenticate(_ context.Context, key, secret string) (auth.User, error) {
	if u.err != nil {
		return nil, u.err
	}
	if key != u.username || secret != u.secret {
		return nil, nil
	}
	return account{name: key}, nil
}

func (u users) Exists() (bool, error) { return u.err == nil, u.err }

// account is the record the stub answers with: an id that is not derived from
// anything, so a test can assert on the exact value the session carries.
type account struct {
	name string
}

func (a account) GetID() string  { return "01J9F3K2PQUSERKEY0000000AA" }
func (a account) String() string { return a.name }

// TestSessionManagerLogin covers what a login puts in the session: the user's
// primary key, where the username is what the credential carried. A row
// written during the session references the user as any other table does.
func TestSessionManagerLogin(t *testing.T) {
	t.Parallel()
	manager, err := session.NewManager(session.NewStorageMemory(), users{username: "admin", secret: "hunter2hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, request := sessionContext(t, nil)

	ok, err := manager.Login(ctx, "admin", "hunter2hunter2")
	if err != nil || !ok {
		t.Fatalf("Login = %v, %v; want true, nil", ok, err)
	}
	if got, err := manager.Get(ctx); err != nil || got != "01J9F3K2PQUSERKEY0000000AA" {
		t.Fatalf("Get after Login = %q, %v; want the user's id", got, err)
	}
	if valid, err := manager.Valid(ctx); err != nil || !valid {
		t.Fatalf("Valid after Login = %v, %v; want true, nil", valid, err)
	}
	// A login is a session start, so it stages the cookie with the same
	// attributes Start writes and the script spells none of them.
	if staged := request.ResponseHeaders().Values("Set-Cookie"); len(staged) != 1 {
		t.Fatalf("Set-Cookie count = %d, want 1", len(staged))
	}
}

// TestSessionManagerLoginRefused covers the two refusals a login distinguishes
// from each other: a credential that misses starts no session and raises no
// error, and a store nothing could read raises the error in place of false.
func TestSessionManagerLoginRefused(t *testing.T) {
	t.Parallel()
	store := users{username: "admin", secret: "hunter2hunter2"}

	for _, test := range []struct{ name, key, secret string }{
		{name: "the wrong password", key: "admin", secret: "wrong"},
		{name: "an unknown user", key: "nobody", secret: "hunter2hunter2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			manager, err := session.NewManager(session.NewStorageMemory(), store)
			if err != nil {
				t.Fatal(err)
			}
			ctx, request := sessionContext(t, nil)

			ok, err := manager.Login(ctx, test.key, test.secret)
			if err != nil || ok {
				t.Fatalf("Login = %v, %v; want false, nil", ok, err)
			}
			if valid, _ := manager.Valid(ctx); valid {
				t.Error("a refused login left a valid session")
			}
			if staged := request.ResponseHeaders().Values("Set-Cookie"); len(staged) != 0 {
				t.Errorf("a refused login staged %v", staged)
			}
		})
	}

	broken, err := session.NewManager(session.NewStorageMemory(), users{err: errors.New("no such file")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, _ := sessionContext(t, nil)
	if ok, err := broken.Login(ctx, "admin", "hunter2hunter2"); ok || err == nil {
		t.Errorf("Login against an unreadable store = %v, %v; want false and the error", ok, err)
	}
}

// TestSessionManagerLoginWithoutAUserDatabase covers a manager a Go host built
// with storage alone. There is nothing to put the credential to, and the
// message says that; reporting a wrong password would name the wrong fault.
func TestSessionManagerLoginWithoutAUserDatabase(t *testing.T) {
	t.Parallel()
	manager, err := session.NewManager(session.NewStorageMemory())
	if err != nil {
		t.Fatal(err)
	}
	ctx, _ := sessionContext(t, nil)

	if ok, err := manager.Login(ctx, "admin", "hunter2hunter2"); ok || err == nil {
		t.Fatalf("Login = %v, %v; want false and an error", ok, err)
	}
}

// TestSessionManagerDestroy covers the logout: the storage drops the session,
// the response expires the cookie, and a manager reading that same cookie
// afterwards answers anonymous.
func TestSessionManagerDestroy(t *testing.T) {
	t.Parallel()
	storage := session.NewStorageMemory()
	manager, err := session.NewManager(storage)
	if err != nil {
		t.Fatal(err)
	}

	ctx, request := servedContext(t, "")
	if err := manager.Start(ctx, "user-42"); err != nil {
		t.Fatal(err)
	}
	issued := (&http.Response{Header: http.Header{
		"Set-Cookie": request.ResponseHeaders().Values("Set-Cookie"),
	}}).Cookies()
	if len(issued) != 1 {
		t.Fatalf("Set-Cookie count = %d, want 1", len(issued))
	}

	if err := manager.Destroy(ctx); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if valid, err := manager.Valid(ctx); err != nil || valid {
		t.Fatalf("Valid after Destroy = %v, %v; want false, nil", valid, err)
	}

	// The cookie the response carries now is the deletion: same name and
	// path, so a client applies it to the one it holds, with no expiry left.
	staged := (&http.Response{Header: http.Header{
		"Set-Cookie": request.ResponseHeaders().Values("Set-Cookie"),
	}}).Cookies()
	if len(staged) != 2 {
		t.Fatalf("Set-Cookie count = %d, want the session and its deletion", len(staged))
	}
	deletion := staged[1]
	if deletion.Name != "session" || deletion.Value != "" || deletion.MaxAge >= 0 || deletion.Path != "/" {
		t.Errorf("deletion cookie = %#v", deletion)
	}

	// The storage dropped the session, and not only this manager, so the
	// cookie a client kept is worth nothing on the next request.
	next, err := session.NewManager(storage)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := servedContext(t, issued[0].Value)
	if valid, err := next.Valid(after); err != nil || valid {
		t.Fatalf("Valid with a destroyed session = %v, %v; want false, nil", valid, err)
	}
}

// TestSessionManagerDestroyWithoutASession covers logging out twice, and
// logging out of a session a prune has already taken. Both end with no
// session, which is the request the client made, and both return nil.
func TestSessionManagerDestroyWithoutASession(t *testing.T) {
	t.Parallel()
	manager, err := session.NewManager(session.NewStorageMemory())
	if err != nil {
		t.Fatal(err)
	}

	ctx, _ := servedContext(t, "")
	if err := manager.Destroy(ctx); err != nil {
		t.Fatalf("Destroy without a session: %v", err)
	}
	if err := manager.Start(ctx, "user-42"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Destroy(ctx); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if err := manager.Destroy(ctx); err != nil {
		t.Fatalf("Destroy twice: %v", err)
	}
}

// TestSessionManagerSetters covers the other half of the constructor: the
// setters point a manager built with the defaults at something else, and
// neither takes a nil.
func TestSessionManagerSetters(t *testing.T) {
	t.Parallel()
	manager, err := session.NewManager(session.NewStorageMemory())
	if err != nil {
		t.Fatal(err)
	}

	storage := session.NewStorageMemory()
	if err := manager.SetStorage(storage); err != nil {
		t.Fatalf("SetStorage: %v", err)
	}
	if err := manager.SetAuth(users{username: "admin", secret: "hunter2hunter2"}); err != nil {
		t.Fatalf("SetAuth: %v", err)
	}

	ctx, _ := sessionContext(t, nil)
	if ok, err := manager.Login(ctx, "admin", "hunter2hunter2"); err != nil || !ok {
		t.Fatalf("Login after SetAuth = %v, %v; want true, nil", ok, err)
	}
	// The session landed in the storage the setter named, which the
	// constructor's own storage would fail.
	id, err := manager.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Load(ctx, sessionIDOf(t, ctx, manager)); err != nil {
		t.Fatalf("the session is not in the storage SetStorage named: %v (id %q)", err, id)
	}

	if err := manager.SetStorage(nil); err == nil {
		t.Error("SetStorage(nil) was accepted")
	}
	if err := manager.SetAuth(nil); err == nil {
		t.Error("SetAuth(nil) was accepted")
	}
}

// sessionIDOf reads back the id the manager holds, the one it staged in the
// cookie. Nothing exposes that id, the credential itself, so a test needing to
// look in storage reads it off the response the way a client would.
func sessionIDOf(t *testing.T, ctx context.Context, manager *session.Manager) string {
	t.Helper()
	request, ok := runner.RequestContext(ctx)
	if !ok {
		t.Fatal("no request context")
	}
	staged := (&http.Response{Header: http.Header{
		"Set-Cookie": request.ResponseHeaders().Values("Set-Cookie"),
	}}).Cookies()
	if len(staged) == 0 {
		t.Fatal("no session cookie was staged")
	}
	return staged[len(staged)-1].Value
}
