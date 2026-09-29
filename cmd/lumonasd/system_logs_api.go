package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/runner"
)

type systemLogEntry struct {
	Timestamp string `json:"timestamp"`
	Priority  string `json:"priority"`
	Unit      string `json:"unit"`
	Message   string `json:"message"`
}

var runSystemJournal = runner.OutputContext

var logUnits = map[string]bool{
	"lumonasd.service": true, "lumonas-web.service": true, "smbd.service": true,
	"nfs-server.service": true, "docker.service": true, "ssh.service": true,
	"smartmontools.service": true, "systemd-oomd.service": true,
}

func (s *apiServer) systemLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	unit := strings.TrimSpace(r.URL.Query().Get("unit"))
	if unit != "" && !logUnits[unit] {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "unsupported service filter"})
		return
	}
	args := []string{"--no-pager", "--output=json", "--lines=" + strconv.Itoa(limit)}
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	if source != "" && source != "smb-audit" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "unsupported log source"})
		return
	}
	if source == "smb-audit" && unit != "" && unit != "smbd.service" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "SMB audit source cannot be combined with another service"})
		return
	}
	if unit != "" {
		args = append(args, "--unit="+unit)
	}
	if source == "smb-audit" {
		args = append(args, "--identifier=smbd_audit")
	}
	sinceRaw := r.URL.Query().Get("since")
	if len(sinceRaw) > 40 || strings.ContainsAny(sinceRaw, "\x00\r\n") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid time filter"})
		return
	}
	if since := strings.TrimSpace(sinceRaw); since != "" {
		if len(since) > 40 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid time filter"})
			return
		}
		args = append(args, "--since="+since)
	}
	query := r.URL.Query().Get("q")
	if len(query) > 256 || strings.ContainsAny(query, "\x00\r\n") {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "log search must be at most 256 characters"})
		return
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	ctx, cancel := context.WithTimeout(r.Context(), 8_000_000_000)
	defer cancel()
	output, err := runSystemJournal(ctx, "journalctl", args...)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "system journal is unavailable"})
		return
	}
	entries := make([]systemLogEntry, 0, limit)
	for _, line := range strings.Split(string(output), "\n") {
		if line == "" {
			continue
		}
		var raw map[string]any
		if json.Unmarshal([]byte(line), &raw) != nil {
			continue
		}
		entry := systemLogEntry{Timestamp: journalString(raw["__REALTIME_TIMESTAMP"]), Priority: stringValue(raw["PRIORITY"]), Unit: stringValue(raw["_SYSTEMD_UNIT"]), Message: stringValue(raw["MESSAGE"])}
		if entry.Unit == "" {
			entry.Unit = stringValue(raw["SYSLOG_IDENTIFIER"])
		}
		if needle != "" && !strings.Contains(strings.ToLower(entry.Message+" "+entry.Unit), needle) {
			continue
		}
		entries = append(entries, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "unit": unit, "query": needle})
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
func journalString(value any) string {
	text := stringValue(value)
	if text == "" {
		return ""
	}
	if micros, err := strconv.ParseInt(text, 10, 64); err == nil {
		return time.UnixMicro(micros).UTC().Format(time.RFC3339Nano)
	}
	return text
}
