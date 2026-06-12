package loop

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Repo discovery for the front door: when bare `loopy` runs outside a git
// repository, the monitor offers nearby repositories instead of a dead end.
// The scan streams: candidates are emitted as they are found so the picker
// fills in live, and the walk stays bounded in depth, breadth, and time —
// it backs a UI, not an index.

// RepoCandidate is one discovered git repository.
type RepoCandidate struct {
	Path  string    // absolute path to the repo root
	Loops int       // entries under .loopy/loops, 0 when none
	Mod   time.Time // last git activity (mtime of .git)
}

// ScanSummary reports how a scan ended.
type ScanSummary struct {
	// Denied lists near-top-level directories the scan could not read for
	// permission reasons — on macOS this is how a TCC block on Documents or
	// Desktop presents. The picker turns it into an actionable hint.
	Denied []string
}

const (
	repoScanMaxDepth = 4
	repoScanMaxDirs  = 50000
	repoScanMaxHits  = 100
	repoScanBudget   = 8 * time.Second
)

// repoScanSkip names directories that are never worth descending into:
// dependency trees and the giant macOS homedir furniture. Hidden directories
// are skipped wholesale.
var repoScanSkip = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"Library":      true,
	"Applications": true,
	"Music":        true,
	"Movies":       true,
	"Pictures":     true,
}

// ScanRepos walks breadth-first from start, calling emit for each git
// repository found, in discovery order. It does not descend into found
// repos, hidden directories, or the skip list, and stops at its depth,
// directory, hit, and time bounds.
func ScanRepos(start string, emit func(RepoCandidate)) ScanSummary {
	deadline := time.Now().Add(repoScanBudget)
	type entry struct {
		path  string
		depth int
	}
	queue := []entry{{start, 0}}
	visited, hits := 0, 0
	var summary ScanSummary

	for len(queue) > 0 && visited < repoScanMaxDirs && hits < repoScanMaxHits {
		if time.Now().After(deadline) {
			break
		}
		dir := queue[0]
		queue = queue[1:]
		visited++

		entries, err := os.ReadDir(dir.path)
		if err != nil {
			if os.IsPermission(err) && dir.depth <= 1 {
				summary.Denied = append(summary.Denied, filepath.Base(dir.path))
			}
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasPrefix(name, ".") || repoScanSkip[name] {
				continue
			}
			child := filepath.Join(dir.path, name)
			if isRepoRoot(child) {
				emit(candidate(child))
				hits++
				continue
			}
			if dir.depth+1 <= repoScanMaxDepth {
				queue = append(queue, entry{child, dir.depth + 1})
			}
		}
	}
	return summary
}

// FindRepos is the synchronous form: scan, then sort with
// SortRepoCandidates.
func FindRepos(start string) []RepoCandidate {
	var found []RepoCandidate
	ScanRepos(start, func(c RepoCandidate) { found = append(found, c) })
	SortRepoCandidates(found)
	return found
}

// SortRepoCandidates orders repos the way the picker lists them: repos
// already holding loops first (most loops, then most recent git activity),
// the rest by activity.
func SortRepoCandidates(found []RepoCandidate) {
	sort.SliceStable(found, func(i, j int) bool {
		if (found[i].Loops > 0) != (found[j].Loops > 0) {
			return found[i].Loops > 0
		}
		if found[i].Loops != found[j].Loops {
			return found[i].Loops > found[j].Loops
		}
		return found[i].Mod.After(found[j].Mod)
	})
}

func isRepoRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func candidate(root string) RepoCandidate {
	c := RepoCandidate{Path: root}
	if info, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		c.Mod = info.ModTime()
	}
	if entries, err := os.ReadDir(filepath.Join(root, LoopyDir, LoopsDir)); err == nil {
		c.Loops = len(entries)
	}
	return c
}
