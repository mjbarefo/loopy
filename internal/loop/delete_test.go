package loop

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeleteLoopRemovesEverythingButTheLogbookLine(t *testing.T) {
	root := newLoopProject(t, `echo done > done.txt`)
	l := mustCreate(t, root, CreateOptions{
		Goal:     "create done.txt",
		Verifier: []Stage{{Name: "done", Cmd: "test -f done.txt"}},
		Budget:   Budget{MaxIterations: 2},
	})
	if _, err := RunEngine(root, l.ID, Events{}); err != nil {
		t.Fatal(err)
	}

	deleted, err := DeleteLoop(root, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Status != StatusGreen {
		t.Fatalf("deleted loop status = %s, want green", deleted.Status)
	}
	if _, err := os.Stat(LoopDir(root, l.ID)); !os.IsNotExist(err) {
		t.Fatal("the loop directory should be gone")
	}
	if _, err := os.Stat(WorktreePath(root, l.ID)); !os.IsNotExist(err) {
		t.Fatal("the worktree should be gone")
	}
	out, err := os.ReadFile(LogbookPath(root))
	if err != nil {
		t.Fatalf("the logbook should survive: %v", err)
	}
	if !strings.Contains(string(out), l.ID+" — deleted") {
		t.Fatalf("the logbook should record the deletion:\n%s", out)
	}
	// The loop is gone from listings, not just unreadable.
	loops, broken, err := ListLoops(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loops) != 0 || len(broken) != 0 {
		t.Fatalf("listing after delete: %d loops, %d broken — want none", len(loops), len(broken))
	}
}

func TestDeleteLoopRefusesALiveEngine(t *testing.T) {
	root := newLoopProject(t, `true`)
	l := mustCreate(t, root, CreateOptions{
		Goal:     "anything",
		Verifier: []Stage{{Name: "v", Cmd: "true"}},
		Budget:   Budget{MaxIterations: 1},
	})
	// The lock is same-pid re-entrant, so a foreign live engine is faked
	// with a real other process holding the lock.
	sleeper := exec.Command("sleep", "30")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = sleeper.Process.Kill()
		_, _ = sleeper.Process.Wait()
	}()
	lock := filepath.Join(LoopDir(root, l.ID), "engine.lock")
	if err := WriteJSON(lock, EngineLock{PID: sleeper.Process.Pid, StartedAt: utcNowISO()}); err != nil {
		t.Fatal(err)
	}

	if _, err := DeleteLoop(root, l.ID); err == nil {
		t.Fatal("deleting a loop with a live engine must be refused")
	}
	if _, err := os.Stat(LoopDir(root, l.ID)); err != nil {
		t.Fatal("a refused delete must leave the loop untouched")
	}
}

func TestDeleteLoopHandlesUnreadableState(t *testing.T) {
	root := newLoopProject(t, `true`)
	l := mustCreate(t, root, CreateOptions{
		Goal:     "anything",
		Verifier: []Stage{{Name: "v", Cmd: "true"}},
		Budget:   Budget{MaxIterations: 1},
	})
	if err := os.WriteFile(filepath.Join(LoopDir(root, l.ID), "loop.json"), []byte("{garbage"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := DeleteLoop(root, l.ID); err != nil {
		t.Fatalf("unreadable state should still be deletable: %v", err)
	}
	if _, err := os.Stat(LoopDir(root, l.ID)); !os.IsNotExist(err) {
		t.Fatal("the unreadable loop directory should be gone")
	}
	out, _ := os.ReadFile(LogbookPath(root))
	if !strings.Contains(string(out), "unreadable") {
		t.Fatalf("the logbook should note the unreadable state:\n%s", out)
	}
}
