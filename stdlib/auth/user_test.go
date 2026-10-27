package auth

import (
	"fmt"
	"testing"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/stdlib/crypto"
)

// TestUser covers the record's two methods and the property the primary key
// rests on: the id follows from the username and one name answers one key, for
// as long as the name exists and again after a second line adds it back.
func TestUser(t *testing.T) {
	t.Parallel()

	admin := newUser("admin")
	if admin.String() != "admin" {
		t.Errorf("String() = %q, want the username", admin.String())
	}
	if want := crypto.ULIDFrom("admin"); admin.GetID() != want {
		t.Errorf("GetID() = %q, want %q", admin.GetID(), want)
	}
	if again := newUser("admin"); again.GetID() != admin.GetID() {
		t.Errorf("a second record for admin has id %q, want %q", again.GetID(), admin.GetID())
	}
	if other := newUser("reader"); other.GetID() == admin.GetID() {
		t.Errorf("admin and reader share the id %q", other.GetID())
	}
}

// TestUserRendersAsItsUsername is the reason the second method is String and
// no GetUsername: phpval.GoString takes a fmt.Stringer and `echo $user` and a
// Go binding's string parameter both read the username with nothing spelled at
// the call site. A record that dropped Stringer would print the empty string,
// and no other test here would catch it.
func TestUserRendersAsItsUsername(t *testing.T) {
	t.Parallel()

	var record User = newUser("admin")
	if _, ok := record.(fmt.Stringer); !ok {
		t.Fatal("a user is not a fmt.Stringer, so a string context renders nothing")
	}
	if got, ok := phpval.GoString(record); !ok || got != "admin" {
		t.Errorf("phpval.GoString(user) = %q, %v, want \"admin\"", got, ok)
	}
	if got := phpval.String(record); got != "admin" {
		t.Errorf("phpval.String(user) = %q, want \"admin\"", got)
	}
}
