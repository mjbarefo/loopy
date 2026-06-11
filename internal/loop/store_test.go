package loop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInitProjectIdempotentAndGitignore(t *testing.T) {
	root := t.TempDir()
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(data), ".loopy/"); count != 1 {
		t.Fatalf(".gitignore has %d .loopy/ entries, want 1:\n%s", count, data)
	}
}

func TestInitProjectPreservesExistingGitignore(t *testing.T) {
	root := t.TempDir()
	original := "node_modules/\n*.log"
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	got := string(data)
	if !strings.HasPrefix(got, original) || !strings.Contains(got, ".loopy/") {
		t.Fatalf("gitignore mangled:\n%s", got)
	}
}

func TestLoopRoundTrip(t *testing.T) {
	root := t.TempDir()
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	l := Loop{
		ID:       "fix-it",
		Goal:     "fix it",
		Agent:    "shell",
		Verifier: []Stage{{Name: "test", Cmd: "true"}},
		Budget:   Budget{MaxIterations: 4, MaxWallClock: Duration(10 * time.Minute)},
		Stuck:    DefaultStuckPolicy,
		Status:   StatusRunning,
	}
	if err := SaveLoop(root, l); err != nil {
		t.Fatal(err)
	}
	back, err := LoadLoop(root, "fix-it")
	if err != nil {
		t.Fatal(err)
	}
	if back.Goal != l.Goal || back.Budget.MaxIterations != 4 || time.Duration(back.Budget.MaxWallClock) != 10*time.Minute {
		t.Fatalf("round trip mismatch: %+v", back)
	}
	if _, err := LoadLoop(root, "missing"); err == nil || !strings.Contains(err.Error(), "loop not found") {
		t.Fatalf("expected loop-not-found, got %v", err)
	}
}

func TestIterationsRoundTripAndOrder(t *testing.T) {
	root := t.TempDir()
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	for _, idx := range []int{2, 0, 1} {
		if err := SaveIteration(root, "fix-it", Iteration{Index: idx, DiffHash: "h"}); err != nil {
			t.Fatal(err)
		}
	}
	iterations, err := LoadIterations(root, "fix-it")
	if err != nil {
		t.Fatal(err)
	}
	if len(iterations) != 3 {
		t.Fatalf("got %d iterations", len(iterations))
	}
	for i, it := range iterations {
		if it.Index != i {
			t.Fatalf("iteration %d has index %d", i, it.Index)
		}
	}
}

func TestControlRoundTrip(t *testing.T) {
	root := t.TempDir()
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	// Missing control file reads as zero value.
	c, err := ReadControl(root, "x")
	if err != nil || c.Pause || c.Abort {
		t.Fatalf("empty control = %+v, err %v", c, err)
	}
	if err := WriteControl(root, "x", Control{Abort: true, Reason: "tired"}); err != nil {
		t.Fatal(err)
	}
	c, err = ReadControl(root, "x")
	if err != nil || !c.Abort || c.Reason != "tired" {
		t.Fatalf("control = %+v, err %v", c, err)
	}
	if err := ClearControl(root, "x"); err != nil {
		t.Fatal(err)
	}
	if err := ClearControl(root, "x"); err != nil {
		t.Fatal(err) // clearing twice is fine
	}
}

func TestListLoopsSortedByCreation(t *testing.T) {
	root := t.TempDir()
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"b-loop", "a-loop"} {
		l := Loop{ID: id, CreatedAt: time.Date(2026, 6, 10, 12, i, 0, 0, time.UTC).Format(time.RFC3339)}
		if err := SaveLoop(root, l); err != nil {
			t.Fatal(err)
		}
	}
	loops, broken, err := ListLoops(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(broken) != 0 {
		t.Fatalf("unexpected broken loops: %+v", broken)
	}
	if len(loops) != 2 || loops[0].ID != "b-loop" || loops[1].ID != "a-loop" {
		t.Fatalf("order wrong: %+v", loops)
	}
}

func TestListLoopsToleratesCorruptState(t *testing.T) {
	root := t.TempDir()
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	if err := SaveLoop(root, Loop{ID: "good-loop", CreatedAt: "2026-06-10T12:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	dir := LoopDir(root, "bad-loop")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "loop.json"), []byte("{ corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	loops, broken, err := ListLoops(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loops) != 1 || loops[0].ID != "good-loop" {
		t.Fatalf("good loop missing: %+v", loops)
	}
	if len(broken) != 1 || broken[0].ID != "bad-loop" || broken[0].Err == "" {
		t.Fatalf("broken loop not reported: %+v", broken)
	}
}

func TestLoadIterationsReportsCorruptRecords(t *testing.T) {
	root := t.TempDir()
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	if err := SaveLoop(root, Loop{ID: "l", CreatedAt: "2026-06-10T12:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveIteration(root, "l", Iteration{Index: 0}); err != nil {
		t.Fatal(err)
	}
	dir := IterationDir(root, "l", 1)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "iteration.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIterations(root, "l"); err == nil {
		t.Fatal("strict LoadIterations should fail on a corrupt record")
	}
	iterations, damaged, err := LoadIterationsLenient(root, "l")
	if err != nil {
		t.Fatal(err)
	}
	if len(iterations) != 1 || iterations[0].Index != 0 {
		t.Fatalf("readable iteration missing: %+v", iterations)
	}
	if len(damaged) != 1 {
		t.Fatalf("damaged record not reported: %+v", damaged)
	}
}

func TestLoadIterationsSkipsInFlight(t *testing.T) {
	root := t.TempDir()
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	if err := SaveIteration(root, "demo", Iteration{Index: 0, Green: false}); err != nil {
		t.Fatal(err)
	}
	// The engine creates the next evidence directory before the record
	// exists; readers must keep working while the iteration is in flight.
	if err := os.MkdirAll(IterationDir(root, "demo", 1), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(IterationDir(root, "demo", 1), AgentLogFile), []byte("working…\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	iterations, err := LoadIterations(root, "demo")
	if err != nil {
		t.Fatalf("LoadIterations with an in-flight iteration: %v", err)
	}
	if len(iterations) != 1 || iterations[0].Index != 0 {
		t.Fatalf("want only the recorded iteration, got %+v", iterations)
	}
}
