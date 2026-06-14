package loop

import (
	"context"
	"strings"
)

// GateStageName labels the deterministic command gate the engine designs in
// the background for an ask-only loop. It runs first (fast, key-free,
// short-circuits) so the ask stage only answers once the gate is green.
const GateStageName = "gate"

// isAskOnly reports a verifier made entirely of ask stages with at least one
// ask. Only such a verifier is eligible for a background gate: a loop that
// already has a command stage made a deliberate choice loopy must not override.
func isAskOnly(stages []Stage) bool {
	hasAsk := false
	for _, s := range stages {
		if s.kind() == KindCommand {
			return false
		}
		if s.kind() == KindAsk {
			hasAsk = true
		}
	}
	return hasAsk
}

// startGateSynthesis kicks a background agent that designs a deterministic
// command gate for an ask-only loop and returns it on the channel when ready.
// The loop runs instantly meanwhile; the engine folds the gate in additively
// at an iteration boundary. Returns a nil channel (and a no-op stop) when the
// loop is not eligible, so the caller can treat "no synthesis" uniformly.
//
// The returned stop cancels an in-flight synthesis and tears its throwaway
// worktree down — call it when the engine returns. The goroutine never writes
// loop state (the engine is the single writer) and emits no events: a failed
// or cancelled synthesis is a silent no-op that leaves the ask-only verifier
// in place, and the engine notes the fold-in itself when it drains the channel.
func startGateSynthesis(root string, l Loop) (<-chan Stage, func()) {
	if !l.AutoGate || !isAskOnly(l.Verifier) {
		return nil, func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan Stage, 1)
	go func() {
		res, err := SynthesizeVerifier(ctx, root, l.Agent, l.Goal)
		if err != nil {
			return // keep the ask-only verifier; cancelled or failed is a no-op
		}
		gate, ok := gateFromSynthesis(res)
		if !ok {
			return
		}
		select {
		case ch <- gate:
		case <-ctx.Done():
		}
	}()
	return ch, cancel
}

// gateFromSynthesis turns a synthesis result into a gate stage, or reports
// that there is none worth folding in. A proposal that already passes at HEAD
// is no gate at all — it does not test the goal — so it is dropped rather than
// folded in as a no-op. The channel is never closed on a drop: the engine's
// non-blocking receive must not fire on a zero-value gate.
func gateFromSynthesis(res SynthesisResult) (Stage, bool) {
	cmd := strings.TrimSpace(res.Cmd)
	if cmd == "" || res.AlreadyGreen {
		return Stage{}, false
	}
	return Stage{Name: GateStageName, Cmd: cmd}, true
}

// foldInGate prepends a synthesized gate before the ask stage(s) so the cheap
// deterministic check short-circuits ahead of the agent call. It is additive:
// the loop already runs under a human-approved ask verifier, and the new gate
// only makes green stricter — never auto-accepts — so no re-sign-off is owed
// (the human still seals at review). The engine is the single writer of loop
// state, so this only runs on the engine's own goroutine at a boundary.
func foldInGate(l Loop, gate Stage) Loop {
	l.Verifier = append([]Stage{gate}, l.Verifier...)
	return l
}
