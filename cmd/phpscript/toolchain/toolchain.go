// Package toolchain discovers external tools named <binary>-<tool> on PATH.
//
// The prefix is the base name of the running binary, so phpscript finds
// phpscript-gen and a renamed copy called phpx finds phpx-gen. A binary never
// picks up the tools of another prefix: phpscript ignores phpx-gen, and phpx
// ignores phpscript-gen. docs/cli/README.md carries the rule.
package toolchain

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Tool is one executable found on PATH.
type Tool struct {
	// Name is what follows the prefix: gen for phpscript-gen.
	Name string
	// Path is the executable's location.
	Path string
}

var (
	mu    sync.Mutex
	cache = map[string][]Tool{}
	helps = map[string]string{}
)

// Binary returns the name a binary runs as, from its argv[0].
func Binary(argv0 string) string {
	return strings.TrimSuffix(filepath.Base(argv0), ".exe")
}

// Discover lists the <binary>-<tool> executables of pathEnv, memoized.
func Discover(binary, pathEnv string) []Tool {
	key := binary + "\x00" + pathEnv
	mu.Lock()
	defer mu.Unlock()
	if tools, ok := cache[key]; ok {
		return tools
	}
	tools := scan(binary, pathEnv)
	cache[key] = tools
	return tools
}

func scan(binary, pathEnv string) []Tool {
	prefix := binary + "-"
	seen := map[string]bool{}
	var tools []Tool
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := strings.TrimSuffix(entry.Name(), ".exe")
			if !strings.HasPrefix(name, prefix) || len(name) == len(prefix) || seen[name] {
				continue
			}
			full := filepath.Join(dir, entry.Name())
			info, err := os.Stat(full)
			if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
				continue
			}
			seen[name] = true
			tools = append(tools, Tool{Name: name[len(prefix):], Path: full})
		}
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools
}

// Find returns the tool called name.
func Find(tools []Tool, name string) (Tool, bool) {
	for _, tool := range tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}

// Sanitize rewrites <binary>-<tool> to "<binary> <tool>" for every tool given.
func Sanitize(text, binary string, tools []Tool) string {
	byLength := append([]Tool{}, tools...)
	sort.Slice(byLength, func(i, j int) bool { return len(byLength[i].Name) > len(byLength[j].Name) })
	pairs := make([]string, 0, 2*len(byLength))
	for _, tool := range byLength {
		pairs = append(pairs, binary+"-"+tool.Name, binary+" "+tool.Name)
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

// Help returns the sanitized --help output of the tool, memoized per path.
func Help(binary string, tool Tool, tools []Tool) string {
	mu.Lock()
	defer mu.Unlock()
	key := binary + "\x00" + tool.Path
	if text, ok := helps[key]; ok {
		return text
	}
	out, _ := exec.Command(tool.Path, "--help").CombinedOutput()
	text := strings.TrimSpace(Sanitize(string(out), binary, tools))
	helps[key] = text
	return text
}

// Title returns the first line of the tool's help, or a stock line when the
// tool printed nothing.
func Title(binary string, tool Tool, tools []Tool) string {
	first, _, _ := strings.Cut(Help(binary, tool, tools), "\n")
	if first = strings.TrimSpace(first); first != "" {
		return first
	}
	return "External tool " + filepath.Base(tool.Path)
}

// Exec runs the tool with args, attached to the process's standard streams,
// and returns its exit code.
func Exec(tool Tool, args []string) int {
	cmd := exec.Command(tool.Path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		return 127
	}
	return 0
}
