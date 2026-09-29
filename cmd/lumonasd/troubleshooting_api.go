package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/storage"
)

func (s *apiServer) troubleshooting(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	issues := make([]model.TroubleshootingIssue, 0)
	add := func(issue model.TroubleshootingIssue) {
		issue.Steps = append([]string(nil), issue.Steps...)
		issues = append(issues, issue)
	}
	disks, err := s.diskFunc()
	if err != nil {
		add(model.TroubleshootingIssue{ID: "disk-discovery", Severity: model.Attention, Title: "Disk discovery is unavailable", Summary: "LumoNAS cannot currently inspect the storage hardware.", Cause: "The read-only lsblk/udev collector returned an error.", Steps: []string{"Check that the storage tools are installed.", "Review the lumonasd service log for collector errors."}, Links: []model.TroubleshootingLink{{Label: "Open Storage", Path: "/storage"}}})
	} else {
		for _, disk := range disks {
			if disk.Health != model.Warning && disk.Health != model.Critical {
				continue
			}
			severity := disk.Health
			recommendation := "Review SMART attributes and run a short self-test before replacing the disk."
			if disk.Health == model.Critical {
				recommendation = "Stop destructive storage operations, confirm the disk identity, and plan a replacement."
			}
			add(model.TroubleshootingIssue{ID: "disk-" + disk.ID, Severity: severity, Title: fmt.Sprintf("%s needs attention", disk.Name), Summary: fmt.Sprintf("SMART reports %s health for %s.", disk.Health, disk.Model), Cause: smartCause(disk), Steps: []string{recommendation, "Confirm that recent backups are verified.", "Do not force a SnapRAID sync while a protected disk is missing."}, Resource: &model.ResourceRef{Type: "disk", ID: disk.ID}, Links: []model.TroubleshootingLink{{Label: "Inspect disk", Path: "/storage?tab=disks&disk=" + disk.ID}}})
		}
		known, _ := s.store.KnownDisks()
		current := make(map[string]bool, len(disks))
		for _, disk := range disks {
			current[disk.ID] = true
		}
		for _, disk := range known {
			if current[disk.ID] {
				continue
			}
			add(model.TroubleshootingIssue{ID: "missing-" + disk.ID, Severity: model.Critical, Title: "A known disk is missing", Summary: disk.ID + " is not present in the current inventory.", Cause: "The disk may have lost power, a cable may be loose, or the device path may have changed.", Steps: []string{"Check power and data connections.", "Rescan disks and verify the stable identity.", "Do not initialize or erase the replacement until the protected state is understood."}, Resource: &model.ResourceRef{Type: "disk", ID: disk.ID}, Links: []model.TroubleshootingLink{{Label: "Open disk inventory", Path: "/storage?tab=disks"}}})
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	protection := storage.DiscoverProtection(ctx, disks, nil, envOr("LUMONAS_SNAPRAID_CONFIG", "/etc/lumonas/snapraid.conf"))
	s.enrichProtection(&protection)
	if protection.Status != model.Healthy {
		steps := []string{"Review the missing or stale disk identities.", "Run a sync only after all protected disks are present and verified."}
		if protection.ChangesSinceSync == 0 {
			steps = []string{"Review the SnapRAID configuration and parity assignment.", "Run a protected sync after validation completes."}
		}
		add(model.TroubleshootingIssue{ID: "protection", Severity: protection.Status, Title: "Data protection is not current", Summary: "SnapRAID protection needs operator attention.", Cause: fmt.Sprintf("Protection status is %s; %d bytes are not represented in the last sync.", protection.Status, protection.ChangesSinceSync), Steps: steps, Links: []model.TroubleshootingLink{{Label: "Open Protection", Path: "/storage?tab=protection"}}})
	}
	if !s.dockerService.Available(ctx) {
		add(model.TroubleshootingIssue{ID: "docker", Severity: model.Attention, Title: "Docker Engine is unavailable", Summary: "Apps cannot start or be inspected until Docker responds.", Cause: "The Docker Engine socket or service did not respond to a read-only version check.", Steps: []string{"Check docker.service status.", "Confirm the Docker socket is available to lumonasd.", "Retry after the service becomes healthy."}, Links: []model.TroubleshootingLink{{Label: "Open Docker", Path: "/docker"}}})
	}
	if key := s.recoveryKeyString(); key == "" {
		add(model.TroubleshootingIssue{ID: "recovery-key", Severity: model.Warning, Title: "Recovery key is not configured", Summary: "Encrypted recovery bundles cannot be created or tested.", Cause: "No recovery key environment value or protected key file was found.", Steps: []string{"Create a recovery key from the Recovery page.", "Store a second copy independently of the NAS."}, Links: []model.TroubleshootingLink{{Label: "Open Recovery", Path: "/backups?tab=recovery"}}})
	}
	status := "clear"
	if len(issues) > 0 {
		status = "action_required"
	}
	writeJSON(w, http.StatusOK, model.TroubleshootingReport{Status: status, Issues: issues})
}

func smartCause(disk model.Disk) string {
	var causes []string
	if disk.SMART.ReallocatedSectors > 0 {
		causes = append(causes, fmt.Sprintf("%d reallocated sectors", disk.SMART.ReallocatedSectors))
	}
	if disk.SMART.PendingSectors > 0 {
		causes = append(causes, fmt.Sprintf("%d pending sectors", disk.SMART.PendingSectors))
	}
	if disk.SMART.UncorrectableSectors > 0 {
		causes = append(causes, fmt.Sprintf("%d uncorrectable sectors", disk.SMART.UncorrectableSectors))
	}
	if disk.SMART.CRCErrors > 0 {
		causes = append(causes, fmt.Sprintf("%d interface CRC errors", disk.SMART.CRCErrors))
	}
	if len(causes) == 0 {
		return "The disk collector reported a non-healthy state without detailed counters."
	}
	return strings.Join(causes, ", ") + "."
}
