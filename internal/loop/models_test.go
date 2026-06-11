package loop

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDurationRoundTrip(t *testing.T) {
	d := Duration(90 * time.Minute)
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"1h30m0s"` {
		t.Fatalf("marshal = %s", data)
	}
	var back Duration
	if err := json.Unmarshal([]byte(`"30m"`), &back); err != nil {
		t.Fatal(err)
	}
	if time.Duration(back) != 30*time.Minute {
		t.Fatalf("unmarshal = %v", back)
	}
	if err := json.Unmarshal([]byte(`1800`), &back); err == nil {
		t.Fatal("expected error for numeric duration")
	}
	if err := json.Unmarshal([]byte(`"not-a-duration"`), &back); err == nil {
		t.Fatal("expected error for garbage duration")
	}
}

func TestLoopDone(t *testing.T) {
	for status, want := range map[string]bool{
		StatusRunning:  false,
		StatusPaused:   false,
		StatusGreen:    true,
		StatusParked:   true,
		StatusAccepted: true,
		StatusRejected: true,
	} {
		if got := (Loop{Status: status}).Done(); got != want {
			t.Errorf("Done() with %s = %v, want %v", status, got, want)
		}
	}
}
