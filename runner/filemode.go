package runner

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// FileMode is a Unix file mode written the way chmod writes it, in octal, with
// or without the leading zero: 0644 and 644 are the same mode. The value is
// octal whether it says so or not, because a mode is never anything else.
//
// The zero value means "not configured", which is a caller's cue to use its own
// default, and no mode of 0000.
type FileMode uint32

// DefaultUploadFileMode is the mode move_uploaded_file() sets on a stored upload
// when the configuration does not name one. The temporary copy an upload
// arrives in is private to this process, so the move has to apply a mode on the
// way out or nothing else could read what the script stored.
const DefaultUploadFileMode = FileMode(0o644)

// ParseFileMode reads an octal file mode. An empty value is the zero value.
func ParseFileMode(s string) (FileMode, error) {
	text := strings.TrimSpace(s)
	if text == "" || text == "null" {
		return 0, nil
	}
	// The 0o form is what YAML and newer PHP spell octal with; strip it and
	// parse the rest as the octal it already spells.
	text = strings.TrimPrefix(strings.TrimPrefix(text, "0o"), "0O")

	value, err := strconv.ParseUint(text, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("file mode %q: want octal permissions, such as 0644", s)
	}
	if value > 0o7777 {
		return 0, fmt.Errorf("file mode %q: want at most 7777", s)
	}
	return FileMode(value), nil
}

// Mode converts to the Go representation, where the three special bits live
// outside the permission bits and not above them.
func (m FileMode) Mode() os.FileMode {
	out := os.FileMode(m & 0o777)
	if m&0o4000 != 0 {
		out |= os.ModeSetuid
	}
	if m&0o2000 != 0 {
		out |= os.ModeSetgid
	}
	if m&0o1000 != 0 {
		out |= os.ModeSticky
	}
	return out
}

// String writes the mode as four octal digits.
func (m FileMode) String() string {
	return fmt.Sprintf("%04o", uint32(m))
}

// UnmarshalYAML reads a mode from a configuration file, spelled as octal,
// quoted or not.
func (m *FileMode) UnmarshalYAML(data []byte) error {
	mode, err := ParseFileMode(strings.Trim(strings.TrimSpace(string(data)), `"'`))
	if err != nil {
		return err
	}
	*m = mode
	return nil
}

// MarshalYAML writes the mode as a quoted octal string, never a decimal.
func (m FileMode) MarshalYAML() ([]byte, error) {
	return []byte(`"` + m.String() + `"`), nil
}
