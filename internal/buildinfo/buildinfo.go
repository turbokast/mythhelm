// Package buildinfo reports the identity of the running mythhelm binary.
package buildinfo

import (
	"runtime"
	"runtime/debug"
)

// Version and Commit are set at link time, for example:
//
//	go build -ldflags "-X github.com/turbokast/mythhelm/internal/buildinfo.Version=v0.1.0
//	  -X github.com/turbokast/mythhelm/internal/buildinfo.Commit=<sha>" ./cmd/mythhelm
//
// When they are empty, Get falls back to the module version and VCS
// revision that the go command embeds in the binary.
var (
	Version string
	Commit  string
)

// Info identifies a build. Version is "devel" and Commit is "unknown" when
// neither the linker nor the embedded build information supplies them.
type Info struct {
	Version   string
	Commit    string
	GoVersion string
	OS        string
	Arch      string
}

// Get returns the build information of the running binary.
func Get() Info {
	info := Info{Version: Version, Commit: Commit, GoVersion: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if info.Version == "" && bi.Main.Version != "(devel)" {
			info.Version = bi.Main.Version
		}
		if info.Commit == "" {
			info.Commit = vcsRevision(bi.Settings)
		}
	}
	if info.Version == "" {
		info.Version = "devel"
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	return info
}

func vcsRevision(settings []debug.BuildSetting) string {
	var revision, suffix string
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				suffix = "-dirty"
			}
		}
	}
	if revision == "" {
		return ""
	}
	return revision + suffix
}
