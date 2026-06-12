package loop

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	ErrGitUnavailable = errors.New("git executable not found")
	ErrNotGitRepo     = errors.New("not inside a git repository")
)

// DetectGitRoot resolves the repository top level for path.
func DetectGitRoot(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	output, err := runGit(absPath, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		if errors.Is(err, ErrGitUnavailable) {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", ErrNotGitRepo, absPath)
	}
	top := strings.TrimSpace(string(output))
	if top == "" {
		return "", fmt.Errorf("%w: %s", ErrNotGitRepo, absPath)
	}
	return filepath.Clean(top), nil
}

// DirtyPaths lists tracked files with uncommitted changes. Untracked files
// don't count: loop worktrees branch from HEAD and never see them, so they
// can't make a loop's diff unreproducible.
func DirtyPaths(root string) ([]string, error) {
	output, err := runGitChecked(root, nil, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return nil, err
	}
	var paths []string
	// Porcelain v1: two status columns, a space, then the path.
	for _, line := range strings.Split(string(output), "\n") {
		if len(line) > 3 {
			paths = append(paths, line[3:])
		}
	}
	return paths, nil
}

// IsGitDirty reports uncommitted changes to tracked files.
func IsGitDirty(root string) (bool, error) {
	paths, err := DirtyPaths(root)
	return len(paths) > 0, err
}

// EnsureWorktreePreconditions verifies git + worktree support and refuses a
// dirty repository: an isolated worktree from an uncommitted base would make
// the loop's diff unreproducible.
func EnsureWorktreePreconditions(root string) error {
	if _, err := gitOutput(root, "worktree", "list", "--porcelain"); err != nil {
		return fmt.Errorf("git worktree is unavailable for %s: %w", root, err)
	}
	dirty, err := DirtyPaths(root)
	if err != nil {
		return err
	}
	if len(dirty) > 0 {
		shown := dirty
		if len(shown) > 5 {
			shown = shown[:5]
		}
		suffix := ""
		if len(dirty) > len(shown) {
			suffix = fmt.Sprintf(" … and %d more", len(dirty)-len(shown))
		}
		return fmt.Errorf("uncommitted changes to tracked files (%s%s): commit or stash before starting a loop", strings.Join(shown, ", "), suffix)
	}
	return nil
}

// CreateLoopWorktree adds a worktree for the loop on branch loopy/<loop-id>
// at HEAD, returning the worktree path, branch name, and base commit.
func CreateLoopWorktree(root, loopID string) (path, branch, baseCommit string, err error) {
	if err := EnsureWorktreePreconditions(root); err != nil {
		return "", "", "", err
	}
	baseCommit, err = gitOutput(root, "rev-parse", "HEAD")
	if err != nil {
		return "", "", "", err
	}
	path = WorktreePath(root, loopID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", "", "", err
	}
	branch = "loopy/" + loopID
	if _, err := gitOutput(root, "worktree", "add", "-b", branch, path, "HEAD"); err != nil {
		return "", "", "", err
	}
	return path, branch, strings.TrimSpace(baseCommit), nil
}

// RemoveLoopWorktree drops a loop's worktree and branch. Used by reject and
// by doctor repairs; the evidence under .loopy/loops/ is never touched.
func RemoveLoopWorktree(root, loopID string) error {
	path := WorktreePath(root, loopID)
	if _, err := os.Stat(path); err == nil {
		if _, err := gitOutput(root, "worktree", "remove", "--force", path); err != nil {
			return err
		}
	}
	_, err := gitOutput(root, "branch", "-D", "loopy/"+loopID)
	if err != nil && strings.Contains(err.Error(), "not found") {
		return nil
	}
	return err
}

// Snapshot captures the worktree's cumulative state against the loop's base
// commit: a binary diff (apply-able with `git apply`) plus changed file paths.
// It uses a temporary index so untracked files are included and the
// worktree's real index is never touched, even if the agent staged or
// committed things it shouldn't have.
func Snapshot(worktree, baseCommit string) (diff []byte, changed []string, err error) {
	indexFile, err := os.CreateTemp("", "loopy-index-*")
	if err != nil {
		return nil, nil, err
	}
	indexPath := indexFile.Name()
	if err := indexFile.Close(); err != nil {
		_ = os.Remove(indexPath)
		return nil, nil, err
	}
	defer os.Remove(indexPath)

	env := append(os.Environ(), "GIT_INDEX_FILE="+indexPath)
	if _, err := runGitChecked(worktree, env, "read-tree", baseCommit); err != nil {
		return nil, nil, err
	}
	if _, err := runGitChecked(worktree, env, "add", "-A", "--", "."); err != nil {
		return nil, nil, err
	}
	diff, err = runGitChecked(worktree, env, "diff", "--cached", "--binary", baseCommit, "--")
	if err != nil {
		return nil, nil, err
	}
	nameStatus, err := runGitChecked(worktree, env, "diff", "--cached", "--name-status", "-z", baseCommit, "--")
	if err != nil {
		return nil, nil, err
	}
	return diff, parseNameStatus(nameStatus), nil
}

// parseNameStatus reads `diff --name-status -z` output. Renames and copies
// carry two paths; the new path is what matters for forbidden-path checks.
func parseNameStatus(data []byte) []string {
	parts := strings.Split(string(data), "\x00")
	var files []string
	for i := 0; i < len(parts); {
		status := parts[i]
		i++
		if status == "" {
			continue
		}
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			if i+1 >= len(parts) {
				break
			}
			files = append(files, parts[i+1])
			i += 2
			continue
		}
		if i >= len(parts) {
			break
		}
		files = append(files, parts[i])
		i++
	}
	return files
}

func gitOutput(repoPath string, args ...string) (string, error) {
	output, err := runGitChecked(repoPath, nil, args...)
	return strings.TrimSpace(string(output)), err
}

func runGitChecked(repoPath string, env []string, args ...string) ([]byte, error) {
	output, err := runGit(repoPath, env, args...)
	if err != nil {
		if errors.Is(err, ErrGitUnavailable) {
			return nil, err
		}
		trimmed := bytes.TrimSpace(output)
		if len(trimmed) > 0 {
			return nil, fmt.Errorf("git %s failed: %s", strings.Join(args, " "), trimmed)
		}
		return nil, fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
	}
	return output, nil
}

func runGit(repoPath string, env []string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("%w: git", ErrGitUnavailable)
	}
	gitArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.Command("git", gitArgs...)
	if env != nil {
		cmd.Env = env
	}
	return cmd.CombinedOutput()
}

// RestoreWorktree forces a loop worktree back to exactly base commit + the
// recorded diff: reset to base, drop everything untracked, re-apply the
// verified diff. Used after a reviewer run — the reviewer reads, it must not
// ship, and the parked diff must be exactly the one the verifier approved.
func RestoreWorktree(worktree, baseCommit, diffPath string) error {
	env := os.Environ()
	if _, err := runGitChecked(worktree, env, "reset", "--hard", baseCommit); err != nil {
		return err
	}
	if _, err := runGitChecked(worktree, env, "clean", "-fdq"); err != nil {
		return err
	}
	info, err := os.Stat(diffPath)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return nil
	}
	_, err = runGitChecked(worktree, env, "apply", diffPath)
	return err
}
