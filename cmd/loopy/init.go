package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mjbarefo/loopy/internal/loop"
)

func handleInit(cwd string, args []string) error {
	if len(args) > 0 {
		return usagef("init takes no arguments")
	}
	root, err := projectRoot(cwd)
	if err != nil {
		return err
	}
	path, err := loop.InitProject(root)
	if err != nil {
		return err
	}
	fmt.Printf("initialized %s (.loopy/ added to .gitignore)\n", path)

	offerDetectedAgents(root)

	reg, err := loop.LoadAgents(root)
	if err == nil && len(reg.Agents) == 0 {
		fmt.Println("\nnext: register an agent, e.g.")
		fmt.Println(`  loopy agent add claude --cmd "claude -p {prompt} --permission-mode acceptEdits" --default`)
	}
	return nil
}

// detectedAgentSuggestions maps agent CLIs found on PATH to suggested
// headless invocations. Suggestions, not gospel: the headless-flag matrix is
// documented as untested in DESIGN.md.
var detectedAgentSuggestions = []struct {
	binary string
	name   string
	cmd    string
}{
	{"claude", "claude", "claude -p {prompt} --permission-mode acceptEdits"},
	{"codex", "codex", "codex exec {prompt}"},
	{"gemini", "gemini", "gemini -p {prompt} --yolo"},
}

// offerDetectedAgents looks for known agent CLIs and, on a TTY, offers to
// register each. Non-interactive runs just print what was found.
func offerDetectedAgents(root string) {
	reg, err := loop.LoadAgents(root)
	if err != nil {
		return
	}
	interactive := isTTY(os.Stdin)
	reader := bufio.NewReader(os.Stdin)
	for _, s := range detectedAgentSuggestions {
		if _, exists := reg.Agents[s.name]; exists {
			continue
		}
		if _, err := exec.LookPath(s.binary); err != nil {
			continue
		}
		if !interactive {
			fmt.Printf("found %s on PATH; register it with:\n  loopy agent add %s --cmd %q\n", s.binary, s.name, s.cmd)
			continue
		}
		fmt.Printf("found %s on PATH — register as agent %q with command:\n  %s\nregister? [Y/n] ", s.binary, s.name, s.cmd)
		line, readErr := reader.ReadString('\n')
		if readErr != nil && line == "" {
			// EOF: stdin isn't really interactive (e.g. </dev/null passes the
			// char-device check); fall back to printing the command.
			fmt.Printf("\nno input; register it later with:\n  loopy agent add %s --cmd %q\n", s.name, s.cmd)
			continue
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer == "" || answer == "y" || answer == "yes" {
			if err := loop.AddAgent(root, s.name, s.cmd, false); err != nil {
				fmt.Fprintf(os.Stderr, "loopy: could not register %s: %v\n", s.name, err)
				continue
			}
			fmt.Printf("registered %s\n", s.name)
		}
	}
}

func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
