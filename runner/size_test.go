package runner_test

import (
	"testing"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/runner"
)

// TestParseSize covers the two spellings a size takes, and the rejection of
// the php.ini shorthands that are not among them: a size ParseSize cannot
// read is an error, and never a guess.
func TestParseSize(t *testing.T) {
	t.Parallel()
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

// TestSizeExceeds pins the meaning of the zero value: no limit, as in
// php.ini and nothing is ever over it.
func TestSizeExceeds(t *testing.T) {
	t.Parallel()
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

// TestSizeYAML covers reading a size out of a configuration file, spelled the
// way php.ini writes it, and writing it back.
func TestSizeYAML(t *testing.T) {
	t.Parallel()
	var opts runner.Options
	if err := config.Unmarshal([]byte("upload_max_filesize: 2M\npost_max_size: 8388608\n"), &opts); err != nil {
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

	if err := config.Unmarshal([]byte("post_max_size: 8K\n"), &opts); err == nil {
		t.Error("a K suffix parsed, want an error")
	}
}

// TestSize_UnmarshalYAML covers the decoder hook on its own. Quoted or bare is
// the same size, because a document naming 8M means the bytes either way, and a
// refused spelling leaves the field at the value it already held and never
// zeroes it and a bad document cannot read as "no limit".
func TestSize_UnmarshalYAML(t *testing.T) {
	t.Parallel()
	for _, text := range []string{`"8M"`, "8M", " 8M ", `'8M'`, "8388608"} {
		var size runner.Size
		if err := size.UnmarshalYAML([]byte(text)); err != nil {
			t.Errorf("UnmarshalYAML(%q): %v", text, err)
			continue
		}
		if got := size.Bytes(); got != 8<<20 {
			t.Errorf("UnmarshalYAML(%q) = %d, want %d", text, got, 8<<20)
		}
	}

	size := runner.Size(512)
	if err := size.UnmarshalYAML([]byte("8K")); err == nil {
		t.Error("UnmarshalYAML(8K) = nil, want an error")
	}
	if got := size.Bytes(); got != 512 {
		t.Errorf("a refused document left %d behind, want the 512 the field held", got)
	}
}

// TestSize_MarshalYAML covers the spelling MarshalYAML writes a size back in,
// which is the one it reads best in: a whole number of megabytes keeps the
// suffix and a configuration file a process rewrites still says 8M and not
// 8388608. Every spelling round trips to the same byte count.
func TestSize_MarshalYAML(t *testing.T) {
	t.Parallel()
	written := map[string]string{
		"8M":      "8M",
		"8388608": "8M",
		"512":     "512",
		"0":       "0",
		"1048577": "1048577",
	}
	for text, want := range written {
		size, err := runner.ParseSize(text)
		if err != nil {
			t.Fatal(err)
		}
		data, err := size.MarshalYAML()
		if err != nil {
			t.Errorf("MarshalYAML(%q): %v", text, err)
			continue
		}
		if got := string(data); got != want {
			t.Errorf("MarshalYAML(%q) = %q, want %q", text, got, want)
		}

		var back runner.Size
		if err := back.UnmarshalYAML(data); err != nil {
			t.Errorf("round trip of %q: %v", text, err)
			continue
		}
		if back != size {
			t.Errorf("round trip of %q = %d, want %d", text, back.Bytes(), size.Bytes())
		}
	}
}
