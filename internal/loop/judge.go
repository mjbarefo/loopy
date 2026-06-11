package loop

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// The judge is the ported crux council ranking: deterministic, evidence-based
// comparison of competing loops. No model calls, no API key — it ranks what
// is on disk and it is allowed to conclude that nothing is safe to take.

// manifestNames are dependency manifests; a diff that touches one changes
// what the project depends on, which no automated ranking should wave
// through.
var manifestNames = map[string]bool{
	"go.mod": true, "go.sum": true,
	"package.json": true, "package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"cargo.toml": true, "cargo.lock": true,
	"requirements.txt": true, "pyproject.toml": true, "poetry.lock": true, "uv.lock": true,
	"gemfile": true, "gemfile.lock": true,
	"composer.json": true, "composer.lock": true,
}

// Candidate is one loop as the judge saw it, with every flag it raised.
type Candidate struct {
	LoopID       string   `json:"loop_id"`
	Agent        string   `json:"agent"`
	Status       string   `json:"status"`
	Green        bool     `json:"green"`
	Iterations   int      `json:"iterations"`
	WallClock    string   `json:"wall_clock"`
	DiffBytes    int      `json:"diff_bytes"`
	FilesChanged int      `json:"files_changed"`
	Manifests    []string `json:"manifest_changes,omitempty"`
	Notes        []string `json:"notes,omitempty"`

	changedFiles []string
}

// Overlap records files touched by two competing green loops — applying both
// would collide; the flag is informational for the human picking one.
type Overlap struct {
	A     string   `json:"a"`
	B     string   `json:"b"`
	Files []string `json:"files"`
}

// Verdict is the judge's full output. Winner is empty when nothing is safe
// to take — a legitimate result, not a failure.
type Verdict struct {
	Winner     string      `json:"winner,omitempty"`
	Reason     string      `json:"reason"`
	Candidates []Candidate `json:"candidates"` // ranked: best first
	Overlaps   []Overlap   `json:"overlaps,omitempty"`
}

// Judge ranks competing loops by their recorded evidence. The order is
// total and deterministic: green before red; among green, manifest-free
// before manifest-touching, then fewer files, smaller diff, fewer
// iterations, less wall clock, and finally loop ID.
func Judge(root string, loopIDs []string) (Verdict, error) {
	if len(loopIDs) < 2 {
		return Verdict{}, fmt.Errorf("the judge compares loops: give it at least 2, got %d", len(loopIDs))
	}
	candidates := make([]Candidate, 0, len(loopIDs))
	for _, id := range loopIDs {
		l, err := LoadLoop(root, id)
		if err != nil {
			return Verdict{}, err
		}
		if !l.Done() && l.Status != StatusPaused {
			return Verdict{}, fmt.Errorf("loop %s is still %s: the judge only ranks finished loops", l.ID, l.Status)
		}
		c, err := buildCandidate(root, l)
		if err != nil {
			return Verdict{}, err
		}
		candidates = append(candidates, c)
	}

	sort.SliceStable(candidates, func(i, j int) bool { return rankLess(candidates[i], candidates[j]) })

	verdict := Verdict{
		Candidates: candidates,
		Overlaps:   findOverlaps(candidates),
	}
	verdict.Winner, verdict.Reason = decide(candidates)
	return verdict, nil
}

func buildCandidate(root string, l Loop) (Candidate, error) {
	iterations, err := LoadIterations(root, l.ID)
	if err != nil {
		return Candidate{}, err
	}
	c := Candidate{
		LoopID:     l.ID,
		Agent:      l.Agent,
		Status:     l.Status,
		Green:      l.Status == StatusGreen || l.Status == StatusAccepted,
		Iterations: l.IterationsUsed,
		WallClock:  Duration(l.WallClockUsed).String(),
	}
	if it, ok := lastChanged(iterations); ok {
		c.DiffBytes = it.DiffBytes
		c.FilesChanged = len(it.ChangedFiles)
		c.changedFiles = it.ChangedFiles
		for _, f := range it.ChangedFiles {
			if manifestNames[strings.ToLower(path.Base(f))] {
				c.Manifests = append(c.Manifests, f)
			}
		}
	}
	for _, it := range iterations {
		if it.Violation != "" {
			c.Notes = append(c.Notes, fmt.Sprintf("iteration %d violated forbidden paths", it.Index))
		}
	}
	if !c.Green {
		c.Notes = append(c.Notes, "not green: "+l.ParkedReason)
	}
	if c.Green && c.DiffBytes == 0 {
		c.Notes = append(c.Notes, "green with an empty diff (nothing to apply)")
	}
	if len(c.Manifests) > 0 {
		c.Notes = append(c.Notes, "touches dependency manifests: "+strings.Join(c.Manifests, ", "))
	}
	return c, nil
}

func lastChanged(iterations []Iteration) (Iteration, bool) {
	for i := len(iterations) - 1; i >= 0; i-- {
		if iterations[i].DiffBytes > 0 {
			return iterations[i], true
		}
	}
	return Iteration{}, false
}

// rankLess is the council's total order: every comparison is a recorded,
// explainable preference for the smaller, cleaner, faster change.
func rankLess(a, b Candidate) bool {
	if a.Green != b.Green {
		return a.Green
	}
	if (a.DiffBytes > 0) != (b.DiffBytes > 0) {
		return a.DiffBytes > 0 // an applicable diff beats an empty one
	}
	if (len(a.Manifests) == 0) != (len(b.Manifests) == 0) {
		return len(a.Manifests) == 0
	}
	if a.FilesChanged != b.FilesChanged {
		return a.FilesChanged < b.FilesChanged
	}
	if a.DiffBytes != b.DiffBytes {
		return a.DiffBytes < b.DiffBytes
	}
	if a.Iterations != b.Iterations {
		return a.Iterations < b.Iterations
	}
	if a.WallClock != b.WallClock {
		return a.WallClock < b.WallClock
	}
	return a.LoopID < b.LoopID
}

func findOverlaps(candidates []Candidate) []Overlap {
	var overlaps []Overlap
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			a, b := candidates[i], candidates[j]
			if !a.Green || !b.Green {
				continue
			}
			shared := intersect(a.changedFiles, b.changedFiles)
			if len(shared) > 0 {
				overlaps = append(overlaps, Overlap{A: a.LoopID, B: b.LoopID, Files: shared})
			}
		}
	}
	return overlaps
}

func intersect(a, b []string) []string {
	set := make(map[string]bool, len(a))
	for _, f := range a {
		set[f] = true
	}
	var shared []string
	for _, f := range b {
		if set[f] {
			shared = append(shared, f)
		}
	}
	sort.Strings(shared)
	return shared
}

// decide names a winner only when one is safe: green, an applicable diff,
// and no dependency-manifest changes. Everything else is the human's call.
func decide(ranked []Candidate) (winner, reason string) {
	var greens int
	for _, c := range ranked {
		if !c.Green {
			continue
		}
		greens++
		if c.DiffBytes > 0 && len(c.Manifests) == 0 {
			return c.LoopID, fmt.Sprintf("%s: green, smallest clean diff (%d file(s), %s)", c.LoopID, c.FilesChanged, HumanBytes(c.DiffBytes))
		}
	}
	switch {
	case greens == 0:
		return "", "no safe winner: no loop went green"
	default:
		return "", "no safe winner: every green diff is empty or touches dependency manifests — review by hand"
	}
}
