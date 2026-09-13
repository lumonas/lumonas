package installer

import (
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func installerDisks() []model.Disk {
	return []model.Disk{
		{ID: "wwn:system", Name: "sda", Model: "System SSD", SizeBytes: 256 << 30, Role: "system", Health: model.Healthy, Filesystem: "ext4", Mounted: true, WWN: "wwn:system", Serial: "S1"},
		{ID: "wwn:blank", Name: "sdb", Model: "Blank HDD", SizeBytes: 12 << 30, Role: "unknown", Health: model.Healthy, WWN: "wwn:blank", Serial: "S2"},
		{ID: "wwn:parity", Name: "sdc", Model: "Parity HDD", SizeBytes: 12 << 30, Role: "parity", Health: model.Healthy, WWN: "wwn:parity", Serial: "S3"},
		{ID: "wwn:mounted", Name: "sdd", Model: "Data HDD", SizeBytes: 8 << 30, Role: "data", Health: model.Healthy, Filesystem: "ext4", Mounted: true, WWN: "wwn:mounted", Serial: "S4"},
		{ID: "wwn:tiny", Name: "sde", Model: "USB Stick", SizeBytes: 4 << 30, Role: "unknown", Health: model.Healthy, WWN: "wwn:tiny", Serial: "S5"},
	}
}

func installerNow() time.Time { return time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC) }

func installerRequest() BuildRequest {
	return BuildRequest{TargetDiskID: "wwn:blank", Hostname: "lumonas", AdminUsername: "admin", Filesystem: "ext4", UEFI: true}
}

func TestEvaluateTargetsMarksProtectedDisks(t *testing.T) {
	targets := EvaluateTargets(installerDisks(), "wwn:system")
	byID := make(map[string]Target, len(targets))
	for _, target := range targets {
		byID[target.DiskID] = target
	}
	if byID["wwn:system"].Eligible {
		t.Fatal("running system disk must be protected")
	}
	if !strings.Contains(strings.Join(byID["wwn:system"].ProtectedBy, ","), "running system disk") {
		t.Fatalf("unexpected protection reasons: %#v", byID["wwn:system"].ProtectedBy)
	}
	if byID["wwn:parity"].Eligible {
		t.Fatal("parity disk must be protected")
	}
	if byID["wwn:mounted"].Eligible {
		t.Fatal("mounted disk with filesystem must be protected")
	}
	if byID["wwn:tiny"].Eligible {
		t.Fatal("undersized disk must be protected")
	}
	if !byID["wwn:blank"].Eligible {
		t.Fatalf("blank disk must be eligible: %#v", byID["wwn:blank"].ProtectedBy)
	}
	if byID["wwn:blank"].Identity["wwn"] != "wwn:blank" {
		t.Fatalf("identity not carried: %#v", byID["wwn:blank"].Identity)
	}
}

func TestBuildPlanValidatesRequest(t *testing.T) {
	disks := installerDisks()
	cases := map[string]func(*BuildRequest){
		"bad hostname":   func(r *BuildRequest) { r.Hostname = "Bad Hostname!" },
		"bad admin":      func(r *BuildRequest) { r.AdminUsername = "-admin" },
		"bad filesystem": func(r *BuildRequest) { r.Filesystem = "fat32" },
		"missing disk":   func(r *BuildRequest) { r.TargetDiskID = "wwn:absent" },
		"protected disk": func(r *BuildRequest) { r.TargetDiskID = "wwn:parity" },
	}
	for label, mutate := range cases {
		request := installerRequest()
		mutate(&request)
		if _, _, err := BuildPlan(disks, "wwn:system", request, installerNow()); err == nil {
			t.Fatalf("%s: expected rejection", label)
		}
	}
}

func TestBuildPlanAndApplyContract(t *testing.T) {
	disks := installerDisks()
	plan, targets, err := BuildPlan(disks, "wwn:system", installerRequest(), installerNow())
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != len(disks) {
		t.Fatalf("targets must be returned for the review step: %d", len(targets))
	}
	hash := plan.Hash()
	if hash == "" || hash != plan.Hash() {
		t.Fatal("plan hash must be stable")
	}
	if err := plan.ValidateForApply(hash, true, installerNow()); err != nil {
		t.Fatal(err)
	}
	if err := plan.ValidateForApply(hash, false, installerNow()); err == nil {
		t.Fatal("unconfirmed apply must be rejected")
	}
	if err := plan.ValidateForApply("deadbeef", true, installerNow()); err == nil {
		t.Fatal("hash mismatch must be rejected")
	}
	if err := plan.ValidateForApply(hash, true, installerNow().Add(PlanTTL+time.Minute)); err == nil {
		t.Fatal("expired plan must be rejected")
	}
	// Any plan mutation invalidates the pin.
	plan.Hostname = "renamed"
	if err := plan.ValidateForApply(hash, true, installerNow()); err == nil {
		t.Fatal("mutated plan must not validate against the original hash")
	}
}
