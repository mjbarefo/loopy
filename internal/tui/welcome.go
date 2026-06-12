package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mjbarefo/loopy/internal/loop"
)

// The welcome splash: shown once when the monitor is launched as bare
// `loopy`, dismissed by any key. Branding lives here and in the empty
// state — never in the working frames.

var logoArt = []string{
	"██░░██░░░░██░░██",
	"██░░░░██████░░░░██",
	"██░░░░██░░██░░░░██",
	"██░░░░██████░░░░██",
	"██░░██░░░░██░░██",
}

const logoTagline = "engineer loops, not prompts"

// FrontDoor is bare `loopy` outside a git repository: the same identity as
// the splash, the problem in one line, and the exact next move — instead of
// the full help wall with an error buried under it. CLI output, not a frame:
// the caller prints it and the user lands back at their prompt, ready to cd.
func FrontDoor(color bool, dir string) string {
	text := func(c cell) string {
		if c.styled != "" {
			return c.styled
		}
		return c.plain
	}
	var b strings.Builder
	b.WriteString("\n")
	for i, art := range logoArt {
		line := styled(color, sgrCyan, " "+art)
		switch i {
		case 1:
			line = joinCells(line, styled(color, sgrBold, "   l o o p y"))
		case 3:
			line = joinCells(line, styled(color, sgrDim, "   "+logoTagline))
		}
		b.WriteString(text(line) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(" " + dir + " is not a git repository — loops live inside one.\n\n")
	b.WriteString(text(styled(color, sgrCyan, "   cd into the repo you want loops in, then run: loopy")) + "\n")
	b.WriteString(text(styled(color, sgrDim, "   starting fresh? git init first — loopy sets up the rest")) + "\n")
	b.WriteString("\n")
	b.WriteString(text(styled(color, sgrDim, " full command surface: loopy help")) + "\n")
	return b.String()
}

// welcomeFrame is a full-screen, vertically centered splash.
func welcomeFrame(s frameState, root string) string {
	if s.width < minWidth || s.height < minHeight {
		return renderFrame(s) // tiny terminals skip the ceremony
	}

	var lines []cell
	wordmark := "l o o p y"
	if s.width >= 64 {
		// Logo block with the wordmark and tagline alongside, like the README.
		for i, art := range logoArt {
			var side cell
			switch i {
			case 1:
				side = styled(s.color, sgrBold, "   "+wordmark)
			case 3:
				side = styled(s.color, sgrDim, "   "+logoTagline)
			}
			lines = append(lines, joinCells(styled(s.color, sgrCyan, art), side))
		}
	} else {
		lines = append(lines,
			styled(s.color, sgrBold, wordmark),
			styled(s.color, sgrDim, logoTagline),
		)
	}
	lines = append(lines, cell{}, cell{})

	repo := filepath.Base(root)
	info := fmt.Sprintf("%s · repo %s", loop.ResolvedVersion(), repo)
	if n := len(s.loops); n > 0 {
		live := 0
		for _, v := range s.loops {
			if v.Status == loop.StatusRunning && v.Live {
				live++
			}
		}
		info += fmt.Sprintf(" · %d loop(s)", n)
		if live > 0 {
			info += fmt.Sprintf(", %d running", live)
		}
	}
	lines = append(lines, styled(s.color, sgrDim, info))

	hint := "press any key for the monitor · q quits"
	switch {
	case !s.initialized:
		hint = "press any key — the monitor will set this repo up"
	case !s.agentsRegistered:
		hint = "press any key — one step left: register an agent"
	}
	lines = append(lines, cell{}, styled(s.color, sgrCyan, hint))

	// Center the block.
	blockW := 0
	for _, l := range lines {
		if w := loop.DisplayWidth(l.plain); w > blockW {
			blockW = w
		}
	}
	leftPad := (s.width - blockW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	topPad := (s.height - len(lines)) / 2
	if topPad < 0 {
		topPad = 0
	}

	var b strings.Builder
	for i := 0; i < topPad; i++ {
		b.WriteString("\n")
	}
	pad := strings.Repeat(" ", leftPad)
	for _, l := range lines {
		b.WriteString(pad + padCell(l, s.width-leftPad) + "\n")
	}
	for i := topPad + len(lines); i < s.height; i++ {
		b.WriteString("\n")
	}
	return b.String()
}
