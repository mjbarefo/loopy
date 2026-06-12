package loop

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseDiff(t *testing.T) {
	patch := `diff --git a/docs/README.md b/docs/README.md
new file mode 100644
index 0000000..52ad8d6
--- /dev/null
+++ b/docs/README.md
@@ -0,0 +1,3 @@
+# title
+
+body
diff --git a/internal/loop/engine.go b/internal/loop/engine.go
index 1111111..2222222 100644
--- a/internal/loop/engine.go
+++ b/internal/loop/engine.go
@@ -10,4 +10,4 @@ func RunEngine() {
 context
-old line
+new line
 context
@@ -40,3 +40,4 @@
 context
+++plus-plus content counts as an add
diff --git a/old.go b/renamed.go
similarity index 96%
rename from old.go
rename to renamed.go
diff --git a/img.png b/img.png
Binary files a/img.png and b/img.png differ
diff --git a/gone.go b/gone.go
deleted file mode 100644
index 3333333..0000000
--- a/gone.go
+++ /dev/null
@@ -1,2 +0,0 @@
-bye
-bye`
	got := ParseDiff(strings.Split(patch, "\n"))
	want := []FileDiff{
		{Path: "docs/README.md", Kind: "new file", Adds: 3},
		{Path: "internal/loop/engine.go", Kind: "modified", Adds: 2, Dels: 1},
		{Path: "renamed.go", Kind: "renamed"},
		{Path: "img.png", Kind: "modified", Binary: true},
		{Path: "gone.go", Kind: "deleted", Dels: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseDiff:\n got %+v\nwant %+v", got, want)
	}
}

func TestParseDiffTruncatedHead(t *testing.T) {
	// A tail-loaded patch can start mid-hunk: lines before the first
	// "diff --git" belong to an invisible file and must not panic or count.
	lines := []string{
		"+stray add from a cut-off file",
		"-stray del",
		"diff --git a/a.go b/a.go",
		"index 1..2 100644",
		"--- a/a.go",
		"+++ b/a.go",
		"@@ -1 +1 @@",
		"-x",
		"+y",
	}
	got := ParseDiff(lines)
	want := []FileDiff{{Path: "a.go", Kind: "modified", Adds: 1, Dels: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseDiff truncated:\n got %+v\nwant %+v", got, want)
	}
}

func TestParseDiffEmpty(t *testing.T) {
	if got := ParseDiff(nil); len(got) != 0 {
		t.Errorf("ParseDiff(nil) = %+v, want empty", got)
	}
}
