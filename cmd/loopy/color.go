package main

import "os"

// Minimal semantic color for progress lines. Color is never the only signal:
// every colored token also carries a word or glyph. Honors NO_COLOR and
// non-TTY stdout.
type colorCode string

const (
	green colorCode = "\x1b[32m"
	red   colorCode = "\x1b[31m"
	cyan  colorCode = "\x1b[36m"
	reset           = "\x1b[0m"
)

var colorEnabled = func() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isTTY(os.Stdout)
}()

func colorize(c colorCode, s string) string {
	if !colorEnabled {
		return s
	}
	return string(c) + s + reset
}
