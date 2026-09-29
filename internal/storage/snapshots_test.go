package storage

import (
	"context"
	"strings"
	"testing"
	"time"
)

type snapshotFailure struct {
	output string
	err    error
}

func snapshotTestRunner(output string, commands *[]string, failFor map[string]snapshotFailure) SnapshotRunner {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		command := name + " " + strings.Join(args, " ")
		*commands = append(*commands, command)
		for fragment, failure := range failFor {
			if strings.Contains(command, fragment) {
				return []byte(failure.output), failure.err
			}
		}
		return []byte(output), nil
	}
}

func TestSnapshotNamingUsesSambaCompatibleUTCTimestamp(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.FixedZone("test", 2*3600))
	if got := SnapshotTimestamp(now); got != "2026.09.13-10.00.00" {
		t.Fatalf("timestamp not normalized to UTC: %s", got)
	}
	if got := SnapshotName("nightly", now); got != "2026.09.13-10.00.00" {
		t.Fatalf("human label must not change the SMB-compatible name: %s", got)
	}
	if got := SnapshotName("", now); got != "2026.09.13-10.00.00" {
		t.Fatalf("unexpected bare name: %s", got)
	}
}

func TestValidateSnapshotSource(t *testing.T) {
	if err := ValidateSnapshotSource(SnapshotBtrfs, "/srv/pools/main"); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"", "/", "relative/path", "/srv/pools/../pools"} {
		if err := ValidateSnapshotSource(SnapshotBtrfs, source); err == nil {
			t.Fatalf("expected rejection for %q", source)
		}
	}
	if err := ValidateSnapshotSource(SnapshotZfs, "tank/main/media"); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"", "-tank", "tank//media", "tank/./media", "tank/Media!"} {
		if err := ValidateSnapshotSource(SnapshotZfs, source); err == nil {
			t.Fatalf("expected rejection for %q", source)
		}
	}
	if err := ValidateSnapshotSource("ext4", "/srv"); err == nil {
		t.Fatal("expected unsupported kind to be rejected")
	}
}

func TestValidateSnapshotLabel(t *testing.T) {
	if err := ValidateSnapshotLabel(""); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshotLabel("nightly_1"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"-leading", "with space", "way-too-long-" + strings.Repeat("x", 80)} {
		if err := ValidateSnapshotLabel(label); err == nil {
			t.Fatalf("expected rejection for %q", label)
		}
	}
}

func TestDetectBtrfs(t *testing.T) {
	var commands []string
	runner := snapshotTestRunner("Label: none uuid: abc\n", &commands, map[string]snapshotFailure{
		"subvolume show /srv/data": {err: errTestCommand},
	})
	found, err := DetectBtrfs(context.Background(), runner, "/srv/pool")
	if err != nil || !found {
		t.Fatalf("unexpected detection found=%v err=%v", found, err)
	}

	missing := snapshotTestRunner("", &commands, map[string]snapshotFailure{
		"subvolume show": {output: `'/srv/data' is not a btrfs subvolume`, err: errNotSubvolume},
	})
	found, err = DetectBtrfs(context.Background(), missing, "/srv/data")
	if err != nil || found {
		t.Fatalf("non-subvolume must not be an error: found=%v err=%v", found, err)
	}

	broken := snapshotTestRunner("", &commands, map[string]snapshotFailure{
		"subvolume show": {err: errTestCommand},
	})
	if _, err := DetectBtrfs(context.Background(), broken, "/srv/other"); err == nil || isNotSubvolume(err) {
		t.Fatalf("unrelated failure must surface: %v", err)
	}
}

func TestBtrfsSnapshotLifecycle(t *testing.T) {
	var commands []string
	runner := snapshotTestRunner("", &commands, nil)
	name := SnapshotName("nightly", time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC))
	if err := CreateBtrfsSnapshot(context.Background(), runner, "/srv/pool", name); err != nil {
		t.Fatal(err)
	}
	listing := "ID 256 gen 10 cgen 10 parent 0 top level 5 otime 2026-09-12 22:00:00 path pool.snapshots/" + name + "\n" +
		"ID 257 gen 11 cgen 11 parent 0 top level 5 otime 2026-09-13 08:30:00 path pool.snapshots/older\n"
	listRunner := snapshotTestRunner(listing, &commands, nil)
	snapshots, err := ListBtrfsSnapshots(context.Background(), listRunner, "/srv/pool")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("unexpected snapshot count: %#v", snapshots)
	}
	if snapshots[0].Name != "older" {
		t.Fatalf("snapshots must be newest first: %#v", snapshots[0])
	}
	if snapshots[0].Kind != SnapshotBtrfs || !snapshots[0].Readonly || snapshots[0].Source != "/srv/pool" {
		t.Fatalf("unexpected snapshot metadata: %#v", snapshots[0])
	}
	if err := DeleteBtrfsSnapshot(context.Background(), runner, "/srv/pool", name); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(commands, "\n")
	if !strings.Contains(joined, "btrfs subvolume snapshot -r /srv/pool /srv/pool.snapshots/"+name) {
		t.Fatalf("unexpected create command: %s", joined)
	}
	if !strings.Contains(joined, "btrfs subvolume delete /srv/pool.snapshots/"+name) {
		t.Fatalf("unexpected delete command: %s", joined)
	}
	if err := DeleteBtrfsSnapshot(context.Background(), runner, "/srv/pool", "../escape"); err == nil {
		t.Fatal("expected invalid snapshot name to be rejected")
	}
}

func TestZfsSnapshotLifecycle(t *testing.T) {
	var commands []string
	runner := snapshotTestRunner("", &commands, nil)
	if err := CreateZfsSnapshot(context.Background(), runner, "tank/media", "2026.09.13-10.00.00"); err != nil {
		t.Fatal(err)
	}
	listing := "tank/media@2026.09.13-10.00.00  Sun Sep 13 10:00 2026 1.05M\ntank/media@older            Sat Sep 12 09:00 2026 20K\ntank/other@elsewhere        Sat Sep 12 09:00 2026 20K\n"
	listRunner := snapshotTestRunner(listing, &commands, nil)
	snapshots, err := ListZfsSnapshots(context.Background(), listRunner, "tank/media")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("unexpected snapshot count: %#v", snapshots)
	}
	if snapshots[0].Name != "2026.09.13-10.00.00" || snapshots[1].Name != "older" {
		t.Fatalf("snapshots must be newest first: %#v", snapshots)
	}
	if snapshots[1].UsedBytes != 20<<10 {
		t.Fatalf("unexpected size parse: %#v", snapshots[1])
	}
	if err := DeleteZfsSnapshot(context.Background(), runner, "tank/media", "older"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(commands, "\n")
	if !strings.Contains(joined, "zfs snapshot tank/media@2026.09.13-10.00.00") {
		t.Fatalf("unexpected create command: %s", joined)
	}
	if !strings.Contains(joined, "zfs destroy tank/media@older") {
		t.Fatalf("unexpected destroy command: %s", joined)
	}
	if err := CreateZfsSnapshot(context.Background(), runner, "tank/media", "bad name"); err == nil {
		t.Fatal("expected invalid snapshot name to be rejected")
	}
}

func TestDetectZfs(t *testing.T) {
	var commands []string
	found, err := DetectZfs(context.Background(), snapshotTestRunner("tank\n", &commands, nil), "tank")
	if err != nil || !found {
		t.Fatalf("unexpected detection found=%v err=%v", found, err)
	}
	missing := snapshotTestRunner("", &commands, map[string]snapshotFailure{
		"zfs list": {output: "cannot open 'nope': dataset does not exist", err: errDatasetMissing},
	})
	found, err = DetectZfs(context.Background(), missing, "nope")
	if err != nil || found {
		t.Fatalf("missing dataset must not error: found=%v err=%v", found, err)
	}
}

func TestParseZfsSize(t *testing.T) {
	cases := map[string]uint64{"0B": 0, "20K": 20 << 10, "1.05M": 1101004, "2G": 2 << 30, "-": 0, "": 0, "weird": 0}
	for input, want := range cases {
		if got := parseZfsSize(input); got != want {
			t.Fatalf("parseZfsSize(%q) = %d, want %d", input, got, want)
		}
	}
}

var errTestCommand = &testCommandError{}

type testCommandError struct{}

func (*testCommandError) Error() string { return "exit status 1: simulated failure" }

var errNotSubvolume = &notSubvolumeError{}

type notSubvolumeError struct{}

func (*notSubvolumeError) Error() string {
	return "ERROR: '/srv/data' is not a btrfs subvolume"
}

var errDatasetMissing = &datasetMissingError{}

type datasetMissingError struct{}

func (*datasetMissingError) Error() string {
	return "exit status 1: cannot open 'nope': dataset does not exist"
}
