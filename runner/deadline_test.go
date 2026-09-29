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
	if got := err.Error(); got != "Maximum execution time of 1 seconds exceeded" {
		t.Errorf("message = %q", got)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Errorf("ran for %s, want about a second", elapsed)
	}
}

// TestTimeLimitSleepsAreCut holds the limit over a wait as well as over a
// loop: a sleep is on the runtime context, so it ends when the script does.
func TestTimeLimitSleepsAreCut(t *testing.T) {
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
