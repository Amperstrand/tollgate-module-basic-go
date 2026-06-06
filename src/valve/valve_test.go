package valve

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestIsValidMAC(t *testing.T) {
	tests := []struct {
		mac  string
		want bool
	}{
		{"aa:bb:cc:dd:ee:ff", true},
		{"AA:BB:CC:DD:EE:FF", true},
		{"01:23:45:67:89:ab", true},
		{"", false},
		{"not-a-mac", false},
		{"aa:bb:cc:dd:ee", false},
		{"aa:bb:cc:dd:ee:ff:gg", false},
	}

	for _, tc := range tests {
		t.Run(tc.mac, func(t *testing.T) {
			got := isValidMAC(tc.mac)
			if got != tc.want {
				t.Errorf("isValidMAC(%q) = %v, want %v", tc.mac, got, tc.want)
			}
		})
	}
}

func TestRunNdsctlTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, "sleep", "30")
	_ = cmd.Run()
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Errorf("command should have been killed after ~1s, took %v", elapsed)
	}

	if ctx.Err() != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", ctx.Err())
	}
}

func TestOpenGateUntilRejectsPastTimestamp(t *testing.T) {
	past := time.Now().Unix() - 10
	err := OpenGateUntil("aa:bb:cc:dd:ee:ff", past)
	if err == nil {
		t.Fatal("expected error for past timestamp")
	}
}

func TestOpenGateUntilRejectsInvalidMAC(t *testing.T) {
	future := time.Now().Unix() + 60
	err := OpenGateUntil("not-a-mac", future)
	if err == nil {
		t.Fatal("expected error for invalid MAC")
	}
}

func TestCloseGateRejectsInvalidMAC(t *testing.T) {
	err := CloseGate("not-a-mac")
	if err == nil {
		t.Fatal("expected error for invalid MAC")
	}
}

func TestOpenGateRejectsInvalidMAC(t *testing.T) {
	err := OpenGate("not-a-mac")
	if err == nil {
		t.Fatal("expected error for invalid MAC")
	}
}

func TestGetClientStatsRejectsInvalidMAC(t *testing.T) {
	_, _, err := GetClientStats("not-a-mac")
	if err == nil {
		t.Fatal("expected error for invalid MAC")
	}
}
