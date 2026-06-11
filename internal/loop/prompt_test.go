package loop

import (
	"strings"
	"testing"
)

func TestComposePrompt(t *testing.T) {
	l := Loop{
		ID:             "fix-csv",
		Goal:           "make the importer handle quoted newlines",
		Constraints:    []string{"do not change the public API"},
		ForbiddenPaths: []string{"vendor/"},
		Verifier: []Stage{
			{Name: "vet", Cmd: "go vet ./..."},
			{Name: "test", Cmd: "go test ./importer/..."},
		},
		Budget:     Budget{MaxIterations: 8},
		BaseCommit: "abcdef0123456789",
	}
	prev := &Iteration{
		Index:        1,
		FailingStage: "test",
		FeedbackTail: "importer_test.go:88: TestQuotedNewlines failed",
		ChangedFiles: []string{"importer/importer.go"},
	}
	prompt := ComposePrompt(l, 2, prev)
	for _, want := range []string{
		"iteration 2 of 8",
		"make the importer handle quoted newlines",
		"do not change the public API",
		"vendor/",
		"go vet ./...",
		"go test ./importer/...",
		"stage `test` failed",
		"TestQuotedNewlines",
		"importer/importer.go",
		"abcdef012345", // short base commit
		"Do not commit",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestComposePromptBaselineFeedbackLabel(t *testing.T) {
	l := Loop{ID: "x", Goal: "g", Verifier: []Stage{{Name: "t", Cmd: "true"}}, Budget: Budget{MaxIterations: 3}}
	prev := &Iteration{Index: 0, FailingStage: "t", FeedbackTail: "boom"}
	prompt := ComposePrompt(l, 1, prev)
	if !strings.Contains(prompt, "baseline check") {
		t.Error("baseline feedback should be labeled as the baseline, not an iteration")
	}
}

func TestComposePromptCapsChangedFiles(t *testing.T) {
	files := make([]string, 80)
	for i := range files {
		files[i] = "file.go"
	}
	l := Loop{ID: "x", Goal: "g", Verifier: []Stage{{Name: "t", Cmd: "true"}}, Budget: Budget{MaxIterations: 3}}
	prompt := ComposePrompt(l, 2, &Iteration{Index: 1, FailingStage: "t", FeedbackTail: "f", ChangedFiles: files})
	if !strings.Contains(prompt, "and 30 more") {
		t.Error("changed files list should be capped at 50 with a truncation note")
	}
	if strings.Count(prompt, "- file.go") != 50 {
		t.Errorf("listed %d files, want 50", strings.Count(prompt, "- file.go"))
	}
}

func TestCheckForbidden(t *testing.T) {
	cases := []struct {
		forbidden []string
		changed   []string
		wantHit   bool
	}{
		{[]string{"vendor/"}, []string{"vendor/lib.go"}, true},
		{[]string{"vendor"}, []string{"vendor/lib.go"}, true},
		{[]string{"go.sum"}, []string{"go.sum"}, true},
		{[]string{"vendor/"}, []string{"vendored/file.go"}, false},
		{[]string{"vendor/"}, []string{"src/main.go"}, false},
		{nil, []string{"anything"}, false},
		{[]string{" "}, []string{"anything"}, false},
	}
	for _, c := range cases {
		got := checkForbidden(c.forbidden, c.changed)
		if (got != "") != c.wantHit {
			t.Errorf("checkForbidden(%v, %v) = %q, wantHit=%v", c.forbidden, c.changed, got, c.wantHit)
		}
	}
}
