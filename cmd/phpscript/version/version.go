package version

import (
	"context"
	"debug/buildinfo"
	"fmt"
	"os"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/internal/flags"
)

// Info contains injected build environment information.
type Info struct {
	Version    string
	Commit     string
	CommitTime string
	Branch     string
}

// Name is the command title.
const Name = "Show version/build information"

// Build is what the linker wrote into the binary, set by main before any
// command is built.
//
// It is a package variable rather than a constructor argument because every
// command constructor takes the same two arguments, and build information is
// neither configuration nor a flag: it describes the binary rather than the
// tree it is pointed at or the run an operator asked for.
var Build Info

// NewCommand creates a new version command.
//
// The configuration and the global options are what every command constructor
// takes, so one signature covers all of them. This command reads neither.
func NewCommand(_ *config.Config, _ *flags.Options) *cli.Command {
	return &cli.Command{
		Name:  "version",
		Title: Name,
		Run: func(ctx context.Context, args []string) error {
			return Run(Build)
		},
	}
}

// Run will print version information for the build.
func Run(info Info) error {
	fmt.Printf("  Version:     %s\n", info.Version)

	// Print VCS information
	if info.Commit != "unknown" {
		shortCommit := info.Commit
		if len(info.Commit) > 12 {
			shortCommit = info.Commit[:12]
		}
		fmt.Printf("  Commit:      %s\n", shortCommit)
	}

	if info.CommitTime != "unknown" {
		fmt.Printf("  CommitTime:  %s\n", info.CommitTime)
	}

	if info.Branch != "unknown" {
		fmt.Printf("  Branch:      %s\n", info.Branch)
	}

	// Get build info from embedded metadata
	exePath, err := os.Executable()
	var bi *buildinfo.BuildInfo
	if err == nil {
		bi, _ = buildinfo.ReadFile(exePath)
	}

	if bi != nil {
		// Module information
		fmt.Printf("  Module:      %s\n", bi.Path)
		if bi.Main.Path != "" {
			fmt.Printf("  MainModule:  %s\n", bi.Main.Path)
		}
		if bi.Main.Sum != "" {
			fmt.Printf("  Sum:         %s\n", bi.Main.Sum)
		}

		// Go version
		if bi.GoVersion != "" {
			fmt.Printf("  GoVersion:   %s\n", bi.GoVersion)
		}

		// Extract OS/Arch from settings
		var osVal, archVal string
		for _, setting := range bi.Settings {
			switch setting.Key {
			case "GOOS":
				osVal = setting.Value
			case "GOARCH":
				archVal = setting.Value
			}
		}
		if osVal != "" && archVal != "" {
			fmt.Printf("  OS/Arch:     %s/%s\n", osVal, archVal)
		}

		// Print all build settings for reference
		if len(bi.Settings) > 0 {
			fmt.Printf("\n  Build Settings:\n")
			for _, setting := range bi.Settings {
				// Skip GOOS/GOARCH as we already printed them
				if setting.Key == "GOOS" || setting.Key == "GOARCH" {
					continue
				}
				fmt.Printf("    %s=%s\n", setting.Key, setting.Value)
			}
		}
	}
	return nil
}
