package stdlib

import (
	"io"
	"os"

	"github.com/titpetric/phpscript/runner"
)

// ErrorsArea names the Error classes stdlib itself installs, and ExceptionsArea
// the Exception ones, both ahead of every binding package's area.
const (
	ErrorsArea     = runner.NamespacePS + `\Errors`
	ExceptionsArea = runner.NamespacePS + `\Exceptions`
)

func init() {
	runner.SetAreaMembers(areaMembers)
	runner.SetAreaClasses(areaClasses)
}

// areaMembers names the functions each default area defines or replaces, which
// is how `Runtime\PS\Core\Strings\strlen($s)` reaches the global strlen.
func areaMembers() map[string][]string {
	members := map[string][]string{}
	for _, info := range Areas(All) {
		if info.OptIn {
			continue
		}
		members[info.Name] = append(append(members[info.Name], info.Funcs...), info.Replaced...)
	}
	return members
}

// areaClasses names the classes each default area installs, which is how
// `new \Runtime\PS\Database()` reaches the registered Database.
func areaClasses() map[string][]string {
	classes := map[string][]string{}
	for _, info := range Areas(All) {
		if info.OptIn {
			continue
		}
		classes[info.Name] = append(classes[info.Name], info.Classes...)
	}
	return classes
}

// AreaInfo is one area of the runtime and what installing it defines.
type AreaInfo struct {
	// Name is the namespace, Runtime\PS\Core\Strings.
	Name string
	// OptIn is set for an area a script imports with `use`; the default
	// runtime does not carry it.
	OptIn bool
	// Profile is the mask the area mounts under.
	Profile runner.Profile
	// Delta is what the install defined and what it replaced.
	runner.Delta
}

// Areas installs each area on a runtime built here and reports what it defines and replaces.
func Areas(profile Profile, namespaces ...string) []AreaInfo {
	rt := runner.New(io.Discard, runner.Options{SAPI: "cli", RootFS: os.DirFS(".")})
	rt.SetProfile(profile)

	var infos []AreaInfo
	infos = append(infos, AreaInfo{Name: ErrorsArea, Profile: Secure, Delta: rt.Track(registerErrors)})
	infos = append(infos, AreaInfo{Name: ExceptionsArea, Profile: Secure, Delta: rt.Track(registerExceptions)})
	for _, area := range runner.SelectAreas(runner.AreasFor(profile), namespaces) {
		infos = append(infos, AreaInfo{Name: area.Name, Profile: area.Profile, Delta: rt.Track(area.Install)})
	}

	for _, name := range runner.Namespaces() {
		installer, _ := runner.NamespaceInstaller(name)
		fresh := runner.New(io.Discard, runner.Options{})
		infos = append(infos, AreaInfo{Name: name, OptIn: true, Profile: Secure, Delta: fresh.Track(installer)})
	}
	return infos
}
