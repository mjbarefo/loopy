package loop

import "testing"

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		name string
		n    int
		want string
	}{
		{"zero", 0, "0 B"},
		{"plain bytes", 512, "512 B"},
		{"just below KiB", 1023, "1023 B"},
		{"exactly one KiB", 1024, "1.0 KiB"},
		{"fractional KiB", 1536, "1.5 KiB"},
		{"just below MiB", 1<<20 - 1, "1024.0 KiB"},
		{"exactly one MiB", 1 << 20, "1.0 MiB"},
		{"fractional MiB", 5<<20 + 1<<19, "5.5 MiB"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := humanBytes(tc.n); got != tc.want {
				t.Errorf("humanBytes(%d) = %q, want %q", tc.n, got, tc.want)
			}
		})
	}
}
