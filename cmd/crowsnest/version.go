package main

import (
	"runtime/debug"
	"strings"
)

// version is set at build time (-ldflags "-X main.version=1.2.3"); the release image gets the release number.
// A plain `go build` leaves it "dev", and versionString then adds the commit Go recorded, when there is one.
var version = "dev"

// versionString is what `crowsnest version` prints and the startup log line shows.
func versionString() string {
	v := strings.TrimPrefix(version, "v")
	if v != "dev" {
		return v
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				return "dev-" + s.Value[:7]
			}
		}
	}
	return v
}
