package loop

import (
	"testing"
	"time"
)

func TestIsAskOnly(t *testing.T) {
	cases := []struct {
		name   string
		stages []Stage
		want   bool
	}{
		{"ask only", []Stage{{Name: "j", Kind: KindAsk, Ask: "q"}}, true},
		{"command floor", []Stage{{Name: "c", Cmd: "true"}}, false},
		{"hybrid has command", []Stage{{Name: "c", Cmd: "true"}, {Name: "j", Kind: KindAsk, Ask: "q"}}, false},
		{"empty", nil, false},
	}
	for _, c := range cases {
		if got := isAskOnly(c.stages); got != c.want {
			t.Errorf("%s: isAskOnly = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestGateFromSynthesis(t *testing.T) {
	if g, ok := gateFromSynthesis(SynthesisResult{Cmd: "make check"}); !ok || g.Name != GateStageName || g.Cmd != "make check" {
		t.Fatalf("a real red proposal should fold in as the gate; got %+v ok=%v", g, ok)
	}
	if _, ok := gateFromSynthesis(SynthesisResult{Cmd: "true", AlreadyGreen: true}); ok {
		t.Fatal("a proposal that already passes is no gate and must be dropped")
	}
	if _, ok := gateFromSynthesis(SynthesisResult{Cmd: "   "}); ok {
		t.Fatal("an empty proposal must be dropped")
	}
}

func TestFoldInGatePrependsBeforeAsk(t *testing.T) {
	l := Loop{Verifier: []Stage{{Name: "judge", Kind: KindAsk, Ask: "q"}}}
	l = foldInGate(l, Stage{Name: GateStageName, Cmd: "test -f done.txt"})
	if len(l.Verifier) != 2 {
		t.Fatalf("verifier = %+v", l.Verifier)
	}
	if l.Verifier[0].Name != GateStageName || l.Verifier[0].kind() != KindCommand {
		t.Fatalf("the gate must run first as a command stage: %+v", l.Verifier[0])
	}
	if l.Verifier[1].kind() != KindAsk {
		t.Fatalf("the ask stage must stay last so it only runs once the gate is green: %+v", l.Verifier[1])
	}
}

// startGateSynthesis returns a nil channel for anything but an eligible
// AutoGate ask-only loop — the engine then never folds a gate in.
func TestStartGateSynthesisEligibility(t *testing.T) {
	root := newLoopProject(t, `echo true`)
	ask := []Stage{{Name: "j", Kind: KindAsk, Ask: "q"}}
	cmd := []Stage{{Name: "c", Cmd: "true"}}

	cases := []struct {
		name    string
		l       Loop
		wantNil bool
	}{
		{"eligible", Loop{Agent: "scripted", Goal: "g", AutoGate: true, Verifier: ask}, false},
		{"autogate off", Loop{Agent: "scripted", Goal: "g", AutoGate: false, Verifier: ask}, true},
		{"has command gate", Loop{Agent: "scripted", Goal: "g", AutoGate: true, Verifier: cmd}, true},
	}
	for _, c := range cases {
		ch, stop := startGateSynthesis(root, c.l)
		if (ch == nil) != c.wantNil {
			t.Errorf("%s: channel nil = %v, want %v", c.name, ch == nil, c.wantNil)
		}
		stop()
	}
}

// The background agent designs a real gate and delivers it on the channel.
func TestStartGateSynthesisDeliversGate(t *testing.T) {
	root := newLoopProject(t, `printf 'exploring...\ntest -f done.txt\n'`)
	l := Loop{Agent: "scripted", Goal: "create done.txt", AutoGate: true,
		Verifier: []Stage{{Name: "j", Kind: KindAsk, Ask: "done?"}}}
	ch, stop := startGateSynthesis(root, l)
	defer stop()
	if ch == nil {
		t.Fatal("an eligible loop must start synthesis")
	}
	select {
	case gate := <-ch:
		if gate.Name != GateStageName || gate.Cmd != "test -f done.txt" {
			t.Fatalf("delivered gate = %+v", gate)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("synthesis did not deliver a gate in time")
	}
}

// End to end: an AutoGate ask-only loop folds the agent-designed gate into its
// live verifier. The one scripted agent plays three roles, branching on the
// prompt it is handed: propose a gate, judge (never green here), or do work.
func TestEngineFoldsInBackgroundGate(t *testing.T) {
	agent := `if grep -q "POSIX shell command" {prompt_file}; then
  printf 'false\n'
elif grep -q "FINAL output line EXACTLY" {prompt_file}; then
  printf 'FAIL: not done at %s\n' "$(date +%s%N)"
else
  date +%s%N >> work.txt
fi`
	root := newLoopProject(t, agent)
	l := mustCreate(t, root, CreateOptions{
		Goal:     "do the thing",
		AutoGate: true,
		Verifier: []Stage{{Name: "judge", Kind: KindAsk, Ask: "done?"}},
		Budget:   Budget{MaxIterations: 5},
	})
	final, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusParked {
		t.Fatalf("status = %q, want parked (the ask never greens): %q", final.Status, final.ParkedReason)
	}
	// The verifier the loop ended on must carry the agent-designed gate, ahead
	// of the ask stage it was created with.
	if len(final.Verifier) != 2 || final.Verifier[0].Name != GateStageName {
		t.Fatalf("verifier = %+v, want the gate folded in front of the ask", final.Verifier)
	}
	if final.Verifier[0].Cmd != "false" {
		t.Fatalf("folded gate cmd = %q, want the agent's proposal", final.Verifier[0].Cmd)
	}
}
