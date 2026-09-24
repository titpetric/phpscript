package runner

import (
	"fmt"
	"strings"
)

// InfoField is one row of a phpinfo() section, printed as `Name => Value`.
type InfoField struct {
	Name  string
	Value string
}

// InfoSection reports the rows a subsystem contributes to phpinfo(), or none
// when it has nothing to say. It runs at the moment phpinfo() is called, so a
// section reports what is there then rather than what was there at
// registration.
type InfoSection func(rt *Runtime) []InfoField

// RegisterInfo adds a named section to phpinfo(), after the runtime's own
// block. Sections print in registration order, and one reporting no rows is
// left out entirely rather than printed as a heading over nothing.
//
// It is how a binding answers for memory it holds across requests: a store
// bound into the context, a cache a precompile pass filled. None of that is in
// memory_get_usage(), which measures one script's live values, so phpinfo() is
// where the process-lifetime side is visible.
func (rt *Runtime) RegisterInfo(name string, section InfoSection) {
	rt.infoSections = append(rt.infoSections, namedInfoSection{name: name, section: section})
}

// namedInfoSection is a registered section and the heading it prints under.
type namedInfoSection struct {
	name    string
	section InfoSection
}

// infoReport renders the registered sections, each as a heading and its rows.
func (rt *Runtime) infoReport() string {
	var out strings.Builder
	for _, registered := range rt.infoSections {
		fields := registered.section(rt)
		if len(fields) == 0 {
			continue
		}
		fmt.Fprintf(&out, "\n%s\n\n", registered.name)
		for _, field := range fields {
			fmt.Fprintf(&out, "%s => %s\n", field.Name, field.Value)
		}
	}
	return out.String()
}

// mib renders a byte count in MiB, which is the unit the sizes reported here
// land in and the one php states a memory figure in.
func mib(bytes int64) string {
	return fmt.Sprintf("%.2f MiB", float64(bytes)/(1024*1024))
}

// registerCompiledTreeInfo reports the parsed and compiled source tree under
// phpinfo(). The name is phpscript's own: the mechanism is not opcache, there
// is no arena behind it, and none of the opcache_* functions are implemented.
func (rt *Runtime) registerCompiledTreeInfo() {
	rt.RegisterInfo("OPcache", func(rt *Runtime) []InfoField {
		programs, expressions, heap := rt.CompiledTree()
		if programs == 0 && expressions == 0 {
			return nil
		}
		fields := []InfoField{
			{Name: "Precompile", Value: fmt.Sprintf("%t", rt.Precompiled())},
			{Name: "Cached Files", Value: fmt.Sprintf("%d", programs)},
			{Name: "Compiled Expressions", Value: fmt.Sprintf("%d", expressions)},
		}
		// The size is what a precompile pass measured itself adding. Without
		// one there is no figure, and printing 0.00 MiB over a cache holding
		// files would read as a measurement rather than the absence of one.
		if heap > 0 {
			fields = append(fields, InfoField{Name: "Cached Size", Value: mib(heap)})
		}
		return fields
	})
}
