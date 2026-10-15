package crypto

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/runner"
)

// TestInitContributesEveryInstaller covers the wiring init.go does. Each of
// the four installers is contributed through runner.RegisterBinding, so
// importing this package is what puts password hashing, the CSPRNG, the
// message digests and the id generators into a binary, and stdlib.Register is
// what puts them on a runtime. An installer left out of init() leaves its
// names undefined with nothing else failing, so one name per installer is
// named here. This test binary imports only this package, so the contributed
// set is this package's alone.
func TestInitContributesEveryInstaller(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{})
	for _, install := range runner.BindingsFor(runner.ProfileSecure) {
		install(rt)
	}

	for _, test := range []struct {
		installer string
		name      string
	}{
		{installer: "Register", name: "password_hash"},
		{installer: "RegisterRandom", name: "random_bytes"},
		{installer: "RegisterHash", name: "hash"},
		{installer: "RegisterIdentifiers", name: "ulid"},
	} {
		t.Run(test.installer, func(t *testing.T) {
			if _, ok := rt.LookupFunc(test.name); !ok {
				t.Errorf("%s() is undefined, so %s was not contributed", test.name, test.installer)
			}
		})
	}
}

// TestRegisterIdentifiers covers the two generators PHP has no name for. Both
// render one layout, a 48-bit millisecond timestamp followed by randomness, so
// ids minted in order sort in order as text. The widths are the formats: 26
// Crockford base32 characters for a ULID, and 36 characters in the 8-4-4-4-12
// form for a UUIDv7, with the version and variant bits set.
func TestRegisterIdentifiers(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{})
	RegisterIdentifiers(rt)

	first, err := rt.InvokeNamed("ulid")
	if err != nil {
		t.Fatalf("ulid(): %v", err)
	}
	ulid, ok := first.(string)
	if !ok {
		t.Fatalf("ulid() returned %T, want a string", first)
	}
	if len(ulid) != 26 {
		t.Errorf("ulid() = %q, %d characters, want 26", ulid, len(ulid))
	}
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	for i := range ulid {
		if !strings.ContainsRune(alphabet, rune(ulid[i])) {
			t.Fatalf("ulid() = %q holds %q, which is not Crockford base32", ulid, ulid[i])
		}
	}

	second, err := rt.InvokeNamed("uuid")
	if err != nil {
		t.Fatalf("uuid(): %v", err)
	}
	uuid, ok := second.(string)
	if !ok {
		t.Fatalf("uuid() returned %T, want a string", second)
	}
	if len(uuid) != 36 {
		t.Errorf("uuid() = %q, %d characters, want 36", uuid, len(uuid))
	}
	for _, at := range []int{8, 13, 18, 23} {
		if uuid[at] != '-' {
			t.Errorf("uuid() = %q, want a dash at %d", uuid, at)
		}
	}
	if uuid[14] != '7' {
		t.Errorf("uuid() = %q, want version 7 in the third group", uuid)
	}
	if !strings.ContainsRune("89ab", rune(uuid[19])) {
		t.Errorf("uuid() = %q, want the RFC 4122 variant bits in the fourth group", uuid)
	}

	// The timestamp is in the leading bits, so a later id sorts after an
	// earlier one as text. A millisecond apart is enough to separate them.
	time.Sleep(2 * time.Millisecond)
	later, err := rt.InvokeNamed("ulid")
	if err != nil {
		t.Fatal(err)
	}
	if later.(string) <= ulid {
		t.Errorf("ulid() = %q after %q, want ids to sort by creation time", later, ulid)
	}
}
