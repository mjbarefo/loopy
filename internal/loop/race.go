package loop

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
)

// Race mode: N loops, one per agent, same goal, parallel worktrees. First
// green does not auto-win — when everyone has parked, the judge ranks the
// evidence and "no safe winner" is a legitimate verdict.

// RaceRecord is the durable result at .loopy/races/<race-id>/race.json.
type RaceRecord struct {
	ID        string   `json:"id"`
	Goal      string   `json:"goal"`
	Agents    []string `json:"agents"`
	Loops     []string `json:"loops"`
	Verdict   Verdict  `json:"verdict"`
	CreatedAt string   `json:"created_at"`
}

// RacesDir is where race records live.
const RacesDir = "races"

func racePath(root, raceID string) string {
	return filepath.Join(root, LoopyDir, RacesDir, raceID, "race.json")
}

// RunRace creates one loop per agent, drives all engines in parallel, then
// judges the parked results and records the verdict. The per-loop events
// callback lets the caller prefix progress lines; it may return zero Events.
func RunRace(root string, opts CreateOptions, agents []string, events func(loopID, agent string) Events) (RaceRecord, error) {
	if len(agents) < 2 {
		return RaceRecord{}, errors.New("a race needs at least two agents (--race a,b)")
	}
	seen := map[string]bool{}
	for _, a := range agents {
		if seen[a] {
			return RaceRecord{}, fmt.Errorf("agent %s appears twice in the race", a)
		}
		seen[a] = true
	}

	// Worktree creation hits git; serialize it. Engines then run parallel.
	loops := make([]Loop, 0, len(agents))
	for _, agent := range agents {
		o := opts
		o.Agent = agent
		o.IDHint = opts.Goal + " " + agent
		l, err := CreateLoop(root, o)
		if err != nil {
			return RaceRecord{}, fmt.Errorf("creating %s's loop: %w", agent, err)
		}
		loops = append(loops, l)
	}

	var wg sync.WaitGroup
	errs := make([]error, len(loops))
	for i := range loops {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var ev Events
			if events != nil {
				ev = events(loops[i].ID, loops[i].Agent)
			}
			final, err := RunEngine(root, loops[i].ID, ev)
			if err != nil {
				// Park the casualty so the judge can rank what remains;
				// the error itself is preserved in the parked reason.
				_ = ParkAborted(root, loops[i].ID, fmt.Sprintf("engine error: %v", err))
				errs[i] = err
				return
			}
			loops[i] = final
		}(i)
	}
	wg.Wait()

	ids := make([]string, len(loops))
	for i, l := range loops {
		ids[i] = l.ID
	}
	verdict, err := Judge(root, ids)
	if err != nil {
		return RaceRecord{}, fmt.Errorf("the race finished but judging failed: %w", err)
	}

	raceIDs, err := raceIDsInUse(root)
	if err != nil {
		return RaceRecord{}, err
	}
	record := RaceRecord{
		ID:        UniqueLoopID(opts.Goal+" race", raceIDs),
		Goal:      opts.Goal,
		Agents:    agents,
		Loops:     ids,
		Verdict:   verdict,
		CreatedAt: utcNowISO(),
	}
	if err := WriteJSON(racePath(root, record.ID), record); err != nil {
		return RaceRecord{}, err
	}
	for _, e := range errs {
		if e != nil {
			return record, fmt.Errorf("race recorded, but an engine failed: %w", e)
		}
	}
	return record, nil
}

func raceIDsInUse(root string) ([]string, error) {
	dir := filepath.Join(root, LoopyDir, RacesDir)
	entries, err := readDirIfExists(dir)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}
