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

	// UploadMaxFilesize is the largest file part a request may carry, PHP's
	// upload_max_filesize. A part over it is reported to the script as
	// UPLOAD_ERR_INI_SIZE and is not stored. Zero is no limit.
	UploadMaxFilesize Size `yaml:"upload_max_filesize"`

	// PostMaxSize is the largest request body that is parsed at all, PHP's
	// post_max_size. A body over it leaves both $_POST and $_FILES empty, as it
	// does in PHP. Zero is no limit.
	PostMaxSize Size `yaml:"post_max_size"`

	// UploadFileMode is the mode move_uploaded_file() gives a stored upload.
	// Zero means DefaultUploadFileMode; a host that serves uploads to nobody
	// but itself sets something tighter, 0600 or 0640.
	UploadFileMode FileMode `yaml:"upload_file_mode"`
}
