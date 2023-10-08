package main

import (
	"fmt"
	"runtime/debug"
)

// ServerVersion defines the server application version.
// Use -ldflags "-X main.ServerVersion=1.0.0" to override the version.
var ServerVersion string

func loadServerVersionFromBuildInfo() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	var (
		commit string = "unknown"
		dirty  bool
	)
	for _, s := range info.Settings {
		switch {
		case s.Key == "vcs.revision":
			commit = s.Value
			if len(s.Value) > 10 {
				commit = commit[:10]
			}
		case s.Key == "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if dirty {
		commit += "-dirty"
	}

	s := fmt.Sprintf("sqlite-rest/%s (%s, commit/%s)", info.Main.Version, info.GoVersion, commit)

	return s
}

func setServerVersion() {
	if ServerVersion != "" {
		return
	}

	if v := loadServerVersionFromBuildInfo(); v != "" {
		ServerVersion = v
		return
	}

	ServerVersion = "(devel)"
}

func init() {
	setServerVersion()
}
