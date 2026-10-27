package tests

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	yaml "github.com/goccy/go-yaml"

	"github.com/titpetric/phpscript/config"
)

//go:embed all:fixtures
var fixturesFS embed.FS

var phpFS fs.FS

var phpFSOnce sync.Once

func testPHPFS() fs.FS {
	phpFSOnce.Do(func() {
		var err error
		phpFS, err = fs.Sub(fixturesFS, "fixtures")
		if err != nil {
			panic(err)
		}
	})
	return phpFS
}

// fixtureArea is the fixtures of one area directory, in discovery order, with
// the suite governing them where the directory holds a phpscript.yml.
type fixtureArea struct {
	Name     string
	Fixtures []*Fixture
	Suite    *Suite
}

// isFixturePath reports whether a file is a fixture: the .phpt document, or a
// _test.php body with its expected output beside it. A _test.php with no .txt
// is a php file someone put in the tree, and no fixture.
func isFixturePath(name string) bool {
	if strings.HasSuffix(name, ".phpt") {
		return true
	}
	if !strings.HasSuffix(name, GoldenSuffix) {
		return false
	}
	_, err := os.Stat(goldenOutput(name))
	return err == nil
}

// embeddedFixtures walks the embedded tree and groups every .phpt by the area
// directory holding it. A fixture's include root is that directory, which is
// also where the php runner executes and all three runners resolve a relative
// include to the same file.
//
// LoadSuiteFS reads an area holding a phpscript.yml as a suite and the Go tests
// run under the connections and the prelude the same folder passes to the
// command. Discovery is per area and not per ancestor: the walk starts at
// tests/fixtures and no invocation root sits above it to walk up to.
func embeddedFixtures() ([]fixtureArea, error) {
	index := map[string]int{}
	var areas []fixtureArea

	err := fs.WalkDir(fixturesFS, "fixtures", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		golden := strings.HasSuffix(p, GoldenSuffix)
		if !golden && !strings.HasSuffix(p, ".phpt") {
			return nil
		}

		data, err := fixturesFS.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		var fx *Fixture
		if golden {
			expected, err := fixturesFS.ReadFile(goldenOutput(p))
			if err != nil {
				// No expected output beside the body means a php file in
				// the tree, and no fixture.
				return nil
			}
			fx = ParseGolden(data, expected, p)
		} else if fx, err = ParseFixture(data, p); err != nil {
			return fmt.Errorf("parse %s: %w", p, err)
		}

		dir := path.Dir(p)
		root, err := fs.Sub(fixturesFS, dir)
		if err != nil {
			return fmt.Errorf("sub %s: %w", dir, err)
		}
		fx.SetRootFS(root)

		name := path.Base(dir)
		if _, ok := index[name]; !ok {
			suite, err := LoadSuiteFS(fixturesFS, dir, config.New())
			if err != nil {
				return fmt.Errorf("%s: %w", dir, err)
			}
			index[name] = len(areas)
			areas = append(areas, fixtureArea{Name: name, Suite: suite})
		}
		at := index[name]
		if suite := areas[at].Suite; suite != nil {
			// A suite that declares no prelude leaves the app root unset:
			// the area's own directory is the fixture's include root
			// already, and naming it twice would layer the same filesystem
			// over itself. A suite that declares one names that directory as
			// the app root, and the prelude resolves there.
			fx.SetDatabase(suite.Provider())
			fx.SetMail(suite.Mail())
			if include := suite.Include(); include != "" {
				fx.SetAppRoot(dir, include)
			}
		}
		areas[at].Fixtures = append(areas[at].Fixtures, fx)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, area := range areas {
		ShareSources(area.Fixtures)
	}
	return areas, nil
}

// setupAreas runs the setup hook of every area that has one, and returns the
// teardown the caller defers. The Go tests take the same path a command run
// does: a suite lays its state down once, before the fixtures that read it.
func setupAreas(ctx context.Context, areas []fixtureArea, out io.Writer) (func() error, error) {
	suites := make([]*Suite, 0, len(areas))
	for _, area := range areas {
		if area.Suite != nil {
			suites = append(suites, area.Suite)
		}
	}
	if err := RunSetup(ctx, suites, out); err != nil {
		return func() error { return nil }, err
	}
	return func() error { return RunTeardown(ctx, suites, out) }, nil
}

func errorChainContains(err error, substr string) bool {
	for err != nil {
		if strings.Contains(err.Error(), substr) {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}

// ParseFixture splits a .phpt file into its three sections and parses the YAML metadata.
// GoldenSuffix and GoldenOutputSuffix name the second fixture form: a php file
// the runtimes execute, and the output beside it they must match.
//
// The body is an ordinary file and no section of a document and php runs it
// where it lies and __FILE__ and __DIR__ read back its own path.
// A person can also run the body by hand, which a .phpt cannot. The form
// carries no frontmatter: a fixture needing one declares a .phpt.
const (
	GoldenSuffix       = "_test.php"
	GoldenOutputSuffix = "_test.txt"
)

// ParseGolden builds a fixture from a _test.php body and the _test.txt beside
// it. It takes both as they are, with no section to parse out of either.
func ParseGolden(body, expected []byte, path string) *Fixture {
	return &Fixture{
		Name:     strings.TrimSuffix(filepath.Base(path), GoldenSuffix),
		PHP:      strings.ReplaceAll(string(body), "\r\n", "\n"),
		Expected: strings.TrimRight(strings.ReplaceAll(string(expected), "\r\n", "\n"), "\n"),
		Path:     path,
	}
}

// goldenOutput names the expected-output file beside a _test.php body.
func goldenOutput(path string) string {
	return strings.TrimSuffix(path, GoldenSuffix) + GoldenOutputSuffix
}

// unmarshalHeader reads the yaml header of a .phpt document into target.
//
// This package reads every header through this one door and a fixture and a
// test of the parser agree on what a key means and on which library decided.
func unmarshalHeader(data []byte, target any) error {
	return yaml.Unmarshal(data, target)
}

// ParseFixture reads a .phpt document: the yaml header, the php body and the
// expected output, separated by the two --- lines. The optional path is what the
// fixture reports itself as, since the bytes carry no name.
func ParseFixture(data []byte, path ...string) (*Fixture, error) {
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	parts := strings.SplitN(normalized, "\n---\n", 3)
	if len(parts) != 3 {
		return nil, errors.New("malformed .phpt: want <yaml>---<php>---<output>")
	}

	var f Fixture
	if err := unmarshalHeader([]byte(parts[0]), &f); err != nil {
		return nil, fmt.Errorf("yaml metadata unmarshal error: %w", err)
	}

	if f.Name == "" {
		return nil, errors.New("fixture metadata missing required 'name'")
	}
	if f.Description == "" {
		return nil, errors.New("fixture metadata missing required 'description'")
	}

	f.PHP = strings.TrimPrefix(parts[1], "\n")
	f.Expected = parts[2]
	if len(path) > 0 {
		f.Path = path[0]
	}

	return &f, nil
}

// FindFixtures locates all .phpt files in the specified paths (files or directories).
func FindFixtures(paths []string) ([]*Fixture, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}

	var fixtures []*Fixture
	seen := make(map[string]bool)

	for _, p := range paths {
		p = filepath.Clean(p)
		recursive := false
		if strings.HasSuffix(p, "/...") || strings.HasSuffix(p, string(filepath.Separator)+"...") || p == "..." {
			recursive = true
			p = strings.TrimSuffix(p, "...")
			p = strings.TrimSuffix(p, string(filepath.Separator))
			if p == "" {
				p = "."
			}
		}

		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}

		if !info.IsDir() {
			if strings.HasSuffix(p, ".phpt") && !seen[p] {
				seen[p] = true
				fx, err := loadFixtureFile(p)
				if err != nil {
					return nil, err
				}
				fixtures = append(fixtures, fx)
			}
			continue
		}

		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != p && !recursive {
					return filepath.SkipDir
				}
				return nil
			}
			if isFixturePath(path) && !seen[path] {
				seen[path] = true
				fx, err := loadFixtureFile(path)
				if err != nil {
					return err
				}
				fixtures = append(fixtures, fx)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", p, err)
		}
	}

	sort.Slice(fixtures, func(i, j int) bool {
		return fixtures[i].Path < fixtures[j].Path
	})
	ShareSources(fixtures)

	return fixtures, nil
}

func loadFixtureFile(name string) (*Fixture, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	var fx *Fixture
	if strings.HasSuffix(name, GoldenSuffix) {
		expected, err := os.ReadFile(goldenOutput(name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", goldenOutput(name), err)
		}
		fx = ParseGolden(data, expected, name)
	} else {
		fx, err = ParseFixture(data, name)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
	}
	fx.SetRootFS(os.DirFS(filepath.Dir(name)))
	return fx, nil
}

func parseArgs(raw any, path string) []string {
	scriptName := "phpscript"
	if path != "" {
		scriptName = filepath.Base(path)
	}
	argv := []string{scriptName}

	switch v := raw.(type) {
	case []any:
		for _, arg := range v {
			argv = append(argv, fmt.Sprint(arg))
		}
	case map[string]any:
		// Map representation e.g. args: {name: Alice} -> ["script", "Alice"] or values sorted by key
		var keys []string
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			argv = append(argv, fmt.Sprint(v[k]))
		}
	case string:
		argv = append(argv, v)
	default:
		rv := reflect.ValueOf(raw)
		if rv.Kind() == reflect.Slice {
			for i := 0; i < rv.Len(); i++ {
				argv = append(argv, fmt.Sprint(rv.Index(i).Interface()))
			}
		}
	}
	return argv
}
