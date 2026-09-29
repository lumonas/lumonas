package main

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/shares"
)

const smbChangeBurstThreshold = 80

type smbChangeBurst struct {
	shareID string
	actor   string
	count   int
}

// parseSMBChangeBursts recognizes Samba full_audit records for destructive
// operations. The share name is matched against configured shares instead of
// trusting arbitrary path fields from the journal.
func parseSMBChangeBursts(output []byte, configured []shares.ManagedShare) map[string]smbChangeBurst {
	sharesByName := make(map[string]shares.ManagedShare)
	for _, share := range configured {
		if share.Enabled && smbAuditEnabled(share) {
			sharesByName[share.Name] = share
		}
	}
	bursts := make(map[string]smbChangeBurst)
	for _, line := range strings.Split(string(output), "\n") {
		var record struct {
			Message string `json:"MESSAGE"`
		}
		if line == "" || json.Unmarshal([]byte(line), &record) != nil {
			continue
		}
		fields := strings.Split(record.Message, "|")
		if len(fields) < 5 {
			continue
		}
		actor := strings.TrimSpace(fields[0])
		if actor == "" || actor == "-" {
			continue
		}
		operation := ""
		operationIndex := -1
		for index, field := range fields[1:] {
			switch strings.TrimSpace(field) {
			case "unlinkat", "rmdir", "renameat":
				operation = strings.TrimSpace(field)
				operationIndex = index + 1
			}
			if operation != "" {
				break
			}
		}
		if operation == "" {
			continue
		}
		if operationIndex+1 >= len(fields) || (strings.TrimSpace(fields[operationIndex+1]) != "ok" && strings.TrimSpace(fields[operationIndex+1]) != "success") {
			continue
		}
		var share shares.ManagedShare
		for _, field := range fields[1:] {
			if candidate, ok := sharesByName[strings.TrimSpace(field)]; ok {
				share = candidate
				break
			}
		}
		if share.ID == "" {
			continue
		}
		key := share.ID + "\x00" + actor
		burst := bursts[key]
		burst.shareID, burst.actor, burst.count = share.ID, actor, burst.count+1
		bursts[key] = burst
	}
	return bursts
}

func smbAuditEnabled(share shares.ManagedShare) bool {
	for _, protocol := range share.Protocols {
		if protocol.Name == "smb" {
			if enabled, _ := protocol.Settings["auditEnabled"].(bool); enabled {
				return true
			}
		}
	}
	return false
}

// evaluateSMBChangeBursts raises a review-first ransomware warning from the
// last five minutes of configured SMB audit events. It deliberately does not
// disable the share: an operator can inspect logs and restore selected files
// without unexpectedly cutting off legitimate users.
func (s *apiServer) evaluateSMBChangeBursts() {
	configured, err := s.store.ListManagedShares()
	if err != nil {
		return
	}
	hasAuditedShare := false
	for _, share := range configured {
		if share.Enabled && smbAuditEnabled(share) {
			hasAuditedShare = true
			break
		}
	}
	if !hasAuditedShare {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	output, err := runSystemJournal(ctx, "journalctl", "--no-pager", "--output=json", "--lines=5000", "--since=5 minutes ago", "--identifier=smbd_audit")
	if err != nil {
		return
	}
	bursts := parseSMBChangeBursts(output, configured)
	active := make(map[string]bool)
	for _, burst := range bursts {
		if burst.count < smbChangeBurstThreshold {
			continue
		}
		active[burst.shareID] = true
		share, err := s.store.ManagedShare(burst.shareID)
		if err != nil {
			continue
		}
		description := share.Name + " recorded " + fmtInt(burst.count) + " delete, directory-remove, or rename operations by " + burst.actor + " in 5 minutes. Review SMB activity, stop the client if unexpected, then restore affected files from snapshot history."
		s.fireAlertWithSeverity("rule-ransomware", "critical", "Unusual file-change burst", description, &model.ResourceRef{Type: "share", ID: share.ID})
	}
	for _, share := range configured {
		if share.Enabled && smbAuditEnabled(share) && !active[share.ID] {
			s.resolveAlertForRule("rule-ransomware", share.ID)
		}
	}
}

func fmtInt(value int) string {
	return strconv.Itoa(value)
}
