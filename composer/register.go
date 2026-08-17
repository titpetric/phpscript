package composer

import (
	"io/fs"

	"github.com/titpetric/phpscript/runner"
)

// Register discovers the composer project covering dir and binds its
// vendor/autoload.php so that including it wires the project's autoload
// metadata into rt.
//
// Nothing is loaded until a script includes the autoloader, which is how
// composer projects behave under stock PHP: bootstrap.php includes
// vendor/autoload.php, everything reached from there can name vendor classes,
// and a script that never includes it sees no vendor classes at all.
//
// It is a no-op when no composer.json is found, or when composer has not
// generated its autoloader yet, so every host can call it unconditionally.
// Only a malformed composer.json is reported as an error.
func Register(rt *runner.Runtime, fsys fs.FS, dir string) error {
	if fsys == nil {
		return nil
	}
	project, ok, err := Discover(fsys, dir)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	project.Register(rt, fsys)
	return nil
}

// Register binds this project's autoloader onto rt. Including
// vendor/autoload.php runs the Go implementation instead of parsing composer's
// generated bootstrap, which relies on PHP features the interpreter does not
// provide.
//
// The binding is skipped when composer has not generated the file: PHP would
// fail on the missing include, and so should phpscript.
func (p *Project) Register(rt *runner.Runtime, fsys fs.FS) {
	autoloadFile := p.AutoloadFile()
	if _, err := fs.Stat(fsys, autoloadFile); err != nil {
		return
	}
	installed := false
	rt.RegisterInclude(autoloadFile, func() (any, error) {
		// composer's autoload.php is include-safe: it hands back the same
		// loader on a second include rather than registering another one.
		if installed {
			return int64(1), nil
		}
		installed = true
		if err := p.install(rt, fsys); err != nil {
			return nil, err
		}
		return int64(1), nil
	})
}

// install loads the project's autoload.files and appends its PSR-4/PSR-0
// autoloader to the SPL queue.
func (p *Project) install(rt *runner.Runtime, fsys fs.FS) error {
	for _, file := range p.Files {
		if _, err := fs.Stat(fsys, file); err != nil {
			continue
		}
		if _, err := rt.IncludeFile(file); err != nil {
			return err
		}
	}
	if len(p.PSR4) == 0 && len(p.PSR0) == 0 {
		return nil
	}
	rt.RegisterAutoloader(func(class string) error {
		for _, candidate := range p.Resolve(class) {
			if _, err := fs.Stat(fsys, candidate); err != nil {
				continue
			}
			_, err := rt.IncludeFile(candidate)
			return err
		}
		// A class this project does not own is not an error: the next
		// autoloader in the queue gets its turn.
		return nil
	}, false)
	return nil
}
