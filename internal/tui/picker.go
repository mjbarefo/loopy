package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mjbarefo/loopy/internal/loop"
)

// The repo picker: bare `loopy` outside a git repository, with repositories
// discovered nearby. Instead of a dead-end message the front door becomes a
// chooser — pick a repo, land in its monitor (onboarding takes over from
// there), or press g to git-init right here. The picker renders nothing but
// the choice; all repo state stays untouched until the monitor runs.

type pickerState struct {
	width, height int
	color         bool
	start         string // the non-repo directory loopy was launched from
	repos         []loop.RepoCandidate
	selected      int

	choice   string // the picked repo root; empty until enter
	initHere bool   // g: git init in start instead
}

// PickRepo runs the chooser and returns the selected repo root, or
// initHere=true when the user asked to git-init the current directory.
// Both are zero when the user quit.
func PickRepo(start string, repos []loop.RepoCandidate, color bool) (choice string, initHere bool, err error) {
	m := pickerModel{pickerState{
		width: 80, height: 24,
		color: color, start: start, repos: repos,
	}}
	res, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", false, err
	}
	if final, ok := res.(pickerModel); ok {
		return final.choice, final.initHere, nil
	}
	return "", false, nil
}

type pickerModel struct{ pickerState }

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "enter":
			if len(m.repos) > 0 {
				m.choice = m.repos[m.selected].Path
			}
			return m, tea.Quit
		case "g":
			m.initHere = true
			return m, tea.Quit
		case "up", "k":
			m.selected = clamp(m.selected-1, 0, len(m.repos)-1)
			return m, nil
		case "down", "j":
			m.selected = clamp(m.selected+1, 0, len(m.repos)-1)
			return m, nil
		}
	}
	return m, nil
}

func (m pickerModel) View() tea.View {
	v := tea.NewView(renderPicker(m.pickerState))
	v.AltScreen = true
	return v
}

// renderPicker is pure: pickerState in, one full frame out — the same
// contract as renderFrame, so geometry stays testable.
func renderPicker(s pickerState) string {
	if s.width < minWidth || s.height < minHeight {
		return fmt.Sprintf("terminal too small for the monitor (need at least %dx%d)\n", minWidth, minHeight)
	}
	var lines []cell

	// The identity block, when there is room for it.
	if s.height >= 18 {
		for i, art := range logoArt {
			line := styled(s.color, sgrCyan, " "+art)
			switch i {
			case 1:
				line = joinCells(line, styled(s.color, sgrBold, "   l o o p y"))
			case 3:
				line = joinCells(line, styled(s.color, sgrDim, "   "+logoTagline))
			}
			lines = append(lines, line)
		}
		lines = append(lines, cell{})
	}

	lines = append(lines,
		joinCells(
			plainCell(" "+abbrevHome(s.start)+" is not a git repository — "),
			styled(s.color, sgrBold, "pick where loops should live:"),
		),
		cell{},
	)

	// The list, windowed around the selection like the monitor's rail.
	footer := 4 // blank, action line, blank, key hints
	rows := s.height - len(lines) - footer
	if rows < 1 {
		rows = 1
	}
	start := 0
	if len(s.repos) > rows && s.selected >= rows {
		start = s.selected - rows + 1
	}
	pathW := 0
	for _, r := range s.repos {
		if w := loop.DisplayWidth(abbrevHome(r.Path)); w > pathW {
			pathW = w
		}
	}
	if pathW > s.width-16 {
		pathW = s.width - 16
	}
	for i := start; i < len(s.repos) && i-start < rows; i++ {
		r := s.repos[i]
		marker := styled(s.color, sgrCyan, " ▶ ")
		path := styled(s.color, sgrBold, loop.PadDisplay(abbrevHome(r.Path), pathW))
		if i != s.selected {
			marker = plainCell("   ")
			path = plainCell(loop.PadDisplay(abbrevHome(r.Path), pathW))
		}
		note := ""
		if r.Loops > 0 {
			note = fmt.Sprintf("  %d loop(s)", r.Loops)
		}
		lines = append(lines, joinCells(marker, path, styled(s.color, sgrDim, note)))
	}

	lines = append(lines, cell{})
	if len(s.repos) > 0 {
		action := "enter opens the monitor in " + abbrevHome(s.repos[s.selected].Path)
		lines = append(lines, styled(s.color, sgrCyan, " "+loop.TruncateDisplay(action, s.width-1)))
	}
	lines = append(lines, cell{})
	lines = append(lines, styled(s.color, sgrDim, " ↑↓ choose · enter open · g git init here · q quit"))

	var b strings.Builder
	for i := 0; i < s.height; i++ {
		b.WriteString(padCell(lineAt(lines, i), s.width) + "\n")
	}
	return b.String()
}

// abbrevHome shortens the home directory prefix to ~ for display.
func abbrevHome(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(dir, home+string(os.PathSeparator)); ok {
		return "~" + string(os.PathSeparator) + rest
	}
	return dir
}
