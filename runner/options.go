package runner

import "io/fs"

// Options configures a Runtime.
type Options struct {
	// RootFS is the filesystem used to load PHP entrypoints and includes.
	RootFS fs.FS

	// SAPI provides output for `php_sapi_name`.
	SAPI string

	// WorkDir is the directory inside RootFS used as the script working directory.
	// Empty means the RootFS root.
	WorkDir string

	// WritablePaths optionally restricts filesystem writes. When empty, writes are
	// left to normal OS/user permissions. Enforcement is done by filesystem shims.
	WritablePaths []string
}
