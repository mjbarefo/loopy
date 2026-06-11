package loop

import (
	"fmt"
	"os"
	"os/exec"
)

// Doctor check statuses.
const (
	DoctorOK   = "ok"
	DoctorWarn = "warn"
	DoctorFail = "fail"
)

// DoctorCheck is one diagnostic finding.
type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// Doctor runs environment and state diagnostics: tool availability, init
// state, agent registration, stale engine locks, and missing worktrees.
func Doctor(root string) []DoctorCheck {
	var checks []DoctorCheck
	add := func(name, status, detail string) {
		checks = append(checks, DoctorCheck{Name: name, Status: status, Detail: detail})
	}

	if _, err := exec.LookPath("git"); err != nil {
		add("git", DoctorFail, "git executable not found on PATH")
	} else {
		add("git", DoctorOK, "git found")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		add("shell", DoctorFail, "sh not found on PATH; agents and verifiers run via `sh -c`")
	} else {
		add("shell", DoctorOK, "sh found")
	}

	if _, err := DetectGitRoot(root); err != nil {
		add("repository", DoctorFail, fmt.Sprintf("%s is not a git repository; loops need worktree isolation", root))
	} else {
		add("repository", DoctorOK, "inside a git repository")
	}

	if err := EnsureInitialized(root); err != nil {
		add("init", DoctorWarn, "no .loopy directory; run `loopy init`")
		return checks
	}
	add("init", DoctorOK, ".loopy present")

	reg, err := LoadAgents(root)
	switch {
	case err != nil:
		add("agents", DoctorFail, fmt.Sprintf("agents.json unreadable: %v", err))
	case len(reg.Agents) == 0:
		add("agents", DoctorWarn, "no agents registered; run `loopy agent add`")
	case reg.Default == "":
		add("agents", DoctorWarn, "agents registered but no default set; pass --default to `loopy agent add`")
	default:
		add("agents", DoctorOK, fmt.Sprintf("%d agent(s), default %q", len(reg.Agents), reg.Default))
	}

	loops, err := ListLoops(root)
	if err != nil {
		add("loops", DoctorFail, fmt.Sprintf("loop state unreadable: %v", err))
		return checks
	}
	add("loops", DoctorOK, fmt.Sprintf("%d loop(s) recorded", len(loops)))

	for _, l := range loops {
		lock, held, stale := EngineLockState(root, l.ID)
		if stale {
			add("locks", DoctorWarn, fmt.Sprintf("loop %s has a stale engine lock (pid %d is dead); `loopy resume %s` will take over", l.ID, lock.PID, l.ID))
		}
		if l.Status == StatusRunning && !held && !stale {
			add("locks", DoctorWarn, fmt.Sprintf("loop %s is marked running but no engine holds it; `loopy resume %s` continues it", l.ID, l.ID))
		}
		if !l.Done() && l.Worktree != "" {
			if info, err := os.Stat(l.Worktree); err != nil || !info.IsDir() {
				add("worktrees", DoctorWarn, fmt.Sprintf("loop %s worktree missing: %s", l.ID, l.Worktree))
			}
		}
	}
	return checks
}

// DoctorHealthy reports whether no check failed.
func DoctorHealthy(checks []DoctorCheck) bool {
	for _, c := range checks {
		if c.Status == DoctorFail {
			return false
		}
	}
	return true
}
