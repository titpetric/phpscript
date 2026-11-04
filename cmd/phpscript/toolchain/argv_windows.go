//go:build windows

package toolchain

// CoverRelaunch does nothing on Windows, which has no exec to replace the process.
func CoverRelaunch() {}
