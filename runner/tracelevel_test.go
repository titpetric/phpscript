package runner

import (
	"strings"
	"testing"
)

// TestTraceLevel covers every level and the zero value, which is the default a
// configuration naming no level decodes to.
func TestTraceLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		level TraceLevel
		off   bool
		info  bool
		debug bool
	}{
		{level: "", info: true},
		{level: TraceLevelInfo, info: true},
		{level: TraceLevelDebug, debug: true},
		{level: TraceLevelOff, off: true},
	}
	for _, test := range tests {
		t.Run(string(test.level), func(t *testing.T) {
			t.Parallel()
			if got := test.level.Off(); got != test.off {
				t.Errorf("Off() = %v, want %v", got, test.off)
			}
			if got := test.level.Info(); got != test.info {
				t.Errorf("Info() = %v, want %v", got, test.info)
			}
			if got := test.level.Debug(); got != test.debug {
				t.Errorf("Debug() = %v, want %v", got, test.debug)
			}
			if err := test.level.Check(); err != nil {
				t.Errorf("Check() = %v, want a level the runtime accepts", err)
			}
		})
	}

	if got := TraceLevel("").Level(); got != TraceLevelInfo {
		t.Errorf("the zero value reads as %q, want %q", got, TraceLevelInfo)
	}
}

// TestTraceLevel_Check covers the refusal that keeps a misspelled level from
// reading as the default: Options.Check is the whole check over a runner
// block. The server runs it before it starts.
func TestTraceLevel_Check(t *testing.T) {
	t.Parallel()
	err := Options{TraceLevel: "verbose"}.Check()
	if err == nil {
		t.Fatal("Check() accepted a level the package does not define")
	}
	for _, want := range []string{"trace_level", `"verbose"`, "off", "info", "debug"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Check() = %q, want it to name %s", err, want)
		}
	}

	if err := (Options{}).Check(); err != nil {
		t.Errorf("an unconfigured runner block = %v, want it accepted", err)
	}
}
