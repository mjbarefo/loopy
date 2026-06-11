package loop

import (
	"regexp"
	"runtime/debug"
)

// Version is overridden at release time via
// -ldflags "-X github.com/mjbarefo/loopy/internal/loop.Version=v0.x.y".
var Version = ""

// pseudoVersion matches Go's generated pseudo-versions
// (v0.0.0-20260611203006-fb77aa536254[+dirty]) — building from a local git
// checkout stamps one, and "dev-<sha>" reads better for that case.
var pseudoVersion = regexp.MustCompile(`-\d{14}-[0-9a-f]{12}`)

// ResolvedVersion prefers the ldflags version, then the real module version
// that `go install …@vX.Y.Z` stamps, then VCS build info, then dev.
func ResolvedVersion() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" && !pseudoVersion.MatchString(v) {
			return v
		}
		var revision, modified string
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				modified = setting.Value
			}
		}
		if revision != "" {
			short := revision
			if len(short) > 12 {
				short = short[:12]
			}
			if modified == "true" {
				return "dev-" + short + "-dirty"
			}
			return "dev-" + short
		}
	}
	return "dev"
}
