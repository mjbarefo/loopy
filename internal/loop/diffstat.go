package loop

import "strings"

// FileDiff is one file's summary inside a git unified diff: the answer-first
// line a reviewer reads before the patch itself.
type FileDiff struct {
	Path   string
	Kind   string // "new file", "deleted", "renamed", "modified"
	Binary bool
	Adds   int
	Dels   int
}

// ParseDiff summarizes a git unified diff into per-file stats. It is pure and
// tolerant: unknown lines are ignored, and a tail-truncated patch yields
// whatever files remain visible rather than an error.
func ParseDiff(lines []string) []FileDiff {
	var files []FileDiff
	var cur *FileDiff
	inHunk := false
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "diff --git "):
			files = append(files, FileDiff{Kind: "modified", Path: gitDiffPath(l)})
			cur = &files[len(files)-1]
			inHunk = false
		case cur == nil:
			continue
		case strings.HasPrefix(l, "@@"):
			inHunk = true
		// Inside a hunk every +/- is content — an added line may itself start
		// with "++" or "--", so the hunk check must run before the header ones.
		case inHunk && strings.HasPrefix(l, "+"):
			cur.Adds++
		case inHunk && strings.HasPrefix(l, "-"):
			cur.Dels++
		case inHunk:
			continue
		case strings.HasPrefix(l, "new file mode"):
			cur.Kind = "new file"
		case strings.HasPrefix(l, "deleted file mode"):
			cur.Kind = "deleted"
		case strings.HasPrefix(l, "rename to "):
			cur.Kind = "renamed"
			cur.Path = strings.TrimPrefix(l, "rename to ")
		case strings.HasPrefix(l, "Binary files "):
			cur.Binary = true
		case strings.HasPrefix(l, "+++ b/"):
			cur.Path = strings.TrimPrefix(l, "+++ b/")
		}
	}
	return files
}

// gitDiffPath extracts the b-side path from a "diff --git a/x b/x" line; the
// later "+++ b/" header refines it when present (it handles spaces better).
func gitDiffPath(l string) string {
	if i := strings.LastIndex(l, " b/"); i >= 0 {
		return l[i+3:]
	}
	return strings.TrimPrefix(l, "diff --git ")
}
