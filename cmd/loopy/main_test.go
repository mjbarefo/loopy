package main

import "testing"

func TestExitCodeContract(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"no args is help", nil, exitOK},
		{"help", []string{"help"}, exitOK},
		{"version", []string{"version"}, exitOK},
		{"unknown command", []string{"frobnicate"}, exitUsage},
		{"doctor outside repo still runs", []string{"doctor", "--bogus"}, exitUsage},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := runWithExitCode(c.args); got != c.want {
				t.Fatalf("exit = %d, want %d", got, c.want)
			}
		})
	}
}
