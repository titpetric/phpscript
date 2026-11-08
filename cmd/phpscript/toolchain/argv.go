package toolchain

import (
	"path/filepath"
	"runtime/debug"
	"strings"
)

// CommandArgs puts the command named by a <binary>-<tool> argv[0] first.
func CommandArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	_, tool, found := strings.Cut(Binary(args[0]), "-")
	if !found || tool == "" || strings.Contains(tool, "-") {
		return args
	}
	if len(args) > 1 && args[1] == tool {
		return args
	}
	return append([]string{args[0], tool}, args[1:]...)
}

// CoverDir is cover/<binary>, where a coverage build writes by default.
func CoverDir(argv0 string) string {
	return filepath.Join("cover", Binary(argv0))
}

// BuiltWithCover reports whether go build -cover produced this binary.
func BuiltWithCover() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, setting := range info.Settings {
		if setting.Key == "-cover" {
			return setting.Value == "true"
		}
	}
	return false
}
