package loop

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newTestRepo creates a committed git repo in a temp dir.
func newTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	mustGit(t, root, "init", "-q", "-b", "main")
	mustGit(t, root, "config", "user.email", "test@example.com")
	mustGit(t, root, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "initial")
	// macOS: /tmp is a symlink to /private/tmp; resolve so paths compare equal.
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func TestDetectGitRoot(t *testing.T) {
	root := newTestRepo(t)
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := DetectGitRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("DetectGitRoot = %q, want %q", got, root)
	}
	if _, err := DetectGitRoot(t.TempDir()); err == nil {
		t.Fatal("expected error outside a repo")
	}
}

func TestWorktreeLifecycleAndSnapshot(t *testing.T) {
	root := newTestRepo(t)
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	// .gitignore changed by init → dirty → refusal.
	if _, _, _, err := CreateLoopWorktree(root, "my-loop"); err == nil {
		t.Fatal("expected dirty-repo refusal")
	}
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "init loopy")

	wt, branch, base, err := CreateLoopWorktree(root, "my-loop")
	if err != nil {
		t.Fatal(err)
	}
	if branch != "loopy/my-loop" {
		t.Fatalf("branch = %q", branch)
	}
	if base == "" {
		t.Fatal("base commit empty")
	}
	if !strings.HasPrefix(wt, root) {
		t.Fatalf("worktree %q not under root %q", wt, root)
	}

	// Untracked new file + modified file + agent-staged file all appear in
	// the snapshot without touching the real index.
	if err := os.WriteFile(filepath.Join(wt, "hello.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff, changed, err := Snapshot(wt, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff) == 0 {
		t.Fatal("diff empty")
	}
	want := map[string]bool{"hello.txt": true, "new.txt": true}
	if len(changed) != 2 || !want[changed[0]] || !want[changed[1]] {
		t.Fatalf("changed = %v", changed)
	}

	// A clean worktree snapshots to an empty diff.
	wt2, _, base2, err := CreateLoopWorktree(root, "clean-loop")
	if err != nil {
		t.Fatal(err)
	}
	diff2, changed2, err := Snapshot(wt2, base2)
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.TrimSpace(string(diff2))) != 0 || len(changed2) != 0 {
		t.Fatalf("clean snapshot: diff %d bytes, changed %v", len(diff2), changed2)
	}

	if err := RemoveLoopWorktree(root, "my-loop"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree still present: %v", err)
	}
	// Removing twice is fine.
	if err := RemoveLoopWorktree(root, "my-loop"); err != nil {
		t.Fatal(err)
	}
}

func TestParseNameStatus(t *testing.T) {
	data := []byte("M\x00mod.txt\x00A\x00added.txt\x00R100\x00old.txt\x00new.txt\x00")
	got := parseNameStatus(data)
	want := []string{"mod.txt", "added.txt", "new.txt"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
