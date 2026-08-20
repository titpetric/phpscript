package runner_test

import (
	"os"
	"testing"

	yaml "github.com/goccy/go-yaml"

	"github.com/titpetric/phpscript/runner"
)

// TestParseFileMode covers the spellings a mode may be written in, all of them
// octal, and the rejection of anything that is not one.
func TestParseFileMode(t *testing.T) {
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
	var opts runner.Options
	if err := yaml.Unmarshal([]byte(`upload_file_mode: "0600"`+"\n"), &opts); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if opts.UploadFileMode != 0o600 {
		t.Errorf("upload_file_mode = %o, want %o", uint32(opts.UploadFileMode), 0o600)
	}
	if got := opts.UploadFileMode.String(); got != "0600" {
		t.Errorf("String() = %q, want %q", got, "0600")
	}

	if err := yaml.Unmarshal([]byte("upload_file_mode: 0640\n"), &opts); err != nil {
		t.Fatalf("unquoted: %v", err)
	}
	if opts.UploadFileMode != 0o640 {
		t.Errorf("unquoted upload_file_mode = %o, want %o", uint32(opts.UploadFileMode), 0o640)
	}

	if err := yaml.Unmarshal([]byte("upload_file_mode: 0688\n"), &opts); err == nil {
		t.Error("a non-octal mode parsed, want an error")
	}
}

// TestRuntimeUploadFileMode pins the default: an unset mode is 0644, not a
// mode of 0000 that would make a stored upload unreadable.
func TestRuntimeUploadFileMode(t *testing.T) {
	rt := runner.New(nil, runner.Options{})
	if got := rt.UploadFileMode(); got != runner.DefaultUploadFileMode {
		t.Errorf("default = %v, want %v", got, runner.DefaultUploadFileMode)
	}
	rt = runner.New(nil, runner.Options{UploadFileMode: 0o600})
	if got := rt.UploadFileMode(); got != 0o600 {
		t.Errorf("configured = %v, want 0600", got)
	}
}
