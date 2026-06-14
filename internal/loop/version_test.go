package loop

import "testing"

// TestPseudoVersion pins which version strings read as Go pseudo-versions (so
// ResolvedVersion skips them for the friendlier dev-<sha>) and which are real
// tags that must be shown verbatim. The tagged-base forms regressed once: a
// `go build`/`@main` install off a tagged repo stamps v0.1.0-rc.1.0.<ts>-<sha>,
// whose timestamp is preceded by '.', not '-'.
func TestPseudoVersion(t *testing.T) {
	pseudo := []string{
		"v0.0.0-20260611203006-fb77aa536254",        // no base tag
		"v0.1.2-0.20260614173513-51aa11e5977d",      // release base (patch bump)
		"v0.1.0-rc.1.0.20260614173513-51aa11e5977d", // prerelease base
		"v0.0.0-20260611203006-fb77aa536254+dirty",  // with a build suffix
	}
	for _, v := range pseudo {
		if !pseudoVersion.MatchString(v) {
			t.Errorf("%q should read as a pseudo-version", v)
		}
	}

	real := []string{"v0.1.0", "v0.1.0-rc.1", "v1.2.3", "v1.2.3-rc.2", ""}
	for _, v := range real {
		if pseudoVersion.MatchString(v) {
			t.Errorf("%q is a real version and must not match the pseudo-version pattern", v)
		}
	}
}
