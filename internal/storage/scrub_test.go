package storage

import "testing"

func TestValidateScrubSource(t *testing.T) {
	for _, test := range []struct {
		kind   SnapshotKind
		source string
		valid  bool
	}{
		{SnapshotBtrfs, "/srv/pools/media", true},
		{SnapshotBtrfs, "/etc", false},
		{SnapshotBtrfs, "/srv/pools/../etc", false},
		{SnapshotZfs, "tank/media", true},
		{SnapshotZfs, "tank/../etc", false},
		{SnapshotZfs, "", false},
	} {
		err := ValidateScrubSource(test.kind, test.source)
		if (err == nil) != test.valid {
			t.Errorf("ValidateScrubSource(%q, %q) err=%v", test.kind, test.source, err)
		}
	}
}

func TestParseBtrfsScrubStatus(t *testing.T) {
	status, err := ParseBtrfsScrubStatus("Status: running\nBytes scrubbed: 10GiB\n")
	if err != nil || !status.Running {
		t.Fatalf("running scrub was not parsed: %#v err=%v", status, err)
	}
	status, err = ParseBtrfsScrubStatus("Status: finished\nError summary: no errors found\n")
	if err != nil || status.Running || status.Progress != 100 || status.Message != "finished without errors" {
		t.Fatalf("finished scrub was not parsed: %#v err=%v", status, err)
	}
	status, err = ParseBtrfsScrubStatus("Status: finished\nError summary: csum=0, verify=2, read=0\n")
	if err != nil || !status.Errors || status.Progress != 100 {
		t.Fatalf("Btrfs integrity errors were not surfaced: %#v err=%v", status, err)
	}
	if _, err := ParseBtrfsScrubStatus("unknown"); err == nil {
		t.Fatal("missing status was accepted")
	}
}

func TestParseZpoolScrubStatus(t *testing.T) {
	status, err := ParseZpoolScrubStatus("pool: tank\nscan: scrub in progress since Mon Sep 28 00:00:00 2026\n\t54.25% done, 0 days 00:40:00 to go\n")
	if err != nil || !status.Running || status.Progress != 54.25 {
		t.Fatalf("running scrub was not parsed: %#v err=%v", status, err)
	}
	status, err = ParseZpoolScrubStatus("pool: tank\nscan: scrub repaired 0B in 00:42:10 with 0 errors on Mon Sep 28 2026\n")
	if err != nil || status.Running || status.Progress != 100 {
		t.Fatalf("completed scrub was not parsed: %#v err=%v", status, err)
	}
	status, err = ParseZpoolScrubStatus("pool: tank\nscan: scrub repaired 4K in 00:42:10 with 2 errors on Mon Sep 28 2026\n")
	if err != nil || !status.Errors {
		t.Fatalf("ZFS integrity errors were not surfaced: %#v err=%v", status, err)
	}
	if _, err := ParseZpoolScrubStatus("pool: tank\nstate: ONLINE\n"); err == nil {
		t.Fatal("missing scrub summary was accepted")
	}
}
