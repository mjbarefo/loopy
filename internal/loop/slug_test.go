package loop

import "testing"

func TestSlugify(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Fix the flaky importer test", "fix-the-flaky-importer-test"},
		{"make the CSV importer handle quoted newlines", "make-the-csv-importer-handle"},
		{"  weird   spacing\tand\nnewlines ", "weird-spacing-and-newlines"},
		{"emoji ∞ and (punctuation)!", "emoji-and-punctuation"},
		{"", "loop"},
		{"!!!", "loop"},
		{"UPPER case 123", "upper-case-123"},
	}
	for _, c := range cases {
		if got := Slugify(c.in); got != c.want {
			t.Errorf("Slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestUniqueLoopID(t *testing.T) {
	existing := []string{"fix-the-bug", "fix-the-bug-2"}
	if got := UniqueLoopID("fix the bug", existing); got != "fix-the-bug-3" {
		t.Errorf("got %q, want fix-the-bug-3", got)
	}
	if got := UniqueLoopID("fix the bug", nil); got != "fix-the-bug" {
		t.Errorf("got %q, want fix-the-bug", got)
	}
}
