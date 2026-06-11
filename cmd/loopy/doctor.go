package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mjbarefo/loopy/internal/loop"
)

func handleDoctor(cwd string, args []string) error {
	asJSON := false
	for _, arg := range args {
		switch arg {
		case "--json":
			asJSON = true
		default:
			return usagef("usage: loopy doctor [--json]")
		}
	}
	root, err := projectRoot(cwd)
	if err != nil {
		// Doctor should diagnose "not a repo" rather than die on it.
		root = cwd
	}
	checks := loop.Doctor(root)
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(checks); err != nil {
			return err
		}
	} else {
		for _, c := range checks {
			fmt.Printf("%-4s %-12s %s\n", doctorGlyph(c.Status), c.Name, c.Detail)
		}
	}
	if !loop.DoctorHealthy(checks) {
		return fmt.Errorf("doctor found failing checks")
	}
	return nil
}

func doctorGlyph(status string) string {
	switch status {
	case loop.DoctorOK:
		return "ok"
	case loop.DoctorWarn:
		return "warn"
	default:
		return "FAIL"
	}
}
