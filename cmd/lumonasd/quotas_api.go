package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/monitoring"
	"github.com/lumonas/lumonas/internal/quotas"
	managedshares "github.com/lumonas/lumonas/internal/shares"
)

func (s *apiServer) listQuotas(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	policies, err := s.store.Quotas()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	shares, err := s.store.ListManagedShares()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	principals, err := s.store.ListPrincipals("")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	usage, capturedAt, err := s.cachedQuotaUsage(shares, principals)
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "storage usage scan is incomplete: " + err.Error()})
		return
	}
	measured := capturedAt.UTC().Format(time.RFC3339)
	result := make([]quotas.Status, 0, len(policies))
	for _, policy := range policies {
		key := policy.TargetType + ":" + policy.TargetID
		used := usage[key]
		status := quotaStatus(policy, used, measured)
		result = append(result, status)
	}
	writeJSON(w, 200, result)
}

// quotaStatus derives the reported state for one policy. Both the API response
// and the alert evaluator must agree, so they share this single implementation;
// a non-positive limit is treated as unmeasured rather than dividing by zero.
func quotaStatus(policy quotas.Policy, used int64, measured string) quotas.Status {
	status := quotas.Status{Policy: policy, UsedBytes: used, MeasuredAt: measured, State: "healthy"}
	if policy.LimitBytes <= 0 {
		return status
	}
	status.Percent = float64(used) / float64(policy.LimitBytes) * 100
	switch {
	case used >= policy.LimitBytes:
		status.State = "over"
	case status.Percent >= float64(policy.WarningPercent):
		status.State = "warning"
	}
	return status
}

func (s *apiServer) evaluateStorageQuotaAlerts() {
	policies, err := s.store.Quotas()
	if err != nil || len(policies) == 0 {
		return
	}
	shares, err := s.store.ListManagedShares()
	if err != nil {
		return
	}
	principals, err := s.store.ListPrincipals("")
	if err != nil {
		return
	}
	usage, capturedAt, err := s.cachedQuotaUsage(shares, principals)
	if err != nil {
		return
	}
	measured := capturedAt.UTC().Format(time.RFC3339)
	statuses := make([]quotas.Status, 0, len(policies))
	for _, policy := range policies {
		used := usage[policy.TargetType+":"+policy.TargetID]
		statuses = append(statuses, quotaStatus(policy, used, measured))
	}
	s.evaluateQuotaAlerts(statuses)
}

func (s *apiServer) evaluateQuotaAlerts(statuses []quotas.Status) {
	defaults := []monitoring.AlertRule{
		{ID: "rule-quota-warning", Name: "Storage quota warning", Condition: "quota usage reached its configured warning threshold", Severity: "warning", Routes: []string{"web"}, Enabled: true},
		{ID: "rule-quota-over", Name: "Storage quota exceeded", Condition: "quota usage reached or exceeded its configured limit", Severity: "critical", Routes: []string{"web"}, Enabled: true},
	}
	for _, rule := range defaults {
		if _, err := s.store.AlertRule(rule.ID); err != nil {
			_ = s.store.SaveAlertRule(rule)
		}
	}
	active := make(map[string]bool, len(statuses))
	for _, status := range statuses {
		resourceID := status.Policy.ID
		active[resourceID] = true
		resource := &model.ResourceRef{Type: "quota", ID: resourceID}
		label := status.Policy.TargetType + " " + status.Policy.TargetID
		description := fmt.Sprintf("%s uses %s of its %s quota (%.1f%%)", label, formatQuotaBytes(status.UsedBytes), formatQuotaBytes(status.Policy.LimitBytes), status.Percent)
		switch status.State {
		case "over":
			s.fireAlertForRule("rule-quota-over", "Storage quota exceeded", description, resource)
			s.resolveAlertForRule("rule-quota-warning", resourceID)
		case "warning":
			s.fireAlertForRule("rule-quota-warning", "Storage quota approaching", description, resource)
			s.resolveAlertForRule("rule-quota-over", resourceID)
		default:
			s.resolveAlertForRule("rule-quota-warning", resourceID)
			s.resolveAlertForRule("rule-quota-over", resourceID)
		}
	}
	// Remove policies must clear their old firing alerts as well.
	for _, ruleID := range []string{"rule-quota-warning", "rule-quota-over"} {
		alerts, err := s.store.GeneratedAlerts()
		if err != nil {
			continue
		}
		for _, alert := range alerts {
			if alert.Resource != nil && alert.Resource.Type == "quota" && !active[alert.Resource.ID] {
				s.resolveAlertForRule(ruleID, alert.Resource.ID)
			}
		}
	}
}

func formatQuotaBytes(value int64) string {
	const unit = int64(1024)
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	divisor, exponent := unit, 0
	for next := divisor * unit; value >= next && exponent < 4; next = divisor * unit {
		divisor = next
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(divisor), "KMGT"[exponent])
}

func (s *apiServer) cachedQuotaUsage(shares []managedshares.ManagedShare, principals []identity.Principal) (map[string]int64, time.Time, error) {
	now := time.Now()
	s.quotaMu.Lock()
	if s.quotaUsageCache != nil && now.Sub(s.quotaMeasuredAt) < 5*time.Minute {
		copy := map[string]int64{}
		for key, value := range s.quotaUsageCache {
			copy[key] = value
		}
		capturedAt := s.quotaMeasuredAt
		s.quotaMu.Unlock()
		return copy, capturedAt, nil
	}
	s.quotaMu.Unlock()
	usage, err := quotaUsage(shares, principals)
	if err != nil {
		return nil, time.Time{}, err
	}
	s.quotaMu.Lock()
	s.quotaUsageCache, s.quotaMeasuredAt = usage, now
	s.quotaMu.Unlock()
	return usage, now, nil
}

func (s *apiServer) replaceQuotas(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var values []quotas.Policy
	if json.NewDecoder(r.Body).Decode(&values) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	shares, err := s.store.ListManagedShares()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	shareIDs := map[string]bool{}
	for _, share := range shares {
		shareIDs[share.ID] = true
	}
	for i := range values {
		if values[i].ID == "" {
			values[i].ID = newID("quota")
		}
		if err := values[i].Validate(); err != nil {
			writeJSON(w, 422, map[string]string{"error": err.Error()})
			return
		}
		switch values[i].TargetType {
		case "share":
			if !shareIDs[values[i].TargetID] {
				writeJSON(w, 422, map[string]string{"error": "quota share was not found"})
				return
			}
		case "user", "group":
			principal, err := s.store.Principal(values[i].TargetID)
			want := identity.Kind(values[i].TargetType)
			if err != nil || principal.Kind != want {
				writeJSON(w, 422, map[string]string{"error": "quota user or group was not found"})
				return
			}
		}
	}
	previous, _ := s.store.Quotas()
	if err := s.store.ReplaceQuotas(values); err != nil {
		writeJSON(w, 422, map[string]string{"error": err.Error()})
		return
	}
	active := make(map[string]bool, len(values))
	for _, value := range values {
		active[value.ID] = true
	}
	for _, value := range previous {
		if active[value.ID] {
			continue
		}
		s.resolveAlertForRule("rule-quota-warning", value.ID)
		s.resolveAlertForRule("rule-quota-over", value.ID)
	}
	s.recordRequestAudit(r, actor, "storage.quotas.update", "storage-quotas", map[string]any{"count": len(values)})
	s.publishActor(actor, "storage.quotas.updated", "info", &model.ResourceRef{Type: "storage-quotas", ID: "storage-quotas"}, map[string]any{"count": len(values)})
	writeJSON(w, 200, values)
}

func quotaUsage(shares []managedshares.ManagedShare, principals []identity.Principal) (map[string]int64, error) {
	result := map[string]int64{}
	uidTargets := map[uint64][]string{}
	gidTargets := map[uint64][]string{}
	for _, principal := range principals {
		if principal.Kind == identity.KindUser && principal.UID != nil {
			uidTargets[uint64(*principal.UID)] = append(uidTargets[uint64(*principal.UID)], "user:"+principal.ID)
		}
		if principal.Kind == identity.KindGroup && principal.GID != nil {
			gidTargets[uint64(*principal.GID)] = append(gidTargets[uint64(*principal.GID)], "group:"+principal.ID)
		}
	}
	seen := map[string]bool{}
	seenForPrincipal := map[string]bool{}
	files := 0
	for _, share := range shares {
		root, err := filepath.EvalSymlinks(share.Path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if seen[root] {
			continue
		}
		seen[root] = true
		err = filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			files++
			if files > 1_000_000 {
				return errors.New("scan limit of one million files exceeded")
			}
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok {
				return nil
			}
			size := info.Size()
			result["share:"+share.ID] += size
			canonical, err := filepath.Abs(current)
			if err != nil {
				return err
			}
			if seenForPrincipal[canonical] {
				return nil
			}
			seenForPrincipal[canonical] = true
			for _, target := range uidTargets[uint64(stat.Uid)] {
				result[target] += size
			}
			for _, target := range gidTargets[uint64(stat.Gid)] {
				result[target] += size
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
