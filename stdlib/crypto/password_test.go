package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"io"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// benchPassword is the input every derivation below runs on. Its length
// matters to neither algorithm: both consume a fixed-size block of key
// material whatever they were handed.
const benchPassword = "correct horse battery staple"

// TestParamsFrom covers the ($algo, $options) tail both hashing functions
// take: the algorithm decides which option keys are read and what an absent
// key falls back to, so the two cannot be parsed independently.
func TestParamsFrom(t *testing.T) {
	options := func(pairs ...any) *model.Array {
		out := model.NewArray()
		for i := 0; i+1 < len(pairs); i += 2 {
			out.Set(pairs[i].(string), pairs[i+1])
		}
		return out
	}

	cases := []struct {
		name string
		opts []any
		want params
	}{
		{"no tail", nil, params{algo: algoBcrypt, cost: defaultCost}},
		{"null algo", []any{nil}, params{algo: algoBcrypt, cost: defaultCost}},
		{"bcrypt", []any{algoBcrypt}, params{algo: algoBcrypt, cost: defaultCost}},
		{"bcrypt by name", []any{"bcrypt"}, params{algo: algoBcrypt, cost: defaultCost}},
		{"bcrypt cost", []any{algoBcrypt, options("cost", int64(4))}, params{algo: algoBcrypt, cost: 4}},
		{"cost as a string", []any{options("cost", "5")}, params{algo: algoBcrypt, cost: 5}},
		{"php 7 integer bcrypt", []any{int64(1)}, params{algo: algoBcrypt, cost: defaultCost}},
		{"php 7 integer argon2i", []any{int64(2)}, defaultParams(algoArgon2i)},
		{"php 7 integer argon2id", []any{int64(3)}, defaultParams(algoArgon2id)},
		{"argon2id defaults", []any{algoArgon2id}, defaultParams(algoArgon2id)},
		{"argon2i defaults", []any{algoArgon2i}, defaultParams(algoArgon2i)},
		{
			"argon2id tuned",
			[]any{algoArgon2id, options("memory_cost", int64(19456), "time_cost", int64(2), "threads", int64(1))},
			params{algo: algoArgon2id, memory: 19456, time: 2, lanes: 1},
		},
		{
			"argon2id partial options keep the defaults",
			[]any{algoArgon2id, options("time_cost", int64(2))},
			params{algo: algoArgon2id, memory: argon2DefaultMemory, time: 2, lanes: argon2DefaultLanes},
		},
		{
			"a bcrypt cost is not an argon2 option",
			[]any{algoArgon2id, options("cost", int64(4))},
			defaultParams(algoArgon2id),
		},
	}

	for _, tc := range cases {
		got, err := paramsFrom(tc.opts)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: paramsFrom = %+v, want %+v", tc.name, got, tc.want)
		}
	}

	refused := []struct {
		name string
		opts []any
	}{
		{"unknown algorithm", []any{"scrypt"}},
		{"cost below bcrypt's minimum", []any{options("cost", int64(3))}},
		{"cost above bcrypt's maximum", []any{options("cost", int64(32))}},
		{"cost is not a number", []any{options("cost", "cheap")}},
		{"zero passes", []any{algoArgon2id, options("time_cost", int64(0))}},
		{"zero threads", []any{algoArgon2id, options("threads", int64(0))}},
		{"memory below one lane", []any{algoArgon2id, options("memory_cost", int64(4))}},
		{"more lanes than memory", []any{algoArgon2id, options("memory_cost", int64(16), "threads", int64(3))}},
		{"more lanes than the encoding carries", []any{algoArgon2id, options("threads", int64(256))}},
		{"an argument that is neither", []any{true}},
	}

	for _, tc := range refused {
		if got, err := paramsFrom(tc.opts); err == nil {
			t.Errorf("%s: paramsFrom = %+v, want an error", tc.name, got)
		}
	}
}

// TestBcryptParamsOf pins what password_get_info reads back off a bcrypt hash,
// in both the revision php writes and the one x/crypto writes.
func TestBcryptParamsOf(t *testing.T) {
	hash, err := derive("pw", params{algo: algoBcrypt, cost: 4})
	if err != nil {
		t.Fatal(err)
	}
	if hash[:4] != phpPrefix {
		t.Errorf("derive wrote %q", hash[:4])
	}

	for _, stored := range []string{hash, goForm(hash)} {
		got, ok := paramsOf(stored)
		if !ok {
			t.Fatalf("paramsOf(%q) failed", stored)
		}
		if want := (params{algo: algoBcrypt, cost: 4}); got != want {
			t.Errorf("paramsOf(%q) = %+v, want %+v", stored, got, want)
		}
	}

	if _, ok := paramsOf("not-a-hash"); ok {
		t.Error("paramsOf read a non-hash")
	}
}

// BenchmarkPasswordHash prices one derivation, which is the whole of
// password_hash(): the encoding around it is a few string writes.
//
// Every argon2 row runs one lane, because a row with more would spread over
// goroutines and the pinned benchmark job narrows the affinity mask to one
// CPU, which would report a parallel derivation as a serial one.
func BenchmarkPasswordHash(b *testing.B) {
	for _, cost := range []int{4, 8, 10, 11, 12, 13} {
		b.Run("bcrypt/cost="+strconv.Itoa(cost), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := bcrypt.GenerateFromPassword([]byte(benchPassword), cost); err != nil {
					b.Fatal(err)
				}
			}
		})
	}

	// php's own default, then two of RFC 9106's recommended sets: the same
	// resistance bought with less memory and more passes.
	sets := []params{
		{algo: algoArgon2id, memory: argon2DefaultMemory, time: argon2DefaultTime, lanes: 1},
		{algo: algoArgon2id, memory: 19456, time: 2, lanes: 1},
		{algo: algoArgon2id, memory: 9216, time: 4, lanes: 1},
	}
	for _, p := range sets {
		name := "argon2id/m=" + strconv.Itoa(int(p.memory)) + ",t=" + strconv.Itoa(int(p.time)) + ",p=1"
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := argon2Hash(benchPassword, p); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkPasswordVerify prices password_verify() against a stored hash. The
// derivation is the one password_hash runs, at the work factors the hash
// records, so a login costs what the registration cost. Lowering the cost
// leaves every hash already stored at the cost it was written at.
func BenchmarkPasswordVerify(b *testing.B) {
	stored := []params{
		{algo: algoBcrypt, cost: 10},
		{algo: algoBcrypt, cost: defaultCost},
		{algo: algoArgon2id, memory: 19456, time: 2, lanes: 1},
	}

	for _, p := range stored {
		hash, err := derive(benchPassword, p)
		if err != nil {
			b.Fatal(err)
		}
		name := p.algo
		if p.algo == algoBcrypt {
			name = "bcrypt/cost=" + strconv.Itoa(p.cost)
		} else {
			name += "/m=" + strconv.Itoa(int(p.memory)) + ",t=" + strconv.Itoa(int(p.time)) + ",p=1"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if isArgon2(hash) {
					if !argon2Verify(benchPassword, hash) {
						b.Fatal("verify failed")
					}
					continue
				}
				if err := bcrypt.CompareHashAndPassword([]byte(goForm(hash)), []byte(benchPassword)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// refusedHashes are the spellings password_verify answers by shape. No
// derivation can turn any of them into a match: the first three carry no salt
// and no tag, and the fourth is argon2 1.0, which derives a different tag from
// the same parameters and is refused as unreadable.
var refusedHashes = []struct{ name, hash string }{
	{"empty", ""},
	{"not-a-hash", "not-a-hash"},
	{"bcrypt/truncated", "$2y$12$Lw1SjMDYJ.x6mKLVCqTfke"},
	{"argon2id/version=16", "$argon2id$v=16$m=65536,t=4,p=1$c29tZXNhbHRzb21lc2FsdA$RdescudvJCsgt3ub+b+dWRWJTmaaJObG"},
}

// TestPasswordVerifyRefusesWithoutDeriving bounds the time a refusal takes.
// Every return value below was already false.
//
// password_verify("x", "") ran a bcrypt cost-12 derivation first: 214 ms of one
// core, where php returns in 31 ns. 4000 refusals measure 804 us. One
// derivation at cost 4, the cheapest bcrypt accepts, is 898 us, so a 200 ms
// ceiling fails on a decoy at any cost and leaves two orders of magnitude of
// headroom over the measurement.
//
// It runs through the registered function, not passwordVerify, because the decoy
// lived in the registration.
func TestPasswordVerifyRefusesWithoutDeriving(t *testing.T) {
	rt := runner.New(io.Discard, runner.Options{})
	Register(rt)

	const rounds = 1000
	start := time.Now()
	for range rounds {
		for _, tt := range refusedHashes {
			got, err := rt.InvokeNamed("password_verify", benchPassword, tt.hash)
			if err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if got != false {
				t.Fatalf("%s: password_verify returned %v, want false", tt.name, got)
			}
		}
	}

	elapsed := time.Since(start)
	t.Logf("%d refusals in %v", rounds*len(refusedHashes), elapsed)
	if elapsed > 200*time.Millisecond {
		t.Errorf("%d refusals took %v, want under 200ms: a derivation is being spent on a hash that cannot match", rounds*len(refusedHashes), elapsed)
	}
}

// BenchmarkPasswordVerifyRefuse prices the path a stored value that is not a
// hash takes. It is the one row in this file that is not a derivation, which is
// the point of it: the cost is the length check and the prefix compare, and the
// allocation count is zero because nothing is copied to a []byte.
func BenchmarkPasswordVerifyRefuse(b *testing.B) {
	for _, tt := range refusedHashes {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if passwordVerify(benchPassword, tt.hash) {
					b.Fatal("verify accepted a hash it cannot read")
				}
			}
		})
	}
}

// BenchmarkPasswordVerifyThroughput answers how many logins a host serves,
// which is not one derivation divided into the core count.
//
// A bcrypt derivation holds a core and 5 KiB, so two of them run at twice the
// rate of one. An argon2id derivation holds its whole memory_cost and streams
// it, so two of them contend for memory bandwidth and the rate falls short of
// twice. Dividing BenchmarkPasswordVerify's sec/op into the core count reports
// the bcrypt case correctly and overstates the argon2 one, so the rate is
// measured: under RunParallel, sec/op is wall clock per completed
// derivation across the cores in play, and logins per second is its reciprocal.
//
// `-cpu 1,2,4` is how the scaling is read, because RunParallel spawns GOMAXPROCS
// goroutines and -cpu is what sets it. One row per core count, named by it.
func BenchmarkPasswordVerifyThroughput(b *testing.B) {
	stored := []params{
		{algo: algoBcrypt, cost: defaultCost},
		{algo: algoArgon2id, memory: 19456, time: 2, lanes: 1},
	}

	for _, p := range stored {
		name := "bcrypt/cost=" + strconv.Itoa(p.cost)
		if p.algo != algoBcrypt {
			name = "argon2id/m=" + strconv.Itoa(int(p.memory)) +
				",t=" + strconv.Itoa(int(p.time)) + ",p=1"
		}
		hash, err := derive(benchPassword, p)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if !passwordVerify(benchPassword, hash) {
						b.Error("verify failed")
						return
					}
				}
			})
		})
	}
}

// BenchmarkTokenDigest prices the per-request half of an authentication flow:
// a digest of a high-entropy bearer token, and the keyed digest a signed
// session cookie is checked with. Both are the same order as the rest of a
// request, which is the measured reason a session is not re-derived from the
// password on every call.
func BenchmarkTokenDigest(b *testing.B) {
	token := []byte("01K6Y3Q7VT8ZC4N2M9XJ5B7D6F")
	key := []byte("a-32-byte-signing-key-for-hmac!!")

	b.Run("sha256", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = sha256.Sum256(token)
		}
	})
	b.Run("hmac-sha256", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			mac := hmac.New(sha256.New, key)
			mac.Write(token)
			_ = mac.Sum(nil)
		}
	})
}
