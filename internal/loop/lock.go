package loop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const lockFile = "engine.lock"

// EngineLock marks a loop as owned by a live engine process. A lock whose pid
// is dead is stale — the engine crashed — and `loopy resume` may take over.
type EngineLock struct {
	PID       int    `json:"pid"`
	StartedAt string `json:"started_at"`
}

func lockPath(root, loopID string) string {
	return filepath.Join(LoopDir(root, loopID), lockFile)
}

// AcquireEngineLock claims the loop for this process. It fails when another
// live engine holds the lock and silently replaces a stale one.
func AcquireEngineLock(root, loopID string) error {
	path := lockPath(root, loopID)
	var existing EngineLock
	err := ReadJSON(path, &existing)
	switch {
	case err == nil:
		if existing.PID != os.Getpid() && processAlive(existing.PID) {
			return fmt.Errorf("loop %s is already running (pid %d)", loopID, existing.PID)
		}
	case errors.Is(err, os.ErrNotExist):
		// free
	default:
		// Unreadable lock: treat as stale but mention it.
		_ = os.Remove(path)
	}
	return WriteJSON(path, EngineLock{PID: os.Getpid(), StartedAt: utcNowISO()})
}

// ReleaseEngineLock drops this process's lock; it never removes another live
// process's lock.
func ReleaseEngineLock(root, loopID string) error {
	path := lockPath(root, loopID)
	var existing EngineLock
	if err := ReadJSON(path, &existing); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return os.Remove(path)
	}
	if existing.PID != os.Getpid() && processAlive(existing.PID) {
		return nil
	}
	return os.Remove(path)
}

// EngineLockState reports the lock for diagnostics: held (live), stale, or
// absent.
func EngineLockState(root, loopID string) (lock EngineLock, held, stale bool) {
	var existing EngineLock
	if err := ReadJSON(lockPath(root, loopID), &existing); err != nil {
		return EngineLock{}, false, false
	}
	if processAlive(existing.PID) {
		return existing, true, false
	}
	return existing, false, true
}
