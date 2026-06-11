package loop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// InferredVerifier is a guessed default verifier plus where the guess came
// from, so the confirmation prompt can say why.
type InferredVerifier struct {
	Stages []Stage
	Source string
}

var makefileCheckTarget = regexp.MustCompile(`(?m)^check\s*:`)

// InferVerifier guesses a verifier from the repository contents. The guess is
// never used silently: callers must confirm it with the user once and store
// it in config (see DESIGN.md, "Default workflow").
func InferVerifier(root string) (InferredVerifier, bool) {
	for _, name := range []string{"Makefile", "makefile", "GNUmakefile"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err == nil && makefileCheckTarget.Match(data) {
			return InferredVerifier{
				Stages: []Stage{{Name: "check", Cmd: "make check"}},
				Source: name + " has a check target",
			}, true
		}
	}
	if fileExists(filepath.Join(root, "go.mod")) {
		return InferredVerifier{
			Stages: []Stage{
				{Name: "vet", Cmd: "go vet ./..."},
				{Name: "test", Cmd: "go test ./..."},
			},
			Source: "go.mod",
		}, true
	}
	if stages, ok := npmTestVerifier(root); ok {
		return InferredVerifier{Stages: stages, Source: "package.json test script"}, true
	}
	if fileExists(filepath.Join(root, "Cargo.toml")) {
		return InferredVerifier{
			Stages: []Stage{{Name: "test", Cmd: "cargo test"}},
			Source: "Cargo.toml",
		}, true
	}
	for _, name := range []string{"pytest.ini", "pyproject.toml", "setup.py"} {
		if fileExists(filepath.Join(root, name)) {
			return InferredVerifier{
				Stages: []Stage{{Name: "test", Cmd: "python3 -m pytest"}},
				Source: name,
			}, true
		}
	}
	return InferredVerifier{}, false
}

// npmTestVerifier accepts package.json only when its test script is real, not
// npm's "Error: no test specified" placeholder.
func npmTestVerifier(root string) ([]Stage, bool) {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil, false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, false
	}
	script, ok := pkg.Scripts["test"]
	if !ok || strings.Contains(script, "no test specified") {
		return nil, false
	}
	return []Stage{{Name: "test", Cmd: "npm test"}}, true
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
