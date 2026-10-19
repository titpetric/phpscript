package crypto

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// hashRuntime returns a runtime carrying the digest bindings alone.
func hashRuntime(t *testing.T) *runner.Runtime {
	t.Helper()

	rt := runner.New(io.Discard, runner.Options{})
	RegisterHash(rt)
	return rt
}

// TestRegisterHash covers the digest names a script calls and what they answer.
// Every hex vector below is the published digest of its input, so a change to
// the table or to the rendering fails here. The return is a string and never a
// []byte: a PHP string is a Go string in this runtime, and a []byte would reach
// a script as a foreign value that strlen and concatenation do not recognise.
func TestRegisterHash(t *testing.T) {
	t.Parallel()
	rt := hashRuntime(t)

	for _, test := range []struct {
		name string
		args []any
		want string
	}{
		{name: "md5", args: []any{""}, want: "d41d8cd98f00b204e9800998ecf8427e"},
		{name: "md5", args: []any{"abc"}, want: "900150983cd24fb0d6963f7d28e17f72"},
		{name: "sha1", args: []any{""}, want: "da39a3ee5e6b4b0d3255bfef95601890afd80709"},
		{name: "sha1", args: []any{"abc"}, want: "a9993e364706816aba3e25717850c26c9cd0d89d"},
		{
			name: "hash",
			args: []any{"sha256", "abc"},
			want: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		},
		{name: "hash", args: []any{"crc32b", "abc"}, want: "352441c2"},
		{
			// HMAC digests a key longer than the block size, which is
			// HMAC's own rule and no choice made here.
			name: "hash_hmac",
			args: []any{"sha256", "abc", "key"},
			want: "9c196e32dc0175f86f4b1cb89289d6619de6bee699e4c378e68309ed97a1a6ab",
		},
	} {
		t.Run(test.name+" "+test.want[:8], func(t *testing.T) {
			got, err := rt.InvokeNamed(test.name, test.args...)
			if err != nil {
				t.Fatalf("%s(): %v", test.name, err)
			}
			if got != test.want {
				t.Errorf("%s(%v) = %v, want %q", test.name, test.args, got, test.want)
			}
		})
	}
}

// TestRegisterHash_binary covers the raw-bytes tail. A digest requested as binary
// output answers the sum itself, so its length is the algorithm's digest size
// and the hex form is twice that.
func TestRegisterHash_binary(t *testing.T) {
	t.Parallel()
	rt := hashRuntime(t)

	for _, test := range []struct {
		name string
		args []any
		size int
	}{
		{name: "md5", args: []any{"abc", true}, size: 16},
		{name: "sha1", args: []any{"abc", true}, size: 20},
		{name: "hash", args: []any{"sha512", "abc", true}, size: 64},
		{name: "hash_hmac", args: []any{"sha256", "abc", "key", true}, size: 32},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := rt.InvokeNamed(test.name, test.args...)
			if err != nil {
				t.Fatalf("%s(): %v", test.name, err)
			}
			raw, ok := got.(string)
			if !ok {
				t.Fatalf("%s() returned %T, want a string", test.name, got)
			}
			if len(raw) != test.size {
				t.Errorf("%s() returned %d bytes, want %d", test.name, len(raw), test.size)
			}
		})
	}
}

// TestRegisterHash_unknownAlgorithm covers a name this build does not carry.
// There is nothing to clamp to, so the call fails with PHP 8's ValueError
// wording and a script that catches it reads a familiar message.
func TestRegisterHash_unknownAlgorithm(t *testing.T) {
	t.Parallel()
	rt := hashRuntime(t)

	for _, name := range []string{"hash", "hash_hmac"} {
		args := []any{"whirlpool", "abc"}
		if name == "hash_hmac" {
			args = append(args, "key")
		}
		_, err := rt.InvokeNamed(name, args...)
		if err == nil {
			t.Fatalf("%s() accepted an algorithm this build does not carry", name)
		}
		want := name + `(): Argument #1 ($algo) must be a valid hashing algorithm, "whirlpool" given`
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	}
}

// TestRegisterHash_algos covers the list a script reads before it lets a user
// pick an algorithm. It answers this build's table and not PHP's, so every
// name it reports is one hash() accepts, and the names arrive sorted.
func TestRegisterHash_algos(t *testing.T) {
	t.Parallel()
	rt := hashRuntime(t)

	got, err := rt.InvokeNamed("hash_algos")
	if err != nil {
		t.Fatal(err)
	}
	names, ok := got.([]any)
	if !ok {
		t.Fatalf("hash_algos() returned %T, want a list", got)
	}
	if len(names) == 0 {
		t.Fatal("hash_algos() answered nothing")
	}

	previous := ""
	for _, value := range names {
		name, ok := value.(string)
		if !ok {
			t.Fatalf("hash_algos() held %T, want a string", value)
		}
		if name <= previous {
			t.Errorf("hash_algos() reported %q after %q, want them sorted", name, previous)
		}
		previous = name

		if _, err := rt.InvokeNamed("hash", name, "abc"); err != nil {
			t.Errorf("hash_algos() reported %q, which hash() refuses: %v", name, err)
		}
	}
}

// TestRegisterHash_equals covers hash_equals(). Strings of different lengths
// are never equal, and the comparison of equal-length strings runs in time
// that does not depend on how far along they first differ, so a token check
// leaks nothing but the length of the guess.
func TestRegisterHash_equals(t *testing.T) {
	t.Parallel()
	rt := hashRuntime(t)

	for _, test := range []struct {
		known string
		user  string
		want  bool
	}{
		{known: "token", user: "token", want: true},
		{known: "", user: "", want: true},
		{known: "token", user: "tokeN"},
		{known: "token", user: "token "},
		{known: "token", user: ""},
	} {
		t.Run(test.known+"/"+test.user, func(t *testing.T) {
			got, err := rt.InvokeNamed("hash_equals", test.known, test.user)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Errorf("hash_equals(%q, %q) = %v, want %v", test.known, test.user, got, test.want)
			}
		})
	}
}
