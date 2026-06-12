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
// The walk is bounded in depth, breadth, and time — a front door must open
// instantly, so an incomplete list beats a complete hang.

// RepoCandidate is one discovered git repository.
type RepoCandidate struct {
	Path  string    // absolute path to the repo root
	Loops int       // entries under .loopy/loops, 0 when none
	Mod   time.Time // last git activity (mtime of .git)
}

const (
	repoScanMaxDepth = 4
	repoScanMaxDirs  = 8000
	repoScanMaxHits  = 100
	repoScanBudget   = 1500 * time.Millisecond
)

// repoScanSkip names directories that are never worth descending into:
// dependency trees and the giant macOS homedir furniture. Hidden directories
// are skipped wholesale (after the .git check).
var repoScanSkip = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"Library":      true,
	"Applications": true,
	"Music":        true,
	"Movies":       true,
	"Pictures":     true,
}

// FindRepos walks breadth-first from start looking for git repositories.
// Repos that already hold loops sort first (most loops, then most recent git
// activity); the rest sort by activity. The walk does not descend into found
// repos, hidden directories, or the skip list.
func FindRepos(start string) []RepoCandidate {
	deadline := time.Now().Add(repoScanBudget)
	type entry struct {
		path  string
		depth int
	}
	queue := []entry{{start, 0}}
	visited := 0
	var found []RepoCandidate

	for len(queue) > 0 && visited < repoScanMaxDirs && len(found) < repoScanMaxHits {
		if time.Now().After(deadline) {
			break
		}
		dir := queue[0]
		queue = queue[1:]
		visited++

		entries, err := os.ReadDir(dir.path)
		if err != nil {
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
				found = append(found, candidate(child))
				continue
			}
			if dir.depth+1 <= repoScanMaxDepth {
				queue = append(queue, entry{child, dir.depth + 1})
			}
		}
	}

	sort.SliceStable(found, func(i, j int) bool {
		if (found[i].Loops > 0) != (found[j].Loops > 0) {
			return found[i].Loops > 0
		}
		if found[i].Loops != found[j].Loops {
			return found[i].Loops > found[j].Loops
		}
		return found[i].Mod.After(found[j].Mod)
	})
	return found
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
