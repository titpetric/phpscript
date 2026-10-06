package runner_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// runLimited runs src on a runtime carrying ctx and returns what it printed and
// how it ended.
func runLimited(t testing.TB, ctx context.Context, src string) (string, error) {
	t.Helper()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	rt.SetContext(ctx)

	program, err := rt.Load(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	runErr := rt.Run(program)
	return out.String(), runErr
}

// TestTimeLimitEndsALoop is the whole of set_time_limit: a script that would
// not stop on its own is stopped, and told why.
func TestTimeLimitEndsALoop(t *testing.T) {
	t.Parallel()
	started := time.Now()
	_, err := runLimited(t, context.Background(), `<?php
set_time_limit(1);
while (true) { $n = 1; }
`)

	var limit *runner.TimeLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("error = %v, want *runner.TimeLimitError", err)
	}
	if limit.Aborted {
		t.Error("reported as an abort, want a limit")
	}
	if got := err.Error(); got != "Maximum execution time of 1 second exceeded" {
		t.Errorf("message = %q", got)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Errorf("ran for %s, want about a second", elapsed)
	}
}

// TestTimeLimitSleepsAreCut holds the limit over a wait as well as over a
// loop: a sleep is on the runtime context, so it ends when the script does.
func TestTimeLimitSleepsAreCut(t *testing.T) {
	t.Parallel()
	started := time.Now()
	_, err := runLimited(t, context.Background(), `<?php
set_time_limit(1);
sleep(30);
echo "woke";
`)
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("slept %s, want the limit to cut it at about a second", elapsed)
	}
	// The sleep returns rather than throwing; the limit is reported on the next
	// statement, which is what the per-statement check is.
	var limit *runner.TimeLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("error = %v, want *runner.TimeLimitError", err)
	}
}

// TestTimeLimitZeroRemovesIt covers the spelling that turns the limit off.
func TestTimeLimitZeroRemovesIt(t *testing.T) {
	t.Parallel()
	out, err := runLimited(t, context.Background(), `<?php
set_time_limit(1);
set_time_limit(0);
for ($i = 0; $i < 100000; $i++) { $n = $i; }
echo "finished";
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "finished" {
		t.Errorf("output = %q, want %q", out, "finished")
	}
}

// TestClientAbortEndsTheScript is the default: the connection went away, so the
// script does too.
func TestClientAbortEndsTheScript(t *testing.T) {
	t.Parallel()
	ctx, abort := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		abort()
	}()

	_, err := runLimited(t, ctx, `<?php
while (true) { $n = 1; }
`)

	var limit *runner.TimeLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("error = %v, want *runner.TimeLimitError", err)
	}
	if !limit.Aborted {
		t.Error("reported as a limit, want an abort")
	}
}

// TestIgnoreUserAbortRunsOn is the other half: the script asked to finish, and
// connection_aborted is how it finds out anyway.
func TestIgnoreUserAbortRunsOn(t *testing.T) {
	t.Parallel()
	ctx, abort := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		abort()
	}()

	out, err := runLimited(t, ctx, `<?php
ignore_user_abort(true);
$ticks = 0;
while ($ticks < 100) {
	usleep(10000);
	$ticks++;
	if (connection_aborted()) {
		break;
	}
}
echo connection_aborted() ? "noticed" : "never noticed";
echo ", ran to the end";
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.HasPrefix(out, "noticed") {
		t.Errorf("output = %q, want it to have noticed the abort", out)
	}
	if !strings.HasSuffix(out, "ran to the end") {
		t.Errorf("output = %q, want the script to have finished", out)
	}
}

// TestConnectionAbortedWithoutARequest keeps the functions present and inert on
// a runtime nothing is waiting on, which is every command line run.
func TestConnectionAbortedWithoutARequest(t *testing.T) {
	t.Parallel()
	out, err := runLimited(t, context.Background(), `<?php
var_dump(connection_aborted());
var_dump(set_time_limit(30));
ignore_user_abort(true);
var_dump(connection_aborted());
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "bool(false)\nbool(true)\nbool(false)\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestTimeLimitIsNotCatchable holds the limit past a try, the way php holds its
// own: there is no more time to carry on with, so the clause does not run and
// the error reaches the host.
func TestTimeLimitIsNotCatchable(t *testing.T) {
	t.Parallel()
	out, err := runLimited(t, context.Background(), `<?php
set_time_limit(1);
try {
	while (true) { $n = 1; }
} catch (Throwable $e) {
	echo "caught";
} finally {
	echo "finally";
}
`)
	if err == nil {
		t.Fatal("the limit was handled, want it reported")
	}
	if out != "" {
		t.Errorf("output = %q, want neither clause to have run", out)
	}
}

// TestShutdownRunsAfterTheLimit is what makes the limit survivable: the clock
// is off for the shutdown pass, so a callback registered to close what the
// script opened gets to run.
func TestShutdownRunsAfterTheLimit(t *testing.T) {
	t.Parallel()
	out, err := runLimited(t, context.Background(), `<?php
set_time_limit(1);
register_shutdown_function(function () {
	echo "closed";
});
while (true) { $n = 1; }
`)
	var limit *runner.TimeLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("error = %v, want *runner.TimeLimitError", err)
	}
	if out != "closed" {
		t.Errorf("output = %q, want the shutdown callback to have run", out)
	}
}

// TestEnterRequestScopesTheConnection covers what HTTP\Mux does per handler:
// the connection being answered is the one connection_aborted reports on, and
// the previous one comes back afterwards.
func TestEnterRequestScopesTheConnection(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)
	rt.SetContext(context.Background())

	if rt.ConnectionAborted() {
		t.Fatal("aborted before anything happened")
	}

	request, disconnect := context.WithCancel(context.Background())
	leave := rt.EnterRequest(request)
	if rt.ConnectionAborted() {
		t.Error("aborted while the request was live")
	}
	disconnect()
	if !rt.ConnectionAborted() {
		t.Error("not aborted after the request's client went away")
	}

	leave()
	if rt.ConnectionAborted() {
		t.Error("the script's own connection was reported as aborted")
	}
}

// TestTimeLimitEndsAnEmptyLoop covers the shortest loop there is. The check
// used to run per statement, and a body with no statements never reached it,
// so `while (true) {}` outran every limit on the interpreter.
func TestTimeLimitEndsAnEmptyLoop(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"{}", "{ }", ";"} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			done := make(chan error, 1)
			go func() {
				_, err := runLimited(t, context.Background(), "<?php\nset_time_limit(1);\nwhile (true) "+body+"\n")
				done <- err
			}()
			select {
			case err := <-done:
				var limit *runner.TimeLimitError
				if !errors.As(err, &limit) {
					t.Fatalf("error = %v, want *runner.TimeLimitError", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("the loop outran its 1s limit")
			}
		})
	}
}

// TestTimeLimitHoldsPastAnAbortBeingIgnored is the contract SetIgnoreUserAbort
// documents: ignoring the client is not permission to run forever. The limit
// used to be unreachable once the host context had been cancelled, because the
// cause was read off that context.
func TestTimeLimitHoldsPastAnAbortBeingIgnored(t *testing.T) {
	t.Parallel()
	ctx, abort := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		abort()
	}()

	done := make(chan error, 1)
	go func() {
		_, err := runLimited(t, ctx, `<?php
ignore_user_abort(true);
set_time_limit(1);
while (true) { $n = 1; }
`)
		done <- err
	}()

	select {
	case err := <-done:
		var limit *runner.TimeLimitError
		if !errors.As(err, &limit) {
			t.Fatalf("error = %v, want *runner.TimeLimitError", err)
		}
		if limit.Aborted {
			t.Error("reported as an abort; the script asked to ignore that")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the script outran its 1s limit after ignoring the abort")
	}
}

// TestTimeLimitDoesNotOutliveItsProgram covers a runtime a host reuses: the
// timer one program armed used to end the next one, which had asked for
// nothing.
func TestTimeLimitDoesNotOutliveItsProgram(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)

	first, err := rt.Load(`<?php set_time_limit(1); echo "one";`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(first); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if rt.TimeLimit() != 0 {
		t.Errorf("limit after the run = %s, want it cleared", rt.TimeLimit())
	}

	time.Sleep(1200 * time.Millisecond)

	out.Reset()
	second, err := rt.Load(`<?php echo "two";`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(second); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if out.String() != "two" {
		t.Errorf("output = %q, want %q", out.String(), "two")
	}
}

// TestDeadlineStillHoldsAfterAShutdownPass covers the other half of reuse: the
// shutdown pass turns the clock off so its callbacks can run, and used to leave
// it off, so every later program on that runtime was unbounded.
func TestDeadlineStillHoldsAfterAShutdownPass(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)

	first, err := rt.Load(`<?php register_shutdown_function(function () { echo "closed"; });`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(first); err != nil {
		t.Fatal(err)
	}
	if out.String() != "closed" {
		t.Fatalf("output = %q, want the callback to have run", out.String())
	}

	done := make(chan error, 1)
	go func() {
		second, loadErr := rt.Load(`<?php set_time_limit(1); while (true) { $n = 1; }`)
		if loadErr != nil {
			done <- loadErr
			return
		}
		done <- rt.Run(second)
	}()

	select {
	case err := <-done:
		var limit *runner.TimeLimitError
		if !errors.As(err, &limit) {
			t.Fatalf("error = %v, want *runner.TimeLimitError", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the second program was not bounded at all")
	}
}

// TestSetTimeLimitKeepsTheContextABindingHolds is the server case directly: the
// context is built once and a limit is a timer on it, because rebuilding it
// ends whatever is parked on it.
func TestSetTimeLimitKeepsTheContextABindingHolds(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)
	rt.SetContext(context.Background())

	parked := rt.Context()
	rt.SetTimeLimit(time.Hour)
	rt.SetTimeLimit(2 * time.Hour)
	rt.SetIgnoreUserAbort(true)

	if rt.Context() != parked {
		t.Error("the context was replaced")
	}
	if err := parked.Err(); err != nil {
		t.Errorf("the parked context was cancelled: %v", err)
	}
}

// TestTimeLimitOverflow keeps an absurd argument from wrapping into a short
// limit: 18446744074 seconds used to come out as 290ms.
func TestTimeLimitOverflow(t *testing.T) {
	t.Parallel()
	out, err := runLimited(t, context.Background(), `<?php
set_time_limit(18446744074);
for ($i = 0; $i < 100000; $i++) { $n = $i; }
echo "finished";
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "finished" {
		t.Errorf("output = %q, want %q", out, "finished")
	}
}

// TestIgnoreUserAbortReadsWithoutClearing covers the idiomatic guard. The
// binding took a required bool, so the no-argument spelling was zero-padded to
// false and turned off the thing it was asking about: the loop below would then
// be stopped by the disconnect the line above it had just said to ignore.
func TestIgnoreUserAbortReadsWithoutClearing(t *testing.T) {
	t.Parallel()
	ctx, abort := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		abort()
	}()

	out, err := runLimited(t, ctx, `<?php
ignore_user_abort(true);
ignore_user_abort();
$ticks = 0;
while ($ticks < 60) {
	usleep(5000);
	$ticks++;
}
echo "ran to the end";
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "ran to the end" {
		t.Errorf("output = %q, want the script to have finished past the abort", out)
	}
}
