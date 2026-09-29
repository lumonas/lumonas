package main

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
)

func dockerRecoveryProfileKey(stack string) string {
	return "docker_recovery_profile:" + stack
}

func (s *apiServer) updateDockerStackRecovery(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		AppdataPaths []string `json:"appdataPaths"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if len(input.AppdataPaths) > 32 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at most 32 appdata paths are allowed"})
		return
	}
	stacks, err := s.decoratedDockerStacks(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Docker stack inventory is unavailable"})
		return
	}
	var selected *dockerruntime.Stack
	for index := range stacks {
		if stacks[index].ID == id || stacks[index].Name == id {
			selected = &stacks[index]
			break
		}
	}
	if selected == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Docker stack not found"})
		return
	}
	paths := make([]string, 0, len(input.AppdataPaths))
	seen := make(map[string]bool, len(input.AppdataPaths))
	for _, value := range input.AppdataPaths {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 512 || !strings.HasPrefix(value, "/") || path.Clean(value) != value || value == "/" || strings.Contains(value, "..") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "appdata paths must be clean absolute container paths below /"})
			return
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		paths = append(paths, value)
	}
	profile := struct {
		AppdataPaths []string `json:"appdataPaths"`
	}{AppdataPaths: paths}
	encoded, err := json.Marshal(profile)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "recovery profile could not be encoded"})
		return
	}
	if err := s.store.SetMeta(dockerRecoveryProfileKey(selected.Name), string(encoded)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "recovery profile could not be saved"})
		return
	}
	s.advanceGeneration("docker.stack.recovery_profile")
	s.recordRequestAudit(r, actor, "docker.stack.recovery_profile.update", selected.Name, map[string]any{"appdataPaths": paths})
	selected.Recovery.AppdataPaths = appendUniquePaths(selected.Recovery.AppdataPaths, paths)
	selected.RecoveryCoverage = dockerruntime.StackRecoveryCoverage(selected.Recovery)
	writeJSON(w, http.StatusOK, selected)
}

func appendUniquePaths(existing, added []string) []string {
	result := append([]string(nil), existing...)
	seen := make(map[string]bool, len(result)+len(added))
	for _, value := range result {
		seen[value] = true
	}
	for _, value := range added {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
