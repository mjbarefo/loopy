package loop

import (
	"fmt"
	"strings"
)

// slugMaxWords keeps loop IDs short enough to type while staying recognizable.
const slugMaxWords = 5

// Slugify turns free text into a lowercase, hyphenated identifier capped at
// slugMaxWords words.
func Slugify(text string) string {
	var words []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}
	for _, r := range strings.ToLower(text) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			current.WriteRune(r)
		default:
			flush()
		}
		if len(words) == slugMaxWords {
			break
		}
	}
	flush()
	if len(words) > slugMaxWords {
		words = words[:slugMaxWords]
	}
	if len(words) == 0 {
		return "loop"
	}
	return strings.Join(words, "-")
}

// UniqueLoopID slugifies the goal and disambiguates against existing loop IDs
// with a -2, -3, ... suffix.
func UniqueLoopID(goal string, existing []string) string {
	base := Slugify(goal)
	taken := make(map[string]bool, len(existing))
	for _, id := range existing {
		taken[id] = true
	}
	if !taken[base] {
		return base
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", base, n)
		if !taken[candidate] {
			return candidate
		}
	}
}
