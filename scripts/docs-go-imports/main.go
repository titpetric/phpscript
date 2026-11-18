// Command docs-go-imports writes docs/reference/extensions/go-imports.md from
// the go runtime's binding inventory and its prose template. The inventory is
// the rows `phpscript list --runtime=go --bindings --json` prints, rendered by
// the same list.Stdlib call the command reads. Run it from the module root:
//
//	go run ./scripts/docs-go-imports > docs/reference/extensions/go-imports.md
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/titpetric/phpscript/list"
	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

const goRuntime = "go"

func main() {
	if err := generate(os.Stdout, "."); err != nil {
		fmt.Fprintln(os.Stderr, "docs-go-imports:", err)
		os.Exit(1)
	}
}

// generate writes the page for the tree at root: the template with the package
// table and per-area inventory in place of two markers.
func generate(out io.Writer, root string) error {
	tpl, err := os.ReadFile(filepath.Join(root, "docs/reference/extensions/go-imports.md.tpl"))
	if err != nil {
		return err
	}
	symbols := list.Stdlib(runner.Options{Runtime: goRuntime})
	g := newGenerator()

	doc := strings.Replace(string(tpl), "{{PACKAGES}}", g.packagesTable(symbols), 1)
	doc = strings.Replace(doc, "{{BINDINGS}}", g.bindingsSections(symbols), 1)
	_, err = io.WriteString(out, doc)
	return err
}

// areaGroup holds one area of the inventory plus the import path that reaches
// it, empty for names no import reaches.
type areaGroup struct {
	area    string
	path    string
	symbols []list.Symbol
}

// groups collects the symbols by area, importable areas first and in path
// order, then the areas no import reaches in area order.
func groups(symbols []list.Symbol) []areaGroup {
	paths := importPaths()
	byArea := map[string]*areaGroup{}
	for _, s := range symbols {
		// stdClass is a language class no area registers. The bindings listing
		// leaves it out.
		if s.Area == "" {
			continue
		}
		g, ok := byArea[s.Area]
		if !ok {
			g = &areaGroup{area: s.Area, path: paths[s.Area]}
			byArea[s.Area] = g
		}
		g.symbols = append(g.symbols, s)
	}

	out := make([]areaGroup, 0, len(byArea))
	for _, g := range byArea {
		sortSymbols(g.symbols)
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.path == "") != (b.path == "") {
			return a.path != ""
		}
		if a.path != b.path {
			return a.path < b.path
		}
		return a.area < b.area
	})
	return out
}

// importPaths maps each bound area back to the go path that reaches it.
func importPaths() map[string]string {
	out := map[string]string{}
	for _, path := range bindings.LinkedPackages() {
		if area, _, ok := modelutil.GoImportArea(path); ok {
			out[area] = path
		}
	}
	return out
}

// kindRank orders rows as a reader looks: calls, values, types, methods.
func kindRank(kind string) int {
	switch kind {
	case "function":
		return 0
	case "constant":
		return 1
	case "class":
		return 2
	}
	return 3
}

func sortSymbols(symbols []list.Symbol) {
	sort.Slice(symbols, func(i, j int) bool {
		a, b := symbols[i], symbols[j]
		if kindRank(a.Kind) != kindRank(b.Kind) {
			return kindRank(a.Kind) < kindRank(b.Kind)
		}
		return a.Name < b.Name
	})
}

// packagesTable renders each reachable area's page and import path.
func (g *generator) packagesTable(symbols []list.Symbol) string {
	rows := [][]string{}
	for _, group := range groups(symbols) {
		path := "-"
		if group.path != "" {
			path = "[" + "`" + group.path + "`" + "](" + packageURL(group.path) + ")"
		}
		rows = append(rows, []string{path, "`" + group.area + "`"})
	}
	return table([]string{"Import", "Area"}, rows)
}

// bindingsSections renders one table per area: every name it binds, with the
// signature a script types.
func (g *generator) bindingsSections(symbols []list.Symbol) string {
	var b strings.Builder
	for i, group := range groups(symbols) {
		if i > 0 {
			b.WriteString("\n")
		}
		heading := "### `" + group.area + "`"
		if group.path != "" {
			heading += " (`" + group.path + "`)"
		}
		b.WriteString(heading + "\n\n")
		rows := make([][]string, 0, len(group.symbols))
		for _, s := range group.symbols {
			name := short(group.area, s.Name)
			rows = append(rows, []string{
				s.Kind,
				g.named(group.area, s.Kind, name),
				"`" + cell(short(group.area, s.Signature)) + "`",
			})
		}
		b.WriteString(table([]string{"Kind", "Name", "Signature"}, rows))
	}
	return b.String()
}

// short drops the area prefix from a name or signature, which the section
// heading already names.
func short(area, text string) string {
	return strings.TrimPrefix(text, area+`\`)
}

// cell escapes what would end a table cell early.
func cell(text string) string {
	return strings.ReplaceAll(text, "|", `\|`)
}

// table renders a markdown table with the cells padded to their column.
func table(head []string, rows [][]string) string {
	widths := make([]int, len(head))
	for i, h := range head {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, c := range row {
			if len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}

	var b strings.Builder
	writeRow := func(row []string) {
		b.WriteString("| ")
		for i, c := range row {
			b.WriteString(c)
			b.WriteString(strings.Repeat(" ", widths[i]-len(c)))
			b.WriteString(" | ")
		}
		b.WriteString("\n")
	}
	writeRow(head)
	b.WriteString("|")
	for _, w := range widths {
		b.WriteString(" " + strings.Repeat("-", w) + " |")
	}
	b.WriteString("\n")
	for _, row := range rows {
		writeRow(row)
	}
	return b.String()
}
