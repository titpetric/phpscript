package runner_test

import (
	"testing"

	yaml "github.com/goccy/go-yaml"

	"github.com/titpetric/phpscript/runner"
)

// TestParseSize covers the two spellings a size may be written in, and the
// rejection of the php.ini shorthands that are not among them: a size that
// cannot be read is an error rather than a guess.
func TestParseSize(t *testing.T) {
	valid := map[string]int64{
		"":         0,
		"0":        0,
		"512":      512,
		"8388608":  8388608,
		"1M":       1 << 20,
		"8M":       8 << 20,
		"2m":       2 << 20,
		"  8M  ":   8 << 20,
		"20000000": 20000000,
	}
	for text, want := range valid {
		got, err := runner.ParseSize(text)
		if err != nil {
			t.Errorf("ParseSize(%q): %v", text, err)
			continue
		}
		if got.Bytes() != want {
			t.Errorf("ParseSize(%q) = %d, want %d", text, got.Bytes(), want)
		}
	}

	for _, text := range []string{"8K", "2G", "1KB", "8MB", "M", "eight", "-1", "1.5M", "8 M"} {
		if got, err := runner.ParseSize(text); err == nil {
			t.Errorf("ParseSize(%q) = %d, want an error", text, got.Bytes())
		}
	}
}

// TestSizeExceeds pins the meaning of the zero value: no limit, as it is in
// php.ini, so nothing is ever over it.
func TestSizeExceeds(t *testing.T) {
	limit, err := runner.ParseSize("1M")
	if err != nil {
		t.Fatal(err)
	}
	if !limit.Exceeds(1<<20 + 1) {
		t.Error("1M does not exceed itself plus a byte")
	}
	if limit.Exceeds(1 << 20) {
		t.Error("1M exceeds exactly 1M")
	}
	var unlimited runner.Size
	if unlimited.Exceeds(1 << 40) {
		t.Error("the zero size is a limit")
	}
	// An unknown Content-Length is negative, and no limit can be over it.
	if limit.Exceeds(-1) {
		t.Error("a limit exceeds an unknown length")
	}
}

// TestSizeYAML covers reading a size out of a configuration file, where it is
// written the way php.ini writes it, and writing it back.
func TestSizeYAML(t *testing.T) {
	var opts runner.Options
	if err := yaml.Unmarshal([]byte("upload_max_filesize: 2M\npost_max_size: 8388608\n"), &opts); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := opts.UploadMaxFilesize.Bytes(); got != 2<<20 {
		t.Errorf("upload_max_filesize = %d, want %d", got, 2<<20)
	}
	if got := opts.PostMaxSize.Bytes(); got != 8388608 {
		t.Errorf("post_max_size = %d, want %d", got, 8388608)
	}
	if got := opts.UploadMaxFilesize.String(); got != "2M" {
		t.Errorf("String() = %q, want %q", got, "2M")
	}

	if err := yaml.Unmarshal([]byte("post_max_size: 8K\n"), &opts); err == nil {
		t.Error("a K suffix parsed, want an error")
	}
}
