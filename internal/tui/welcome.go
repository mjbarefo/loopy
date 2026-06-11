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
