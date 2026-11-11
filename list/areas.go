package list

import (
	"strconv"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// AreaRow is one binding area: the namespace that holds a group of functions,
// classes and constants, and what installing it defines.
type AreaRow struct {
	Area      string   `json:"area"`
	Layer     string   `json:"layer"` // "ps", "px" or "opt-in"
	Functions int      `json:"functions"`
	Classes   int      `json:"classes"`
	Constants int      `json:"constants"`
	Replaces  []string `json:"replaces,omitempty"`
}

// Areas lists the binding areas in install order, narrowed to the namespaces named.
func Areas(namespaces ...string) []AreaRow {
	rows := []AreaRow{}
	for _, info := range stdlib.Areas(stdlib.All) {
		if len(namespaces) > 0 && !inAny(info.Name, namespaces) {
			continue
		}
		rows = append(rows, AreaRow{
			Area:      info.Name,
			Layer:     layerOf(info),
			Functions: len(info.Funcs),
			Classes:   len(info.Classes),
			Constants: len(info.Constants),
			Replaces:  info.Replaced,
		})
	}
	return rows
}

// AreasMarkdown renders the areas as a markdown table.
func AreasMarkdown(rows []AreaRow) string {
	cells := make([][]string, 0, len(rows))
	for _, r := range rows {
		cells = append(cells, []string{r.Area, r.Layer, strconv.Itoa(r.Functions), strconv.Itoa(r.Classes), strconv.Itoa(r.Constants), strconv.Itoa(len(r.Replaces))})
	}
	return markdownTable([]string{"Area", "Layer", "Functions", "Classes", "Constants", "Replaces"}, cells)
}

func layerOf(info stdlib.AreaInfo) string {
	switch {
	case info.OptIn:
		return "opt-in"
	case runner.InNamespace(info.Name, runner.NamespacePX):
		return "px"
	}
	return "ps"
}

func inAny(name string, namespaces []string) bool {
	for _, ns := range namespaces {
		if runner.InNamespace(name, ns) {
			return true
		}
	}
	return false
}
