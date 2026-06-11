package loop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// On-disk layout under the project root. Everything is plain JSON, markdown,
// and patches — inspectable without loopy.
const (
	LoopyDir     = ".loopy"
	LoopsDir     = "loops"
	WorktreesDir = "worktrees"
	IterDir      = "iterations"

	configFile  = "config.json"
	agentsFile  = "agents.json"
	loopFile    = "loop.json"
	controlFile = "control.json"
	iterFile    = "iteration.json"
	phaseFile   = "phase.json"

	// PromptFile, AgentLogFile, VerifierLogFile, DiffFile are the per-iteration
	// evidence artifacts.
	PromptFile      = "prompt.md"
	AgentLogFile    = "agent.log"
	VerifierLogFile = "verifier.log"
	DiffFile        = "diff.patch"
)

var ErrNotInitialized = errors.New("no .loopy directory found: run `loopy init` first")

// LoopyPath returns <root>/.loopy.
func LoopyPath(root string) string { return filepath.Join(root, LoopyDir) }

// LoopDir returns the state directory for one loop.
func LoopDir(root, loopID string) string {
	return filepath.Join(root, LoopyDir, LoopsDir, loopID)
}

// IterationDir returns the evidence directory for one iteration.
func IterationDir(root, loopID string, index int) string {
	return filepath.Join(LoopDir(root, loopID), IterDir, fmt.Sprintf("%04d", index))
}

// WorktreePath returns where a loop's git worktree lives.
func WorktreePath(root, loopID string) string {
	return filepath.Join(root, LoopyDir, WorktreesDir, loopID)
}

// EnsureInitialized errors unless `loopy init` has been run at root.
func EnsureInitialized(root string) error {
	info, err := os.Stat(LoopyPath(root))
	if err != nil || !info.IsDir() {
		return ErrNotInitialized
	}
	return nil
}

// InitProject creates the .loopy skeleton and makes sure the directory is
// git-ignored (live state and worktrees must never dirty the repo — the
// dirty-repo refusal would deadlock loopy against itself). The second return
// reports whether .gitignore was modified.
func InitProject(root string) (string, bool, error) {
	base := LoopyPath(root)
	for _, dir := range []string{base, filepath.Join(base, LoopsDir), filepath.Join(base, WorktreesDir)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", false, err
		}
	}
	if _, err := os.Stat(filepath.Join(base, agentsFile)); errors.Is(err, os.ErrNotExist) {
		if err := WriteJSON(filepath.Join(base, agentsFile), AgentRegistry{Agents: map[string]Agent{}}); err != nil {
			return "", false, err
		}
	}
	if _, err := os.Stat(filepath.Join(base, configFile)); errors.Is(err, os.ErrNotExist) {
		if err := WriteJSON(filepath.Join(base, configFile), Config{}); err != nil {
			return "", false, err
		}
	}
	ignored, err := ensureGitignored(root)
	if err != nil {
		return "", false, err
	}
	return base, ignored, nil
}

// ensureGitignored appends ".loopy/" to the project .gitignore when missing,
// reporting whether it wrote.
func ensureGitignored(root string) (bool, error) {
	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == ".loopy/" || trimmed == ".loopy" || trimmed == "/.loopy/" || trimmed == "/.loopy" {
			return false, nil
		}
	}
	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += ".loopy/\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// WriteJSON writes v as indented JSON atomically: temp file in the target
// directory, then rename. Readers (the monitor, humans, other loopy commands)
// never observe a half-written document.
func WriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'), 0o644)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// ReadJSON loads path into v.
func ReadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// SaveLoop flushes the loop definition + status to disk.
func SaveLoop(root string, l Loop) error {
	return WriteJSON(filepath.Join(LoopDir(root, l.ID), loopFile), l)
}

// LoadLoop reads one loop by ID.
func LoadLoop(root, loopID string) (Loop, error) {
	var l Loop
	path := filepath.Join(LoopDir(root, loopID), loopFile)
	if err := ReadJSON(path, &l); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Loop{}, fmt.Errorf("loop not found: %s", loopID)
		}
		return Loop{}, err
	}
	return l, nil
}

// BrokenLoop reports a loop directory whose loop.json could not be read.
// One damaged file must never take down list/status/watch for every other
// loop — readers degrade and point at the evidence instead.
type BrokenLoop struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Err  string `json:"error"`
}

// ListLoops returns all readable loops sorted by creation time, newest last,
// plus a report of any loop directories whose state is unreadable.
func ListLoops(root string) ([]Loop, []BrokenLoop, error) {
	dir := filepath.Join(root, LoopyDir, LoopsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	var loops []Loop
	var broken []BrokenLoop
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		l, err := LoadLoop(root, entry.Name())
		if err != nil {
			broken = append(broken, BrokenLoop{
				ID:   entry.Name(),
				Path: filepath.Join(dir, entry.Name(), loopFile),
				Err:  err.Error(),
			})
			continue
		}
		loops = append(loops, l)
	}
	sort.Slice(loops, func(i, j int) bool {
		if loops[i].CreatedAt != loops[j].CreatedAt {
			return loops[i].CreatedAt < loops[j].CreatedAt
		}
		return loops[i].ID < loops[j].ID
	})
	return loops, broken, nil
}

// LoopIDs lists existing loop directory names (for ID disambiguation).
func LoopIDs(root string) ([]string, error) {
	dir := filepath.Join(root, LoopyDir, LoopsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		if entry.IsDir() {
			ids = append(ids, entry.Name())
		}
	}
	return ids, nil
}

// SaveIteration flushes one iteration record.
func SaveIteration(root, loopID string, it Iteration) error {
	return WriteJSON(filepath.Join(IterationDir(root, loopID, it.Index), iterFile), it)
}

// LoadIterations returns all recorded iterations for a loop in index order.
// A corrupt record is an error here: the engine resumes from this history and
// must not silently skip evidence. Read-only callers use LoadIterationsLenient.
func LoadIterations(root, loopID string) ([]Iteration, error) {
	iterations, damaged, err := loadIterations(root, loopID)
	if err != nil {
		return nil, err
	}
	if len(damaged) > 0 {
		return nil, fmt.Errorf("iteration state unreadable: %s", strings.Join(damaged, "; "))
	}
	return iterations, nil
}

// LoadIterationsLenient is the readers' variant: corrupt iteration records
// are skipped and reported instead of failing the whole loop view.
func LoadIterationsLenient(root, loopID string) (iterations []Iteration, damaged []string, err error) {
	return loadIterations(root, loopID)
}

func loadIterations(root, loopID string) (iterations []Iteration, damaged []string, err error) {
	dir := filepath.Join(LoopDir(root, loopID), IterDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var it Iteration
		path := filepath.Join(dir, entry.Name(), iterFile)
		if err := ReadJSON(path, &it); err != nil {
			// The engine creates the evidence directory first and writes
			// iteration.json last: a missing record means the iteration is
			// in flight (or died mid-write), not corrupt state. Readers —
			// list, status, the monitor — must keep working while the
			// engine runs.
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			damaged = append(damaged, err.Error())
			continue
		}
		iterations = append(iterations, it)
	}
	sort.Slice(iterations, func(i, j int) bool { return iterations[i].Index < iterations[j].Index })
	return iterations, damaged, nil
}

// readDirIfExists lists a directory, treating a missing one as empty.
func readDirIfExists(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return entries, err
}

// LoadConfig reads .loopy/config.json; a missing file is an empty config.
func LoadConfig(root string) (Config, error) {
	var c Config
	err := ReadJSON(filepath.Join(LoopyPath(root), configFile), &c)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	return c, err
}

// SaveConfig writes .loopy/config.json.
func SaveConfig(root string, c Config) error {
	return WriteJSON(filepath.Join(LoopyPath(root), configFile), c)
}

// WritePhase records what the engine is doing right now (agent or verify,
// which iteration, since when). Engine-only, like all loop state; the monitor
// reads it to answer "what is it doing" without guessing from log mtimes.
func WritePhase(root, loopID string, p Phase) error {
	return WriteJSON(filepath.Join(LoopDir(root, loopID), phaseFile), p)
}

// ReadPhase loads the engine's current phase; a missing file means the engine
// is between iterations (or not running — gate on lock liveness).
func ReadPhase(root, loopID string) (Phase, bool, error) {
	var p Phase
	err := ReadJSON(filepath.Join(LoopDir(root, loopID), phaseFile), &p)
	if errors.Is(err, os.ErrNotExist) {
		return Phase{}, false, nil
	}
	if err != nil {
		return Phase{}, false, err
	}
	return p, true, nil
}

// ClearPhase removes the phase record (iteration boundary or engine exit).
func ClearPhase(root, loopID string) error {
	err := os.Remove(filepath.Join(LoopDir(root, loopID), phaseFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ReadControl loads a loop's control document; missing file means no request.
func ReadControl(root, loopID string) (Control, error) {
	var c Control
	err := ReadJSON(filepath.Join(LoopDir(root, loopID), controlFile), &c)
	if errors.Is(err, os.ErrNotExist) {
		return Control{}, nil
	}
	return c, err
}

// WriteControl stores a control request for the engine to pick up.
func WriteControl(root, loopID string, c Control) error {
	return WriteJSON(filepath.Join(LoopDir(root, loopID), controlFile), c)
}

// ClearControl removes any pending control request.
func ClearControl(root, loopID string) error {
	err := os.Remove(filepath.Join(LoopDir(root, loopID), controlFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
