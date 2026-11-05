package auth

import (
	"strings"
	"testing"
)

// The stored values every test in this package authenticates against, all for
// the one password below. Each comes from the tool that owns its format and a
// row that stops verifying is a compatibility break and no test to update:
//
//	php -r 'echo password_hash("hunter2hunter2", PASSWORD_BCRYPT, ["cost" => 4]);'
//	php -r 'echo password_hash("hunter2hunter2", PASSWORD_ARGON2ID, ["memory_cost" => 8, "time_cost" => 1, "threads" => 1]);'
//	php -r 'echo "{SHA}" . base64_encode(sha1("hunter2hunter2", true));'
//	openssl passwd -apr1 -salt Zx1kLm9q hunter2hunter2
//
// The work factors are the lowest each format accepts, because what is under
// test is the dispatch and not the derivation. A file in production carries
// bcrypt at a cost docs/reference/extensions/hashing.md prices.
const testPassword = "hunter2hunter2"

var testHashes = map[string]string{
	"bcrypt":   "$2y$04$j1zOEFHUWgLb8D1SytMeoeFw/s1.z5eOLow3m0r25ypMopsjEbQGW",
	"argon2id": "$argon2id$v=19$m=8,t=1,p=1$cy9CN0dXU2gvYnVVemZmRw$bJcfAH11Sej7T96VdH4lBTbzdCH4FhajmuGb8ZYGRh0",
	"argon2i":  "$argon2i$v=19$m=8,t=1,p=1$d0ZBSS9jeXNtWk9pTnpPUg$Y0SmMSfibxyy3Zgf6r/qRA6/5I9TS5Uua0L8FcKDig0",
	"apr1":     "$apr1$Zx1kLm9q$BwLItAYS8lPw/bCsIXHax1",
	"sha":      "{SHA}/IxesZSAbjGiE/BzEx5zsAEqD7U=",
}

// unreadableHashes are the values a line may not carry. Four of them are real
// formats this build will not read, and the rest are what a damaged file looks
// like; they are one list because the refusal is one refusal, and a plain-text
// password is not distinguishable from a truncated hash.
var unreadableHashes = map[string]string{
	"plain text":       "hunter2hunter2",
	"empty":            "",
	"crypt des":        "abFjERrjIDnVE",
	"php md5 crypt":    "$1$abcdefgh$W3/Qmrm9IyJrlaEZhHu/p.",
	"truncated bcrypt": "$2y$04$j1zOEFHUWgLb8D1SytMeoe",
	// The 1997 spelling with no minor revision, then a revision php's crypt
	// never wrote. php -r answers bool(false) for both through
	// password_verify and a line carrying either would verify nowhere.
	"bcrypt no revision":  "$2$04$j1zOEFHUWgLb8D1SytMeoeFw/s1.z5eOLow3m0r25ypMopsjEbQGW",
	"bcrypt bad revision": "$2c$04$j1zOEFHUWgLb8D1SytMeoeFw/s1.z5eOLow3m0r25ypMopsjEbQGW",
	"bcrypt bad base64":   "$2y$04$j1zOEFHUWgLb8D1SytMeoeFw/s1.z5eOLow3m0r25ypMopsjEb!!!",
	"argon2 no version":   "$argon2id$m=8,t=1,p=1$cy9CN0dXU2gvYnVVemZmRw$bJcfAH11Sej7T96VdH4lBTbzdCH4FhajmuGb8ZYGRh0",
	"sha not base64":      "{SHA}not base64 at all",
	"sha wrong width":     "{SHA}YWJj",
}

func TestVerify(t *testing.T) {
	t.Parallel()

	for name, stored := range testHashes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if !verify(testPassword, stored) {
				t.Errorf("verify(%q, %q) = false, want true", testPassword, stored)
			}
			if verify("wrong password", stored) {
				t.Errorf("verify accepted a wrong password against %q", stored)
			}
			if !readable(stored) {
				t.Errorf("readable(%q) = false, so a file carrying it would be refused", stored)
			}
		})
	}
}

// TestVerifyRefusesUnreadable covers the fallthrough. A file this package
// parsed cannot reach it, because the parse refuses the same list and what is
// under test is that a store assembled some other way authenticates nobody.
func TestVerifyRefusesUnreadable(t *testing.T) {
	t.Parallel()

	for name, stored := range unreadableHashes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if readable(stored) {
				t.Errorf("readable(%q) = true, want false", stored)
			}
			if verify(testPassword, stored) {
				t.Errorf("verify(%q, %q) = true, want false", testPassword, stored)
			}
			if formatError(stored) == nil {
				t.Errorf("formatError(%q) = nil, so the refusal names nothing", stored)
			}
		})
	}
}

// TestFormatErrorNamesTheRemedy covers the two messages an operator acts on.
// php's `$1$` is one character away from apache's `$apr1$` and is the mistake
// worth a sentence of its own.
func TestFormatErrorNamesTheRemedy(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		stored string
		want   string
	}{
		{stored: "$1$abcdefgh$W3/Qmrm9IyJrlaEZhHu/p.", want: "md5-crypt is php's $1$"},
		{stored: "", want: "no password hash"},
		{stored: "hunter2hunter2", want: "unreadable password hash"},
	} {
		if err := formatError(test.stored); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("formatError(%q) = %v, want a message carrying %q", test.stored, err, test.want)
		}
	}
}

func TestIsSHA(t *testing.T) {
	t.Parallel()

	if !isSHA(testHashes["sha"]) {
		t.Errorf("isSHA(%q) = false", testHashes["sha"])
	}
	// The prefix alone is not the format: a line nothing can verify would
	// otherwise claim it and then refuse every password.
	for _, stored := range []string{"{SHA}", "{SHA}YWJj", "{SHA}not base64", testHashes["bcrypt"]} {
		if isSHA(stored) {
			t.Errorf("isSHA(%q) = true, want false", stored)
		}
	}
}
