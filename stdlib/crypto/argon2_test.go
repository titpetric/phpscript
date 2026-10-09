package crypto

import (
	"strings"
	"testing"
)

// TestArgon2VerifyPHPVectors is the interop check the whole encoding exists
// for: every hash below was written by php 8.5's password_hash(), pasted, and
// has to verify here. The awkward parameter sets are chosen: m=1025 with
// p=3 is not a multiple of 4p, which argon2 rounds down for the lanes while
// keeping the original value in the initial hash, and m=8 is the smallest php
// accepts.
func TestArgon2VerifyPHPVectors(t *testing.T) {
	cases := []struct {
		password string
		hash     string
	}{
		{
			"correct horse battery staple",
			"$argon2id$v=19$m=19456,t=2,p=1$LmJSU2xoQUNSa1p1OVpKWA$7kRchyC7pQ9sCXYoi2xZuRCW6Uax/WYiYbabL3xf7fo",
		},
		{
			"pw",
			"$argon2id$v=19$m=1025,t=1,p=3$OWd2TDdiMUxBMk9tV2syNw$vqfB4dlZHnNFOkkJZknpBDTwUei2hL8bSMY08Prw+3M",
		},
		{
			"pw",
			"$argon2id$v=19$m=8,t=1,p=1$TXFMaVh2TXU5M1E4eEZkbg$c4nH+aOkmzeXQJE1Z4NUzzsLqpodHh53o8HrGJxlvCE",
		},
		{
			"pw",
			"$argon2i$v=19$m=256,t=1,p=1$cS9QSzJUc0FEdG9GMHA2Wg$DqXqGB6ym/9L5u2JTWBA/tIjewj0KUPJOnJgpw5WdK4",
		},
		{
			"pw",
			"$argon2id$v=19$m=256,t=3,p=2$d3ZTWk9yMk16Y0t4SkgvYQ$f+VzAc7Ya5B04A2ZJWeJCPJ5jkZZFRaUTXxR982VaN8",
		},
	}

	for _, tc := range cases {
		if !argon2Verify(tc.password, tc.hash) {
			t.Errorf("argon2Verify(%q, %q) = false", tc.password, tc.hash)
		}
		if argon2Verify(tc.password+"x", tc.hash) {
			t.Errorf("argon2Verify rejected nothing for %q", tc.hash)
		}
	}
}

// TestArgon2VariantsDiffer pins that argon2i and argon2id derive different tags
// from the same parameters. password_verify reads the variant out of the stored
// hash; reading the wrong one answers false for the right password.
func TestArgon2VariantsDiffer(t *testing.T) {
	const i = "$argon2i$v=19$m=256,t=1,p=1$cS9QSzJUc0FEdG9GMHA2Wg$DqXqGB6ym/9L5u2JTWBA/tIjewj0KUPJOnJgpw5WdK4"
	id := strings.Replace(i, "$argon2i$", "$argon2id$", 1)
	if argon2Verify("pw", id) {
		t.Error("an argon2i tag verified as argon2id")
	}
}

// TestArgon2RoundTrip checks that what password_hash writes is what
// password_get_info reads, for both variants.
func TestArgon2RoundTrip(t *testing.T) {
	for _, algo := range []string{algoArgon2i, algoArgon2id} {
		want := params{algo: algo, memory: 256, time: 1, lanes: 1}
		hash, err := argon2Hash("pw", want)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(hash, "$"+algo+"$v=19$m=256,t=1,p=1$") {
			t.Errorf("argon2Hash wrote %q", hash)
		}
		got, ok := paramsOf(hash)
		if !ok || got != want {
			t.Errorf("paramsOf(%q) = %+v, %v, want %+v", hash, got, ok, want)
		}
		if !argon2Verify("pw", hash) {
			t.Errorf("own hash %q did not verify", hash)
		}
	}
}

// TestArgon2SaltIsFresh pins that two hashes of one password differ, which is
// the salt doing its job and is the property a digest does not have.
func TestArgon2SaltIsFresh(t *testing.T) {
	p := params{algo: algoArgon2id, memory: 256, time: 1, lanes: 1}
	first, err := argon2Hash("pw", p)
	if err != nil {
		t.Fatal(err)
	}
	second, err := argon2Hash("pw", p)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Errorf("two hashes of one password are equal: %q", first)
	}
}

// TestArgon2DecodeRejects covers every way a stored string can fail to be a
// hash this build will act on. Each one is an error: a best-effort read would
// take a wrong parameter set, derive a wrong tag, and answer false for the
// right password.
func TestArgon2DecodeRejects(t *testing.T) {
	const good = "$argon2id$v=19$m=256,t=1,p=1$cS9QSzJUc0FEdG9GMHA2Wg$DqXqGB6ym/9L5u2JTWBA/tIjewj0KUPJOnJgpw5WdK4"

	cases := map[string]string{
		"empty":                  "",
		"bcrypt":                 "$2y$12$k0fMOCrVG7hFSPWZ0.FNne/QhQJ3hLnCPEJ3OL7ZPfVoJTF2jrfbC",
		"no leading dollar":      strings.TrimPrefix(good, "$"),
		"unknown variant":        strings.Replace(good, "argon2id", "argon2x", 1),
		"argon2 1.0":             strings.Replace(good, "$v=19", "", 1),
		"future version":         strings.Replace(good, "v=19", "v=20", 1),
		"unknown parameter":      strings.Replace(good, "m=256", "k=256", 1),
		"parameter no value":     strings.Replace(good, "m=256", "m", 1),
		"parameter not a number": strings.Replace(good, "m=256", "m=lots", 1),
		"lanes above uint8":      strings.Replace(good, "p=1", "p=256", 1),
		"memory below a lane":    strings.Replace(good, "m=256", "m=4", 1),
		"zero passes":            strings.Replace(good, "t=1", "t=0", 1),
		"salt not base64":        strings.Replace(good, "cS9QSzJUc0FEdG9GMHA2Wg", "not base64!", 1),
		"empty tag":              strings.Replace(good, "DqXqGB6ym/9L5u2JTWBA/tIjewj0KUPJOnJgpw5WdK4", "", 1),
	}

	for name, hash := range cases {
		if _, _, _, err := argon2Decode(hash); err == nil {
			t.Errorf("%s: argon2Decode(%q) returned no error", name, hash)
		}
		if argon2Verify("pw", hash) {
			t.Errorf("%s: argon2Verify(%q) = true", name, hash)
		}
	}

	if _, _, _, err := argon2Decode(good); err != nil {
		t.Errorf("argon2Decode of the control = %v", err)
	}
}
