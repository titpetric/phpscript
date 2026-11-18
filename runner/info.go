package runner

import (
	"fmt"
	"strings"
)

// InfoField is a row of a phpinfo() section, printed as `Name => Value`.
type InfoField struct {
	Name  string
	Value string
}

// InfoSection reports the rows a subsystem contributes to phpinfo(), or none
// when it contributes no rows. It runs when a script calls phpinfo(). A section
// reports the current state. The state at registration never appears.
type InfoSection func(rt *Runtime) []InfoField

// RegisterInfo adds a named section to phpinfo(), in registration order.
func (rt *Runtime) RegisterInfo(name string, section InfoSection) {
	rt.infoSections = append(rt.infoSections, namedInfoSection{name: name, section: section})
}

// namedInfoSection is a registered section and the heading it prints under.
type namedInfoSection struct {
	name    string
	section InfoSection
}

// infoReport renders the registered sections, with a heading and rows for each.
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

// mib renders a byte count in MiB, php's memory unit.
func mib(bytes int64) string {
	return fmt.Sprintf("%.2f MiB", float64(bytes)/(1024*1024))
}

// registerCompiledTreeInfo adds the compiled source tree to phpinfo() under OPcache.
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
		// The size is the number a precompile pass measured itself adding.
		// When no pass ran, there is no figure. Printing 0.00 MiB for a cache holding
		// files would read as a measurement, not as the absence of one.
		if heap > 0 {
			fields = append(fields, InfoField{Name: "Cached Size", Value: mib(heap)})
		}
		return fields
	})
}
