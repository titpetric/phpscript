package runner

import (
	"io"
	"io/fs"
)

// Options configures a Runtime.
type Options struct {
	// RootFS is the filesystem used to load PHP entrypoints and includes.
	RootFS fs.FS `yaml:"-"`

	// Stdin is exposed to scripts as the STDIN stream. A nil reader produces an
	// empty stream; CLI hosts should pass os.Stdin explicitly.
	Stdin io.Reader `yaml:"-"`

	// SAPI provides output for `php_sapi_name`.
	SAPI string `yaml:"-"`

	// WorkDir is the directory inside RootFS used as the script working directory.
	// Empty means the RootFS root.
	WorkDir string `yaml:"work_dir"`

	// WritablePaths optionally restricts filesystem writes. When empty, writes are
	// left to normal OS/user permissions. Enforcement is done by filesystem shims.
	WritablePaths []string `yaml:"writable_paths"`
}
