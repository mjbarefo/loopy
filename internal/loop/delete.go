package loop

import (
	"fmt"
	"os"
)

// DeleteLoop removes a loop entirely: its worktree and branch, then its
// state directory under .loopy/loops/ — iterations, prompts, logs, diffs,
// and any recorded decision. The logbook keeps one narrative line, so the
// project remembers that evidence was discarded. A loop with a live engine
// is refused: abort or pause it first.
//
// Unreadable loop state is deletable too — `loopy delete` is the cleanup
// path doctor points at when repair isn't worth it.
func DeleteLoop(root, loopID string) (Loop, error) {
	if err := AcquireEngineLock(root, loopID); err != nil {
		return Loop{}, fmt.Errorf("cannot delete: %w (abort or pause it first)", err)
	}
	// No deferred release: the lock file is removed with the directory; a
	// failure before that point releases explicitly.
	release := func() { _ = ReleaseEngineLock(root, loopID) }

	l, err := LoadLoop(root, loopID)
	if err != nil {
		// Corrupt state is still deletable; record what little is known.
		l = Loop{ID: loopID, Status: "unreadable", Goal: "(loop.json unreadable)"}
	}
	if err := RemoveLoopWorktree(root, loopID); err != nil {
		release()
		return l, fmt.Errorf("freeing the worktree failed (see `loopy doctor`): %w", err)
	}
	if err := appendDeletionLogbook(root, l); err != nil {
		release()
		return l, err
	}
	if err := os.RemoveAll(LoopDir(root, loopID)); err != nil {
		release()
		return l, err
	}
	return l, nil
}

// appendDeletionLogbook writes the one line that outlives the evidence.
func appendDeletionLogbook(root string, l Loop) error {
	entry := fmt.Sprintf("## %s — deleted — %s\n\n- goal: %s\n- status at deletion: %s\n- worktree, iterations, and decision records removed\n\n",
		l.ID, utcNowISO(), l.Goal, l.Status)
	f, err := os.OpenFile(LogbookPath(root), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(entry); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
