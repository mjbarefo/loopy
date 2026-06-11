//go:build windows

package loop

import (
	"os"
	"os/exec"
)

// Windows has no process groups in the unix sense; we kill the direct child
// only. Known gap, tracked in DECISIONS.md alongside crux's Windows parity
// notes.
func setProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// processAlive is a best-effort liveness probe on Windows: FindProcess only
// errors for pids that cannot exist.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = proc.Release()
	return true
}
