package loop

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInferVerifier(t *testing.T) {
	t.Run("makefile check target wins", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, root, "Makefile", "build:\n\tgo build\n\ncheck: test\n\tgo vet ./...\n")
		writeTestFile(t, root, "go.mod", "module x\n")
		inferred, ok := InferVerifier(root)
		if !ok || inferred.Stages[0].Cmd != "make check" {
			t.Fatalf("inferred = %+v ok=%v", inferred, ok)
		}
	})
	t.Run("makefile without check target falls through", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, root, "Makefile", "build:\n\tgo build\n")
		writeTestFile(t, root, "go.mod", "module x\n")
		inferred, ok := InferVerifier(root)
		if !ok || inferred.Source != "go.mod" {
			t.Fatalf("inferred = %+v ok=%v", inferred, ok)
		}
		if len(inferred.Stages) != 2 || inferred.Stages[1].Cmd != "go test ./..." {
			t.Fatalf("stages = %+v", inferred.Stages)
		}
	})
	t.Run("npm placeholder test script rejected", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, root, "package.json", `{"scripts":{"test":"echo \"Error: no test specified\" && exit 1"}}`)
		if _, ok := InferVerifier(root); ok {
			t.Fatal("placeholder script should not infer")
		}
	})
	t.Run("npm real test script accepted", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, root, "package.json", `{"scripts":{"test":"vitest run"}}`)
		inferred, ok := InferVerifier(root)
		if !ok || inferred.Stages[0].Cmd != "npm test" {
			t.Fatalf("inferred = %+v ok=%v", inferred, ok)
		}
	})
	t.Run("nothing recognizable", func(t *testing.T) {
		if _, ok := InferVerifier(t.TempDir()); ok {
			t.Fatal("empty dir should not infer")
		}
	})
}
