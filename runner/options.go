package runner

import (
	"io"
	"io/fs"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/stdlib/shared"
)

// Options configures a Runtime.
type Options struct {
	// RootFS is the filesystem used to load PHP entrypoints and includes.
	RootFS fs.FS `yaml:"-"`

	// Database resolves the named connections the Database and
	// Database\Migrate bindings open. Nil leaves the choice to the binding,
	// which falls back to the process environment. A virtual host sets its
	// own, so that the connections a site can name are only the ones it
	// configured.
	Database model.DatabaseProvider `yaml:"-"`

	// Stdin is exposed to scripts as the STDIN stream. A nil reader produces an
	// empty stream; CLI hosts should pass os.Stdin explicitly.
	Stdin io.Reader `yaml:"-"`

	// SAPI provides output for `php_sapi_name`.
	SAPI string `yaml:"-"`

	// WorkDir is the directory inside RootFS used as the script working directory.
	// Empty means the RootFS root.
	WorkDir string `yaml:"work_dir"`

	// Autoload names the directory below the application root whose class
	// declarations load on first reference, without an include and without a
	// registered autoloader. A class resolves to the file its namespace spells:
	// `Acme\Thing` is `autoload/Acme/Thing.php`, and a class in no namespace is
	// `autoload/Bare.php`. Empty is DefaultAutoloadDir, so an embedding host
	// that configures nothing still gets the convention.
	//
	// There is no key that turns it off, because a directory that is not there
	// is already off: the lookup happens once, on a class reference that was
	// otherwise about to fail. Note this is the application root and not the
	// document root below it; a folder published over HTTP would serve its own
	// class files as text.
	Autoload string `yaml:"autoload"`

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

	// MaxInputVars is how many fields are decoded into $_GET, $_POST and
	// $_COOKIE, PHP's max_input_vars. Zero is PHP's default of 1000; negative
	// is no limit.
	MaxInputVars int `yaml:"max_input_vars"`

	// MaxInputNestingLevel is the deepest bracket chain a field name may have,
	// PHP's max_input_nesting_level. A field past it is dropped whole. Zero is
	// PHP's default of 64; negative is no limit.
	//
	// Both bound attacker-controlled input: `a[x][x]...` costs an array a level.
	MaxInputNestingLevel int `yaml:"max_input_nesting_level"`

	// UploadFileMode is the mode move_uploaded_file() gives a stored upload.
	// Zero means DefaultUploadFileMode; a host that serves uploads to nobody
	// but itself sets something tighter, 0600 or 0640.
	UploadFileMode FileMode `yaml:"upload_file_mode"`

	// Env is the environment scripts read with getenv(). Nil means the
	// process environment, minus what ScriptEnvironment holds back. A host
	// that configured an env of its own passes it here, and a virtual host
	// always does.
	Env []string `yaml:"-"`

	// MemoryLimit is the memory one script may allocate, PHP's memory_limit.
	// Zero is no limit.
	MemoryLimit Size `yaml:"memory_limit"`

	// TimeLimit is how long one script may run before it is stopped, in
	// seconds, PHP's max_execution_time. Zero is no limit.
	//
	// NOT ENFORCED YET. The key is accepted and carried so a configuration
	// written today keeps working when enforcement lands, rather than
	// failing to parse.
	TimeLimit int `yaml:"time_limit"`

	// ConcurrencyLimit is how many scripts may execute at once. Zero is no
	// limit. It has no equivalent in php.ini, where the SAPI owns it; here
	// one process serves several sites and each gets its own share.
	//
	// NOT ENFORCED YET. See TimeLimit.
	ConcurrencyLimit int `yaml:"concurrency_limit"`
}

// DefaultAutoloadDir is the directory Options.Autoload names when it is empty.
const DefaultAutoloadDir = "autoload"

// autoloadDir is the folder the class-name convention resolves against.
func (o Options) autoloadDir() string {
	if o.Autoload == "" {
		return DefaultAutoloadDir
	}
	return o.Autoload
}

// inputLimits renders the two input limits as the form decoder takes them.
// Zero means PHP's default on both sides, so an unconfigured Options decodes
// as PHP would.
func (o Options) inputLimits() shared.Limits {
	return shared.Limits{
		MaxNesting: o.MaxInputNestingLevel,
		MaxVars:    o.MaxInputVars,
	}
}
