package loop

// Terminal display-width helpers. Layout in the plain renderers and the
// monitor budgets by terminal columns: CJK and emoji are two columns wide,
// combining marks and control characters are zero. East Asian Ambiguous
// characters count as narrow. Stdlib only — the wide ranges are hand-rolled.

// DisplayWidth returns the number of terminal columns s occupies.
func DisplayWidth(s string) int {
	panic("not implemented")
}

// TruncateDisplay truncates s to at most max columns, appending "…" when
// anything was cut. The result never exceeds max columns.
func TruncateDisplay(s string, max int) string {
	panic("not implemented")
}

// PadDisplay pads s with trailing spaces to exactly width columns,
// truncating first when s is too wide.
func PadDisplay(s string, width int) string {
	panic("not implemented")
}
