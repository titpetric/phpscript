//go:build !windows

package toolchain

import (
	"os"
	"syscall"
)

// CoverRelaunch restarts a coverage build with GOCOVERDIR when the environment lacks it.
func CoverRelaunch() {
	if os.Getenv("GOCOVERDIR") != "" || !BuiltWithCover() {
		return
	}
	self, err := os.Executable()
	if err != nil {
		return
	}
	dir := CoverDir(os.Args[0])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = syscall.Exec(self, os.Args, append(os.Environ(), "GOCOVERDIR="+dir))
}
