package loop

// Terminal display-width helpers. Layout in the plain renderers and the
// monitor budgets by terminal columns: CJK and emoji are two columns wide,
// combining marks and control characters are zero. East Asian Ambiguous
// characters count as narrow. Stdlib only — the wide ranges are hand-rolled.

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// wideRanges holds inclusive rune intervals that render two columns wide:
// the East Asian Wide and Fullwidth blocks plus the emoji blocks. East Asian
// Ambiguous characters are deliberately absent — they count as narrow.
var wideRanges = [...][2]rune{
	{0x1100, 0x115F},   // Hangul Jamo leading consonants
	{0x2E80, 0x303E},   // CJK radicals, Kangxi radicals, CJK punctuation
	{0x3041, 0x33FF},   // Hiragana, Katakana, Hangul Jamo compat, CJK compat
	{0x3400, 0x4DBF},   // CJK Unified Ideographs Extension A
	{0x4E00, 0x9FFF},   // CJK Unified Ideographs
	{0xA000, 0xA4CF},   // Yi Syllables and Radicals
	{0xAC00, 0xD7A3},   // Hangul Syllables
	{0xF900, 0xFAFF},   // CJK Compatibility Ideographs
	{0xFE10, 0xFE19},   // Vertical Forms
	{0xFE30, 0xFE6F},   // CJK Compatibility Forms, Small Form Variants
	{0xFF00, 0xFF60},   // Fullwidth Forms
	{0xFFE0, 0xFFE6},   // Fullwidth signs
	{0x1F300, 0x1F64F}, // emoji: pictographs and emoticons
	{0x1F680, 0x1F6FF}, // emoji: transport and map symbols
	{0x1F900, 0x1F9FF}, // Supplemental Symbols and Pictographs
	{0x1FA70, 0x1FAFF}, // Symbols and Pictographs Extended-A
	{0x20000, 0x3FFFD}, // CJK Unified Ideographs Extensions B and beyond
}

func runeDisplayWidth(r rune) int {
	if r < 0x20 || (r >= 0x7F && r < 0xA0) {
		return 0
	}
	if unicode.In(r, unicode.Mn, unicode.Me) {
		return 0
	}
	for _, rg := range wideRanges {
		if r >= rg[0] && r <= rg[1] {
			return 2
		}
	}
	return 1
}

// DisplayWidth returns the number of terminal columns s occupies. Width is
// additive over runes.
func DisplayWidth(s string) int {
	width := 0
	for _, r := range s {
		width += runeDisplayWidth(r)
	}
	return width
}

// TruncateDisplay truncates s to at most max columns, appending "…" when
// anything was cut. The result never exceeds max columns.
func TruncateDisplay(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if DisplayWidth(s) <= max {
		return s
	}
	budget := max - 1 // reserve one column for the ellipsis
	width := 0
	end := 0
	for i, r := range s {
		w := runeDisplayWidth(r)
		if width+w > budget {
			break
		}
		width += w
		end = i + utf8.RuneLen(r)
	}
	return s[:end] + "…"
}

// PadDisplay pads s with trailing spaces to exactly width columns,
// truncating first when s is too wide.
func PadDisplay(s string, width int) string {
	w := DisplayWidth(s)
	if w > width {
		s = TruncateDisplay(s, width)
		w = DisplayWidth(s)
	}
	if pad := width - w; pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// WrapDisplay greedily word-wraps s into lines of at most width display
// columns (CJK and emoji count two). Internal whitespace collapses to single
// spaces; a word wider than a line gets a line of its own (renderers truncate
// what they draw, so it stays safe). Always returns at least one line.
func WrapDisplay(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	if width <= 0 {
		return []string{strings.Join(words, " ")}
	}
	lines := []string{words[0]}
	lineW := DisplayWidth(words[0])
	for _, word := range words[1:] {
		w := DisplayWidth(word)
		if lineW+1+w <= width {
			lines[len(lines)-1] += " " + word
			lineW += 1 + w
			continue
		}
		lines = append(lines, word)
		lineW = w
	}
	return lines
}
