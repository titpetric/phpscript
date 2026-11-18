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

// areaMembers names the functions each default area defines or replaces.
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

// areaClasses names the classes each default area installs.
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

// AreaInfo describes an area of the runtime and its installation's definitions.
type AreaInfo struct {
	// Name is the namespace, Runtime\PS\Core\Strings.
	Name string
	// OptIn marks an area a script imports with `use`, one the default
	// runtime leaves out.
	OptIn bool
	// Profile is the mask the area mounts under.
	Profile runner.Profile
	// Delta holds the definitions and replacements of the install.
	runner.Delta
}

// Areas installs each area on a local runtime, reporting definitions and replacements.
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
