package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

const replacementConfig = `# managed
parity /srv/disks/serial_P/snapraid.parity
content /var/lib/lumonas/snapraid.content
content /srv/disks/serial_A/snapraid.content
content /srv/disks/serial_B/snapraid.content
data d1 /srv/disks/serial_A
data d2 /srv/disks/serial_B
data d3 /srv/disks/serial_DEAD
`

func replacementDisk(id, role string) model.Disk {
	return model.Disk{ID: id, CurrentPath: "/dev/sdx", Serial: strings.TrimPrefix(id, "serial:"), Role: role, Health: model.Healthy, LastSeen: time.Now().UTC()}
}

func TestReplacementPlanPreservesRetiredSlotName(t *testing.T) {
	disks := []model.Disk{replacementDisk("serial:A", "data"), replacementDisk("serial:B", "data"), replacementDisk("serial:NEW", "data")}
	plan, err := NewReplacementPlan("op-1", "serial:DEAD", "serial:NEW", replacementConfig, disks, 7, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if plan.RetiredDataName != "d3" {
		t.Fatalf("retired slot = %q, want d3", plan.RetiredDataName)
	}
	if plan.ParityDiskID != "serial_P" {
		t.Fatalf("parity = %q", plan.ParityDiskID)
	}
	// The replacement must occupy the retired d3 slot (branch-sanitized).
	mapping := map[string]string{}
	for _, slot := range plan.Slots {
		mapping[slot.Name] = slot.DiskID
	}
	if mapping["d3"] != "serial:NEW" || mapping["d1"] != "serial:A" || mapping["d2"] != "serial:B" {
		t.Fatalf("mapping changed for surviving disks: %#v", mapping)
	}

	// The pinned render round-trips through the parser unchanged.
	rendered, err := RenderSnapraidConfigPinned(plan.ParityDiskID, plan.Slots)
	if err != nil {
		t.Fatal(err)
	}
	parity2, slots2, err := ParseSnapraidDataMapping(rendered)
	if err != nil {
		t.Fatal(err)
	}
	if parity2 != plan.ParityDiskID || len(slots2) != 3 {
		t.Fatalf("round-trip mismatch: parity=%q slots=%#v", parity2, slots2)
	}
	if err := ValidateSnapraidConfig(rendered); err != nil {
		t.Fatalf("pinned render failed managed validation: %v", err)
	}
}

func TestReplacementPlanRejectsImpossibleReplacements(t *testing.T) {
	live := []model.Disk{replacementDisk("serial:A", "data"), replacementDisk("serial:B", "data"), replacementDisk("serial:P", "parity"), replacementDisk("serial:DEAD", "data")}
	cases := []struct {
		name      string
		retired   string
		repl      string
		wantError string
	}{
		{"retired not protected", "serial:ZZZ", "serial:B", "not part of the protected set"},
		{"replacement missing", "serial:A", "serial:GONE", "not currently discovered"},
		{"replacement is parity", "serial:A", "serial:P", "parity-role"},
	}
	for _, testCase := range cases {
		if _, err := NewReplacementPlan("op", testCase.retired, testCase.repl, replacementConfig, live, 1, time.Now()); err == nil || !contains(err.Error(), testCase.wantError) {
			t.Fatalf("%s: expected %q error, got %v", testCase.name, testCase.wantError, err)
		}
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func TestReplacementPlanRejectsTamperedHash(t *testing.T) {
	disks := []model.Disk{replacementDisk("serial:A", "data"), replacementDisk("serial:B", "data"), replacementDisk("serial:NEW", "data")}
	config := strings.Replace(replacementConfig, "data d3 /srv/disks/serial_DEAD\n", "", 1)
	plan, err := NewReplacementPlan("op-1", "serial:B", "serial:NEW", config, disks, 7, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateReplacementPlan(plan, disks, time.Now().Add(time.Minute), 7); err != nil {
		t.Fatalf("fresh plan should validate: %v", err)
	}
	changed := append([]model.Disk(nil), disks...)
	changed[2].Serial = "changed"
	if err := ValidateReplacementPlan(plan, changed, time.Now().Add(time.Minute), 7); err == nil || !contains(err.Error(), "identity changed") {
		t.Fatalf("replacement identity change accepted: %v", err)
	}
	if err := ValidateReplacementPlan(plan, disks, time.Now().Add(time.Minute), 8); err == nil || !contains(err.Error(), "generation") {
		t.Fatalf("stale generation accepted: %v", err)
	}
	plan.RetiredDataName = "d1"
	if err := ValidateReplacementPlan(plan, disks, time.Now().Add(time.Minute), 7); err == nil || !contains(err.Error(), "hash mismatch") {
		t.Fatalf("tampered plan accepted: %v", err)
	}
}
