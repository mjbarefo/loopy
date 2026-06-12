package loop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRepos(t *testing.T) {
	root := t.TempDir()
	mk := func(parts ...string) string {
		p := filepath.Join(append([]string{root}, parts...)...)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}

	mk("projects", "alpha", ".git")
	mk("projects", "deep", "nested", "beta", ".git")
	mk(".hidden", "ghost", ".git")         // hidden trees are not walked
	mk("node_modules", "dep", ".git")      // dependency trees are not walked
	mk("projects", "alpha", "sub", ".git") // inside a repo: not walked past the root

	// gamma already holds loops — it must sort first.
	mk("work", "gamma", ".git")
	mk("work", "gamma", LoopyDir, LoopsDir, "loop-one")
	mk("work", "gamma", LoopyDir, LoopsDir, "loop-two")

	got := FindRepos(root)
	var paths []string
	for _, c := range got {
		rel, _ := filepath.Rel(root, c.Path)
		paths = append(paths, rel)
	}
	if len(got) != 3 {
		t.Fatalf("found %v, want alpha, beta(nested), gamma", paths)
	}
	if paths[0] != filepath.Join("work", "gamma") || got[0].Loops != 2 {
		t.Errorf("repo with loops should sort first with its count, got %v (loops %d)", paths[0], got[0].Loops)
	}
	for _, p := range paths {
		if p == filepath.Join(".hidden", "ghost") || p == filepath.Join("node_modules", "dep") {
			t.Errorf("walked into a skipped tree: %s", p)
		}
		if p == filepath.Join("projects", "alpha", "sub") {
			t.Errorf("walked past a repo root: %s", p)
		}
	}
}

func TestFindReposEmpty(t *testing.T) {
	if got := FindRepos(t.TempDir()); len(got) != 0 {
		t.Fatalf("empty tree found %d repos", len(got))
	}
}
