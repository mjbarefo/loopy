package loop

import "testing"

// The monitor and the plain renderers truncate and pad text by terminal
// columns, not runes or bytes: CJK and emoji occupy two columns, combining
// marks none. East Asian Ambiguous characters (·, ●, box drawing) count as
// narrow, which is how non-CJK terminal setups render them.

func TestDisplayWidth(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"loop-id-42", 10},
		{"…", 1},
		{"héllo", 5},  // precomposed é
		{"é", 1},     // combining acute: one column
		{"a\x01b", 2}, // control characters: zero columns
		{"引用符", 6},    // CJK Unified Ideographs: wide
		{"カタカナ", 8},   // Katakana: wide
		{"ひらがな", 8},   // Hiragana: wide
		{"한글", 4},     // Hangul syllables: wide
		{"ＡＢ", 4},     // Fullwidth forms: wide
		{"。、「」", 8},   // CJK punctuation: wide
		{"🌀", 2},      // emoji: wide
		{"🤖🦾", 4},     // supplementary-plane emoji: wide
		{"a🌀b", 4},
		{"·", 1},   // ambiguous: narrow
		{"●", 1},   // ambiguous: narrow
		{"│┌┴", 3}, // box drawing: narrow
		{"mixed 引用 and 🌀", 17},
	}
	for _, c := range cases {
		if got := DisplayWidth(c.in); got != c.want {
			t.Errorf("DisplayWidth(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestTruncateDisplay(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"", 5, ""},
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel…"},
		{"hello", 1, "…"},
		{"hello", 0, ""},
		{"héllo world", 6, "héllo…"},
		{"引用符付き", 10, "引用符付き"},
		{"引用符付き", 6, "引用…"}, // 引用 (4) + … (1) = 5; 符 would overflow
		{"引用符付き", 5, "引用…"},
		{"🌀🌀", 4, "🌀🌀"},
		{"🌀🌀", 3, "🌀…"},
		{"🌀🌀", 2, "…"},     // a wide rune cannot squeeze next to the ellipsis
		{"ábc", 2, "á…"}, // combining mark travels with its base
	}
	for _, c := range cases {
		if got := TruncateDisplay(c.in, c.max); got != c.want {
			t.Errorf("TruncateDisplay(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
		}
		if w := DisplayWidth(TruncateDisplay(c.in, c.max)); w > c.max {
			t.Errorf("TruncateDisplay(%q, %d) is %d columns wide", c.in, c.max, w)
		}
	}
}

func TestPadDisplay(t *testing.T) {
	cases := []struct {
		in    string
		width int
		want  string
	}{
		{"", 3, "   "},
		{"ab", 5, "ab   "},
		{"ab", 2, "ab"},
		{"abcdef", 4, "abc…"}, // over-wide input truncates
		{"引用", 6, "引用  "},     // wide runes pad by remaining columns
		{"引用符", 5, "引用…"},     // truncation keeps the exact width budget
	}
	for _, c := range cases {
		got := PadDisplay(c.in, c.width)
		if got != c.want {
			t.Errorf("PadDisplay(%q, %d) = %q, want %q", c.in, c.width, got, c.want)
		}
		if w := DisplayWidth(got); w > c.width {
			t.Errorf("PadDisplay(%q, %d) is %d columns wide", c.in, c.width, w)
		}
	}
}
