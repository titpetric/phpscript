package runner

import (
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/stdlib/shared"
)

// The values Options.Runtime accepts.
const (
	RuntimePHPScript = "phpscript"
	RuntimeGo        = "go"
)

// Options configures a Runtime.
//
// Check refuses a value no runtime could accept, which is the trace level
// alone. The rest hold a value only Go can set, or a number where zero means
// the default. The level is the only key a configuration file spells as a
// word. A word that is not a level would otherwise read as the default.
type Options struct {
	// RootFS is the filesystem used to load PHP entrypoints and includes.
	RootFS fs.FS `yaml:"-"`

	// Database resolves the named connections the Database and
	// Database\Migrate bindings open. Nil leaves the choice to the binding,
	// which falls back to the process environment. A virtual host sets a
	// provider. The connections a site can name are the ones that host
	// configured.
	Database model.DatabaseProvider `yaml:"-"`

	// Mail resolves the named mail servers the Mail binding and mail()
	// send through. Nil leaves the choice to the binding, which falls back
	// to a provider holding no servers and mail() refuses catchably naming
	// what is missing. A virtual host sets a provider. The servers a site can
	// name are the ones that host configured.
	Mail model.MailProvider `yaml:"-"`

	// Stdin reaches scripts as the STDIN stream. A nil reader produces an
	// empty stream; CLI hosts should pass os.Stdin explicitly.
	Stdin io.Reader `yaml:"-"`

	// SAPI is the value `php_sapi_name` answers.
	SAPI string `yaml:"-"`

	// Runtime selects the binding namespace. phpscript, the default, installs
	// Areas. go installs Runtime\Go alone, with no area under Runtime\PS or
	// Runtime\PX. It parses every file as though it imported Runtime\Go.
	Runtime string `yaml:"runtime"`

	// Areas names the namespaces whose bindings the default runtime installs:
	// Runtime\PS, Runtime\PX, or the areas below them such as Runtime\PS\Core.
	// Empty installs every area a binding package contributed, in the order
	// PS then PX. The `runner.areas` key of the configuration sets it.
	Areas []string `yaml:"areas"`

	// WorkDir is the directory inside RootFS used as the script working directory.
	// Empty means the RootFS root.
	WorkDir string `yaml:"work_dir"`

	// Include names a file the runtime includes ahead of every entrypoint. It
	// holds the functions and classes an application needs whatever it runs: a
	// composer autoloader, a bootstrap file. The `--include` flag sets the
	// value. A virtual host sets the value in its phpscript.yml runner block.
	//
	// It runs once per request, before the entrypoint the request resolved to.
	// The entrypoint and everything it includes see its declarations. The
	// runtime skips a missing file without error. The name says what to load
	// when the application holds the file. One setting covers a server, a
	// script run, a fixture run of the same tree.
	Include string `yaml:"include"`

	// Precompile moves parsing and bytecode compilation off the request and
	// onto startup. A host walks its source tree through a Precompiler before
	// it serves anything. LoadFile reads an entrypoint back out of the include
	// cache. It does not parse the file again. Every request after the first
	// runs the AST that the caches hold.
	//
	// No call invalidates a cache entry. A reload or a restart takes up an
	// edited file. The parsed tree stays resident until then.
	Precompile bool `yaml:"precompile"`

	// TraceLevel is how much interpreter work the observers see: off, `info`
	// or debug. `info` is the default and the zero value.
	//
	// An interpreter reports work a host never would. A frame of PHP is a
	// span. A page that calls two hundred methods fills a trace with two
	// hundred spans of a microsecond each. The request they belong to is
	// unreadable. `info` publishes them marked as detail, leaving the recorder
	// the choice (oida drops a debug span that ran under
	// Options.DebugSpanMinDuration). It publishes one span at the end of the
	// request naming every file it included. debug publishes everything
	// unmarked, a span per include among them, for taking one request apart.
	// off publishes none of it. Measurements from a binding or a script still
	// record.
	TraceLevel TraceLevel `yaml:"trace_level"`

	// WritablePaths optionally restricts filesystem writes. When empty, normal
	// OS/user permissions govern a write. Filesystem shims enforce the list.
	WritablePaths []string `yaml:"writable_paths"`

	// UploadMaxFilesize is the largest file part a request may hold, PHP's
	// upload_max_filesize. A part over that size reaches the script as
	// UPLOAD_ERR_INI_SIZE, and nothing stores it. Zero is no limit.
	UploadMaxFilesize Size `yaml:"upload_max_filesize"`

	// PostMaxSize is the largest request body the runtime parses at all, PHP's
	// post_max_size. A body over the limit leaves both $_POST and $_FILES
	// empty, as in PHP. Zero is no limit.
	PostMaxSize Size `yaml:"post_max_size"`

	// MaxInputVars is how many fields the decoder puts in $_GET, $_POST and
	// $_COOKIE, PHP's max_input_vars. Zero is PHP's default of 1000; negative
	// is no limit.
	MaxInputVars int `yaml:"max_input_vars"`

	// MaxInputNestingLevel is the deepest bracket chain a field name may have,
	// PHP's max_input_nesting_level. The decoder drops a deeper field whole. Zero is
	// PHP's default of 64; negative is no limit.
	//
	// Both bound attacker-controlled input: `a[x][x]...` costs an array a level.
	MaxInputNestingLevel int `yaml:"max_input_nesting_level"`

	// UploadFileMode is the mode move_uploaded_file() sets on a stored upload.
	// Zero means DefaultUploadFileMode; a host that serves uploads to nobody
	// but itself sets something tighter, 0600 or 0640.
	UploadFileMode FileMode `yaml:"upload_file_mode"`

	// Env is the environment scripts read with getenv(). Nil means the
	// process environment, minus what ScriptEnvironment holds back. A host
	// that configured a separate env passes it here. Every virtual host always
	// does.
	Env []string `yaml:"-"`

	// MemoryLimit is the memory one script may take, PHP's memory_limit.
	// Zero is no limit.
	MemoryLimit Size `yaml:"memory_limit"`

	// TimeLimit is how long one script may run before the runtime stops it, in
	// seconds, PHP's max_execution_time. Zero is no limit.
	//
	// NOT ENFORCED YET. The decoder stores the key. A configuration written
	// today keeps working when enforcement lands. It does not fail to parse
	// along the way.
	TimeLimit int `yaml:"time_limit"`

	// ConcurrencyLimit is how many scripts may execute at once. Zero is no
	// limit. php.ini has no such key: the SAPI owns that number.
	// Here one process serves many sites. Each site takes a share.
	//
	// NOT ENFORCED YET. See TimeLimit.
	ConcurrencyLimit int `yaml:"concurrency_limit"`
}

// Check reports options a runtime could not run under.
func (o Options) Check() error {
	if err := o.TraceLevel.Check(); err != nil {
		return err
	}
	if _, err := o.runtimeMode(); err != nil {
		return err
	}
	if o.GoRuntime() && len(o.Areas) > 0 {
		return fmt.Errorf("runner.runtime: go selects Runtime\\Go; runner.areas names %v", o.Areas)
	}
	return CheckAreas(o.Areas)
}

// GoRuntime reports whether Runtime selects the go namespace.
func (o Options) GoRuntime() bool {
	mode, err := o.runtimeMode()
	return err == nil && mode == RuntimeGo
}

// SelectedAreas answers the namespaces the default runtime installs.
func (o Options) SelectedAreas() []string {
	if o.GoRuntime() {
		return []string{NamespaceGo}
	}
	return o.Areas
}

// runtimeMode folds Runtime into a mode name.
func (o Options) runtimeMode() (string, error) {
	switch strings.ToLower(strings.TrimSpace(o.Runtime)) {
	case "", RuntimePHPScript:
		return RuntimePHPScript, nil
	case RuntimeGo:
		return RuntimeGo, nil
	}
	return "", fmt.Errorf("runner.runtime: %q is not %q or %q", o.Runtime, RuntimePHPScript, RuntimeGo)
}

// inputLimits renders the two input limits as the form decoder takes them.
func (o Options) inputLimits() shared.Limits {
	return shared.Limits{
		MaxNesting: o.MaxInputNestingLevel,
		MaxVars:    o.MaxInputVars,
	}
}
