package power

import (
	"context"
	"testing"
)

func TestDiscoverParsesNUTOutput(t *testing.T) {
	runner := func(_ context.Context, command string, args ...string) ([]byte, error) {
		if command != "upsc" || len(args) != 1 {
			t.Fatalf("unexpected command %s %v", command, args)
		}
		return []byte("device.mfr: APC\ndevice.model: Back-UPS\nbattery.charge: 97\nups.load: 12\nbattery.runtime: 845\nups.status: OB\n"), nil
	}
	items := Discover(context.Background(), []string{"ups-a"}, runner)
	if len(items) != 1 || items[0].Name != "ups-a" || items[0].Status != "OB" || !items[0].OnBattery {
		t.Fatalf("unexpected UPS: %#v", items)
	}
	if items[0].ChargePercent == nil || *items[0].ChargePercent != 97 {
		t.Fatalf("charge was not parsed: %#v", items[0].ChargePercent)
	}
}

func TestDiscoverListsUPSWhenNamesOmitted(t *testing.T) {
	calls := 0
	runner := func(_ context.Context, command string, args ...string) ([]byte, error) {
		calls++
		if len(args) == 1 && args[0] == "-l" {
			return []byte("zeta\nalpha\n"), nil
		}
		return []byte("ups.status: OL\n"), nil
	}
	items := Discover(context.Background(), nil, runner)
	if calls != 3 || len(items) != 2 || items[0].Name != "alpha" || items[1].Name != "zeta" {
		t.Fatalf("unexpected discovery: calls=%d items=%#v", calls, items)
	}
}
