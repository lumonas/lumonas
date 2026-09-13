package power

import (
	"context"
	"strings"
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

func TestDiscoverPreservesConfiguredNamesForStatusQueries(t *testing.T) {
	var calls [][]string
	runner := func(_ context.Context, command string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{command}, args...))
		return []byte("ups.status: OL\n"), nil
	}
	items := Discover(context.Background(), []string{"ups-a", "ups-b"}, runner)
	if len(items) != 2 || items[0].Name != "ups-a" || items[1].Name != "ups-b" {
		t.Fatalf("unexpected configured UPS inventory: %#v", items)
	}
	if len(calls) != 2 || calls[0][0] != "upsc" || calls[0][1] != "ups-a" || calls[1][1] != "ups-b" {
		t.Fatalf("configured names were not queried directly: %#v", calls)
	}
}

func TestNormalizeNamesValidatesAndSortsNUTDevices(t *testing.T) {
	names, err := NormalizeNames([]string{"ups@host:3493", " local-ups ", "ups@host:3493"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(names, ","), "local-ups,ups@host:3493"; got != want {
		t.Fatalf("unexpected normalized names %q, want %q", got, want)
	}
	for _, invalid := range [][]string{{"bad name"}, {"ups,other"}, {"../ups"}} {
		if _, err := NormalizeNames(invalid); err == nil {
			t.Fatalf("invalid UPS name accepted: %#v", invalid)
		}
	}
}
