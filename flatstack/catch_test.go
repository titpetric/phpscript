package flatstack_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/stdlib"
)

// TestFlatstackCatchClauseSelection pins which catch clause a throw enters.
// A fixture cannot tell a bytecode run from an interpreter fallback that
// happened to agree, so every case asserts Supports first.
func TestFlatstackCatchClauseSelection(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		want    string
		wantErr string
	}{
		{
			name:   "later clause matches",
			source: `<?php try { throw new RuntimeException("x"); } catch (LogicException $e) { echo "logic"; } catch (RuntimeException $e) { echo "runtime"; }`,
			want:   "runtime",
		},
		{
			name:   "throwable after a non-matching clause",
			source: `<?php try { throw new RuntimeException("x"); } catch (LogicException $e) { echo "logic"; } catch (Throwable $e) { echo "throwable"; }`,
			want:   "throwable",
		},
		{
			name:   "union type matches its second alternative",
			source: `<?php try { throw new RuntimeException("x"); } catch (LogicException|RuntimeException $e) { echo "union"; }`,
			want:   "union",
		},
		{
			name:   "first matching clause wins over a later one",
			source: `<?php try { throw new RuntimeException("x"); } catch (Throwable $e) { echo "first"; } catch (RuntimeException $e) { echo "second"; }`,
			want:   "first",
		},
		{
			name:   "unmatched throw propagates to the enclosing try",
			source: `<?php try { try { throw new RuntimeException("x"); } catch (LogicException $e) { echo "logic"; } finally { echo "finally:"; } echo "after"; } catch (Throwable $e) { echo "outer"; }`,
			want:   "finally:outer",
		},
		{
			name:   "finally runs when a clause matched",
			source: `<?php try { throw new RuntimeException("x"); } catch (RuntimeException $e) { echo "caught:"; } finally { echo "finally"; }`,
			want:   "caught:finally",
		},
		{
			name:   "throw out of a clause body skips its siblings but not its finally",
			source: `<?php try { try { throw new RuntimeException("x"); } catch (RuntimeException $e) { throw new LogicException("y"); } catch (Throwable $e) { echo "sibling"; } finally { echo "finally:"; } } catch (Throwable $e) { echo "outer"; }`,
			want:   "finally:outer",
		},
		{
			// catch (Exception) declines an engine error, so the clause after
			// it is the one that runs.
			name:   "exception does not catch an engine error",
			source: `<?php try { explode(",", "a,b", array()); } catch (Exception $e) { echo "exception"; } catch (Throwable $e) { echo "throwable"; }`,
			want:   "throwable",
		},
		{
			name:    "unmatched throw leaves the program after finally",
			source:  `<?php try { throw new RuntimeException("boom"); } catch (LogicException $e) { echo "logic"; } finally { echo "finally"; }`,
			want:    "finally",
			wantErr: "boom",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program, err := parser.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			if err := flatstack.Supports(program); err != nil {
				t.Fatalf("expected native bytecode support: %v", err)
			}
			var output strings.Builder
			runtime := flatstack.New(&output, flatstack.Options{})
			// The SPL class names and explode() are stdlib registrations;
			// without them a throw would fail before any clause is reached.
			stdlib.Register(runtime)
			err = runtime.Run(program)
			switch {
			case test.wantErr == "" && err != nil:
				t.Fatalf("run: %v", err)
			case test.wantErr != "" && err == nil:
				t.Fatalf("run returned no error, want one containing %q", test.wantErr)
			case test.wantErr != "" && !strings.Contains(err.Error(), test.wantErr):
				t.Fatalf("error = %v, want one containing %q", err, test.wantErr)
			}
			if got := output.String(); got != test.want {
				t.Fatalf("output = %q, want %q", got, test.want)
			}
		})
	}
}

// TestFlatstackCatchSurvivesCalleeJumps pins the handler discipline across
// frames: opJump discards a handler when the jump leaves its try's pc range,
// and a callee's pcs are always outside the caller's range, so the range rule
// is scoped to the frame that armed the handler. Before that scoping, the
// first branch in a called function silently disarmed the caller's catch.
func TestFlatstackCatchSurvivesCalleeJumps(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "branch in the callee",
			source: `<?php function work() { if (true) { echo "body"; } throw new RuntimeException("x"); } try { work(); } catch (RuntimeException $e) { echo "-caught"; }`,
			want:   "body-caught",
		},
		{
			name:   "loop in the callee",
			source: `<?php function work() { for ($i = 0; $i < 2; $i++) { echo "."; } throw new RuntimeException("x"); } try { work(); } catch (RuntimeException $e) { echo "-caught"; }`,
			want:   "..-caught",
		},
		{
			name:   "callee catches its own throw first",
			source: `<?php function work() { try { throw new LogicException("inner"); } catch (LogicException $e) { echo "inner"; } throw new RuntimeException("x"); } try { work(); } catch (RuntimeException $e) { echo "-outer"; }`,
			want:   "inner-outer",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program, err := parser.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			if err := flatstack.Supports(program); err != nil {
				t.Fatalf("expected native bytecode support: %v", err)
			}
			var output strings.Builder
			runtime := flatstack.New(&output, flatstack.Options{})
			stdlib.Register(runtime)
			if err := runtime.Run(program); err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := output.String(); got != test.want {
				t.Fatalf("output = %q, want %q", got, test.want)
			}
		})
	}
}
