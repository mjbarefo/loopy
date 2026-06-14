package loop

import (
	"regexp"
	"runtime/debug"
)

// Version is overridden at release time via
// -ldflags "-X github.com/mjbarefo/loopy/internal/loop.Version=v0.x.y".
var Version = ""

// pseudoVersion matches Go's generated pseudo-versions so we fall through to
// the friendlier "dev-<sha>". All three forms end in a 14-digit UTC timestamp
// then a 12-char commit prefix; only the separator before the timestamp
// varies with the base tag: none (v0.0.0-<ts>-<sha>), a release base
// (v0.1.2-0.<ts>-<sha>), or a prerelease base (v0.1.0-rc.1.0.<ts>-<sha>) —
// so the timestamp is preceded by either '-' or '.'. Real tags never match.
var pseudoVersion = regexp.MustCompile(`[.-]\d{14}-[0-9a-f]{12}`)

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
