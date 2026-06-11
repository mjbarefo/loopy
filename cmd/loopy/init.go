package main

import (
	"bufio"
	"fmt"
	"os"
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
	path, ignoredNow, err := loop.InitProject(root)
	if err != nil {
		return err
	}
	if ignoredNow {
		fmt.Printf("initialized %s (.loopy/ added to .gitignore — commit that)\n", path)
	} else {
		fmt.Printf("initialized %s\n", path)
	}

	offerDetectedAgents(root)

	reg, err := loop.LoadAgents(root)
	if err == nil && len(reg.Agents) == 0 {
		fmt.Println("\nnext: register an agent, e.g.")
		fmt.Println(`  loopy agent add claude --cmd "claude -p {prompt} --permission-mode acceptEdits" --default`)
	}
	return nil
}

// offerDetectedAgents looks for known agent CLIs (loop.KnownAgentCLIs, the
// tested matrix in docs/agents.md) and, on a TTY, offers to register each.
// Non-interactive runs just print what was found.
func offerDetectedAgents(root string) {
	interactive := isTTY(os.Stdin)
	reader := bufio.NewReader(os.Stdin)
	for _, s := range loop.DetectAgentCLIs(root) {
		if !interactive {
			fmt.Printf("found %s on PATH; register it with:\n  loopy agent add %s --cmd %q\n", s.Binary, s.Name, s.Cmd)
			continue
		}
		fmt.Printf("found %s on PATH — register as agent %q with command:\n  %s\nregister? [Y/n] ", s.Binary, s.Name, s.Cmd)
		line, readErr := reader.ReadString('\n')
		if readErr != nil && line == "" {
			// EOF: stdin isn't really interactive (e.g. </dev/null passes the
			// char-device check); fall back to printing the command.
			fmt.Printf("\nno input; register it later with:\n  loopy agent add %s --cmd %q\n", s.Name, s.Cmd)
			continue
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer == "" || answer == "y" || answer == "yes" {
			if err := loop.AddAgent(root, s.Name, s.Cmd, false); err != nil {
				fmt.Fprintf(os.Stderr, "loopy: could not register %s: %v\n", s.Name, err)
				continue
			}
			fmt.Printf("registered %s\n", s.Name)
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
