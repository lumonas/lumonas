package power

import (
	"context"
	"testing"
)

func TestOrderedShutdownStopsJobsBeforePowerAction(t *testing.T) {
	steps, err := OrderedShutdown("poweroff")
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 5 || steps[0].Name != "stop-jobs" || steps[3].Name != "unmount-storage" || steps[4].Args[0] != "poweroff" {
		t.Fatalf("unexpected shutdown plan: %#v", steps)
	}
	seen := make([]string, 0, len(steps))
	if err := ExecuteShutdown(context.Background(), "reboot", func(_ context.Context, command string, args ...string) ([]byte, error) {
		if len(args) == 0 {
			seen = append(seen, command)
		} else {
			seen = append(seen, command+" "+args[0])
		}
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	if seen[0] != "systemctl stop" || seen[len(seen)-1] != "systemctl reboot" {
		t.Fatalf("unexpected command order: %#v", seen)
	}
}

func TestShutdownPolicyRequiresBatteryThreshold(t *testing.T) {
	runtime := 30.0
	unit := UPS{OnBattery: true, RuntimeSec: &runtime}
	if !ShouldShutdown(unit, ShutdownPolicy{Enabled: true, MinimumRuntimeSec: 60}) {
		t.Fatal("runtime threshold should trigger shutdown")
	}
	unit.OnBattery = false
	if ShouldShutdown(unit, ShutdownPolicy{Enabled: true, MinimumRuntimeSec: 60}) {
		t.Fatal("online UPS should not trigger shutdown")
	}
}
