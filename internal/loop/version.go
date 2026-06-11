package loop

import "runtime/debug"

// Version is overridden at release time via
// -ldflags "-X github.com/mjbarefo/loopy/internal/loop.Version=v0.x.y".
var Version = ""

// ResolvedVersion prefers the ldflags version, then VCS build info, then dev.
func ResolvedVersion() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
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
