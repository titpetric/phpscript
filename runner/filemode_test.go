package runner_test

import (
	"os"
	"testing"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/runner"
)

// TestParseFileMode covers the spellings a mode may be written in, all of them
// octal, and the rejection of anything that is not one.
func TestParseFileMode(t *testing.T) {
	t.Parallel()
	valid := map[string]uint32{
		"":      0,
		"0644":  0o644,
		"644":   0o644,
		"0600":  0o600,
		"0o640": 0o640,
		"1777":  0o1777,
		"0":     0,
	}
	for text, want := range valid {
		got, err := runner.ParseFileMode(text)
		if err != nil {
			t.Errorf("ParseFileMode(%q): %v", text, err)
			continue
		}
		if uint32(got) != want {
			t.Errorf("ParseFileMode(%q) = %o, want %o", text, uint32(got), want)
		}
	}

	for _, text := range []string{"0688", "rw-r--r--", "-1", "0.6", "10000", "0x1a"} {
		if got, err := runner.ParseFileMode(text); err == nil {
			t.Errorf("ParseFileMode(%q) = %o, want an error", text, uint32(got))
		}
	}
}

// TestFileModeMode pins the conversion to Go's representation, where the three
// special bits move out of the number and into their own flags.
func TestFileModeMode(t *testing.T) {
	t.Parallel()
	if got := runner.FileMode(0o640).Mode(); got != os.FileMode(0o640) {
		t.Errorf("0640 = %v, want %v", got, os.FileMode(0o640))
	}
	sticky := runner.FileMode(0o1777).Mode()
	if sticky&os.ModeSticky == 0 || sticky.Perm() != 0o777 {
		t.Errorf("1777 = %v, want sticky and 0777", sticky)
	}
	if got := runner.FileMode(0o4755).Mode(); got&os.ModeSetuid == 0 {
		t.Errorf("4755 = %v, want setuid", got)
	}
}

// TestFileModeYAML covers reading the mode out of a configuration file, where
// writing it as a bare number must not make it decimal.
func TestFileModeYAML(t *testing.T) {
	t.Parallel()
	var opts runner.Options
	if err := config.Unmarshal([]byte(`upload_file_mode: "0600"`+"\n"), &opts); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if opts.UploadFileMode != 0o600 {
		t.Errorf("upload_file_mode = %o, want %o", uint32(opts.UploadFileMode), 0o600)
	}
	if got := opts.UploadFileMode.String(); got != "0600" {
		t.Errorf("String() = %q, want %q", got, "0600")
	}

	if err := config.Unmarshal([]byte("upload_file_mode: 0640\n"), &opts); err != nil {
		t.Fatalf("unquoted: %v", err)
	}
	if opts.UploadFileMode != 0o640 {
		t.Errorf("unquoted upload_file_mode = %o, want %o", uint32(opts.UploadFileMode), 0o640)
	}

	if err := config.Unmarshal([]byte("upload_file_mode: 0688\n"), &opts); err == nil {
		t.Error("a non-octal mode parsed, want an error")
	}
}

// TestFileMode_UnmarshalYAML covers the decoder hook on its own. The quotes a
// document may carry are not part of the mode, and a mode that cannot be read
// leaves the field alone, so a bad document cannot make a stored upload 0000.
func TestFileMode_UnmarshalYAML(t *testing.T) {
	t.Parallel()
	for _, text := range []string{`"0640"`, "0640", " 0640 ", `'0640'`, "640", "0o640"} {
		var mode runner.FileMode
		if err := mode.UnmarshalYAML([]byte(text)); err != nil {
			t.Errorf("UnmarshalYAML(%q): %v", text, err)
			continue
		}
		if mode != 0o640 {
			t.Errorf("UnmarshalYAML(%q) = %o, want %o", text, uint32(mode), 0o640)
		}
	}

	mode := runner.FileMode(0o600)
	if err := mode.UnmarshalYAML([]byte("0688")); err == nil {
		t.Error("UnmarshalYAML(0688) = nil, want an error")
	}
	if mode != 0o600 {
		t.Errorf("a refused document left %o behind, want the 0600 the field held", uint32(mode))
	}
}

// TestFileMode_MarshalYAML covers the quoting the mode is written back under.
// A mode written as a bare number is read as decimal by a YAML parser, so an
// unquoted 0644 would come back as 1204 somewhere; the quotes are what keeps a
// round trip octal.
func TestFileMode_MarshalYAML(t *testing.T) {
	t.Parallel()
	for _, mode := range []runner.FileMode{0, 0o600, 0o644, 0o1777, 0o4755} {
		data, err := mode.MarshalYAML()
		if err != nil {
			t.Errorf("MarshalYAML(%o): %v", uint32(mode), err)
			continue
		}
		if want := `"` + mode.String() + `"`; string(data) != want {
			t.Errorf("MarshalYAML(%o) = %s, want %s", uint32(mode), data, want)
		}

		var back runner.FileMode
		if err := back.UnmarshalYAML(data); err != nil {
			t.Errorf("round trip of %o: %v", uint32(mode), err)
			continue
		}
		if back != mode {
			t.Errorf("round trip of %o = %o", uint32(mode), uint32(back))
		}
	}
}

// TestRuntimeUploadFileMode pins the default: an unset mode is 0644, not a
// mode of 0000 that would make a stored upload unreadable.
func TestRuntimeUploadFileMode(t *testing.T) {
	t.Parallel()
	rt := runner.New(nil, runner.Options{})
	if got := rt.UploadFileMode(); got != runner.DefaultUploadFileMode {
		t.Errorf("default = %v, want %v", got, runner.DefaultUploadFileMode)
	}
	rt = runner.New(nil, runner.Options{UploadFileMode: 0o600})
	if got := rt.UploadFileMode(); got != 0o600 {
		t.Errorf("configured = %v, want 0600", got)
	}
}
