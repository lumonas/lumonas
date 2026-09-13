package network

import (
	"context"
	"strings"
	"testing"
)

func neighborRunner(output string, commands *[]string) LANRunner {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		*commands = append(*commands, name+" "+strings.Join(args, " "))
		return []byte(output), nil
	}
}

const neighborTable = `192.168.1.34 dev eth0 lladdr AA:BB:CC:DD:EE:FF REACHABLE
192.168.1.1 dev eth0 lladdr 00:11:22:33:44:55 REACHABLE
192.168.1.99 dev eth0  FAILED
fe80::1 dev eth0 lladdr aa:bb:cc:00:00:01 router STALE
192.168.1.50 dev eth0 lladdr 11:22:33:44:55:66 INCOMPLETE
172.16.0.9 dev br-lan lladdr aa:bb:cc:dd:ee:01 PERMANENT
this-is-not-an-ip dev eth0 lladdr aa:bb:cc:dd:ee:02 REACHABLE
192.168.1.77 dev eth0 lladdr zz:bb:cc:dd:ee:03 STALE
`

func TestDiscoverLANHostsParsesNeighborTable(t *testing.T) {
	var commands []string
	hosts, err := DiscoverLANHosts(context.Background(), neighborRunner(neighborTable, &commands))
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0] != "ip neigh show" {
		t.Fatalf("unexpected command: %v", commands)
	}
	if len(hosts) != 4 {
		t.Fatalf("unexpected host count: %#v", hosts)
	}
	byMAC := make(map[string]LanHost, len(hosts))
	for _, host := range hosts {
		byMAC[host.MAC] = host
	}
	failed, ok := byMAC["aa:bb:cc:dd:ee:ff"]
	if !ok || failed.IP != "192.168.1.34" || failed.Interface != "eth0" || failed.State != "reachable" {
		t.Fatalf("missing or malformed host: %#v", failed)
	}
	if _, ok := byMAC["aa:bb:cc:dd:ee:02"]; ok {
		t.Fatal("invalid IP line must be skipped")
	}
	if _, ok := byMAC["zz:bb:cc:dd:ee:03"]; ok {
		t.Fatal("invalid MAC must be skipped")
	}
	if byMAC["00:11:22:33:44:55"].IP != "192.168.1.1" {
		t.Fatalf("unexpected first entry: %#v", hosts)
	}
	if byMAC["aa:bb:cc:dd:ee:01"].Interface != "br-lan" || byMAC["aa:bb:cc:dd:ee:01"].State != "permanent" {
		t.Fatalf("unexpected permanent entry: %#v", byMAC["aa:bb:cc:dd:ee:01"])
	}
}

func TestDiscoverLANHostsSurfacesRunnerErrors(t *testing.T) {
	_, err := DiscoverLANHosts(context.Background(), func(context.Context, string, ...string) ([]byte, error) {
		return nil, context.DeadlineExceeded
	})
	if err == nil {
		t.Fatal("runner failure must surface")
	}
}

func TestParseNeighborsIgnoresDuplicates(t *testing.T) {
	output := "192.168.1.34 dev eth0 lladdr aa:bb:cc:dd:ee:ff REACHABLE\n192.168.1.35 dev eth0 lladdr aa:bb:cc:dd:ee:ff STALE\n"
	hosts := ParseNeighbors(output)
	if len(hosts) != 1 {
		t.Fatalf("duplicate mac/interface pairs must be deduplicated: %#v", hosts)
	}
}

func TestValidateWakeTarget(t *testing.T) {
	if err := ValidateWakeTarget("eth0", "AA:BB:CC:DD:EE:FF"); err != nil {
		t.Fatal(err)
	}
	for _, iface := range []string{"", "../escape", "way-too-long-interface-name"} {
		if err := ValidateWakeTarget(iface, "aa:bb:cc:dd:ee:ff"); err == nil {
			t.Fatalf("expected interface %q to be rejected", iface)
		}
	}
	for _, mac := range []string{"", "not-a-mac", "aa:bb:cc:dd:ee", "aa:bb:cc:dd:ee:gg"} {
		if err := ValidateWakeTarget("eth0", mac); err == nil {
			t.Fatalf("expected MAC %q to be rejected", mac)
		}
	}
}

func TestWakeHostSendsEtherwakePacket(t *testing.T) {
	var commands []string
	wakeRunner := func(_ context.Context, name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	if err := WakeHost(context.Background(), "eth0", "AA:BB:CC:DD:EE:FF", wakeRunner); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0] != "etherwake -i eth0 aa:bb:cc:dd:ee:ff" {
		t.Fatalf("unexpected wake command: %v", commands)
	}
	if err := WakeHost(context.Background(), "../escape", "aa:bb:cc:dd:ee:ff", wakeRunner); err == nil {
		t.Fatal("invalid interface must be rejected before any command")
	}
	if err := WakeHost(context.Background(), "eth0", "not-a-mac", wakeRunner); err == nil {
		t.Fatal("invalid MAC must be rejected before any command")
	}
}
