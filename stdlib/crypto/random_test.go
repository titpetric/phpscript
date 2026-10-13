package crypto

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// randomRuntime returns a runtime carrying the CSPRNG bindings alone.
func randomRuntime(t *testing.T) *runner.Runtime {
	t.Helper()

	rt := runner.New(io.Discard, runner.Options{})
	RegisterRandom(rt)
	return rt
}

// TestRegisterRandom covers the three names and the bounds each one holds to.
// random_int and rand both pick from [min, max] inclusive, so a range of one
// value is that value and a range spanning zero reaches both signs. The span
// is computed in big.Int because max-min+1 overflows int64 once the bounds
// cover most of the type, and rand.Int does the rejection sampling that keeps
// the pick unbiased.
func TestRegisterRandom(t *testing.T) {
	rt := randomRuntime(t)

	for _, test := range []struct {
		name string
		min  int64
		max  int64
	}{
		{name: "one value", min: 5, max: 5},
		{name: "small range", min: 1, max: 6},
		{name: "spans zero", min: -1, max: 1},
		{name: "negative only", min: -10, max: -8},
		{name: "the whole type", min: -9223372036854775808, max: 9223372036854775807},
	} {
		t.Run(test.name, func(t *testing.T) {
			for range 64 {
				got, err := rt.InvokeNamed("random_int", test.min, test.max)
				if err != nil {
					t.Fatalf("random_int(%d, %d): %v", test.min, test.max, err)
				}
				picked, ok := got.(int64)
				if !ok {
					t.Fatalf("random_int() returned %T, want an int64", got)
				}
				if picked < test.min || picked > test.max {
					t.Fatalf("random_int(%d, %d) = %d, outside the inclusive range", test.min, test.max, picked)
				}
			}
		})
	}
}

// TestRegisterRandom_bytes covers random_bytes(). The length is honoured
// exactly, and a length below one is refused: PHP throws there, and returning
// an empty string would read as key material a caller never got.
func TestRegisterRandom_bytes(t *testing.T) {
	rt := randomRuntime(t)

	for _, length := range []int64{1, 16, 64} {
		got, err := rt.InvokeNamed("random_bytes", length)
		if err != nil {
			t.Fatalf("random_bytes(%d): %v", length, err)
		}
		bytes, ok := got.(string)
		if !ok {
			t.Fatalf("random_bytes() returned %T, want a string", got)
		}
		if int64(len(bytes)) != length {
			t.Errorf("random_bytes(%d) returned %d bytes", length, len(bytes))
		}
	}

	for _, length := range []int64{0, -1} {
		_, err := rt.InvokeNamed("random_bytes", length)
		if err == nil {
			t.Fatalf("random_bytes(%d) was accepted", length)
		}
		want := "random_bytes(): Argument #1 ($length) must be greater than 0"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	}

	// Two calls of the same length differ, so the source is read per call.
	first, err := rt.InvokeNamed("random_bytes", int64(32))
	if err != nil {
		t.Fatal(err)
	}
	second, err := rt.InvokeNamed("random_bytes", int64(32))
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Error("two 32-byte draws are identical, so the source is not being read")
	}
}

// TestRegisterRandom_bounds covers the refusals and rand()'s argument counts.
// rand() takes nothing or both bounds, and an argument-less call spans 0 to
// PHP's getrandmax(); a reversed range is refused and never silently
// swapped, so a script computing its bounds wrongly fails where it did so.
func TestRegisterRandom_bounds(t *testing.T) {
	rt := randomRuntime(t)

	if _, err := rt.InvokeNamed("random_int", int64(2), int64(1)); err == nil {
		t.Error("random_int(2, 1) was accepted")
	}
	if _, err := rt.InvokeNamed("rand", int64(2), int64(1)); err == nil {
		t.Error("rand(2, 1) was accepted")
	}
	if _, err := rt.InvokeNamed("rand", int64(1)); err == nil {
		t.Error("rand(1) was accepted, want exactly 0 or 2 arguments")
	}

	for range 64 {
		got, err := rt.InvokeNamed("rand")
		if err != nil {
			t.Fatalf("rand(): %v", err)
		}
		picked, ok := got.(int64)
		if !ok {
			t.Fatalf("rand() returned %T, want an int64", got)
		}
		if picked < 0 || picked > randMax {
			t.Fatalf("rand() = %d, outside 0..%d", picked, randMax)
		}
	}

	got, err := rt.InvokeNamed("rand", int64(7), int64(7))
	if err != nil {
		t.Fatal(err)
	}
	if got != int64(7) {
		t.Errorf("rand(7, 7) = %v, want 7", got)
	}
}
