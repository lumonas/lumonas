package collector

import (
	"testing"
	"time"
)

func TestParseCPUStatIncludesIOWaitInIdleTime(t *testing.T) {
	total, idle := parseCPUStat("cpu  10 20 30 40 5 6 7 8 9\n")
	if total != 135 || idle != 45 {
		t.Fatalf("unexpected CPU counters total=%d idle=%d", total, idle)
	}
}

func TestParseNetworkBytes(t *testing.T) {
	data := "Inter-| Receive                                                | Transmit\n" +
		" face |bytes packets errs drop fifo frame compressed multicast|bytes packets errs drop fifo colls carrier compressed\n" +
		" eth0: 100 1 0 0 0 0 0 0 200 2 0 0 0 0 0 0\n"
	rx, tx, ok := parseNetworkBytes(data, "eth0")
	if !ok || rx != 100 || tx != 200 {
		t.Fatalf("unexpected network counters rx=%d tx=%d ok=%v", rx, tx, ok)
	}
}

func TestNetworkThroughputUsesCounterDelta(t *testing.T) {
	metricState.Lock()
	metricState.netIface = ""
	metricState.netAt = time.Time{}
	metricState.Unlock()
	// The helper reads the host counter file, so this verifies only that an
	// unknown interface fails closed without fabricating throughput.
	up, down := networkThroughput("interface-that-does-not-exist", time.Now())
	if up != 0 || down != 0 {
		t.Fatalf("expected zero throughput for unknown interface, got up=%v down=%v", up, down)
	}
}

func TestClassifyFilesystemUsage(t *testing.T) {
	tests := []struct {
		name        string
		used, total uint64
		expected    string
	}{
		{"healthy", 79, 100, "healthy"},
		{"warning", 80, 100, "warning"},
		{"critical", 95, 100, "critical"},
		{"invalid", 101, 100, "unknown"},
		{"empty", 0, 0, "unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyFilesystemUsage(test.used, test.total); got != test.expected {
				t.Fatalf("classifyFilesystemUsage(%d, %d) = %q, want %q", test.used, test.total, got, test.expected)
			}
		})
	}
}

func TestFilesystemUsageReadsExistingPath(t *testing.T) {
	value, err := filesystemUsage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if value.TotalBytes == 0 || value.Path == "" || value.State == "unknown" {
		t.Fatalf("unexpected filesystem usage: %#v", value)
	}
}
