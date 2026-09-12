package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type sshKey struct {
	ID        string `json:"id"`
	PublicKey string `json:"publicKey"`
	Comment   string `json:"comment,omitempty"`
}

func authorizedKeysPath() string {
	if v := os.Getenv("MYNAS_SSH_AUTHORIZED_KEYS"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		return filepath.Join(home, ".ssh", "authorized_keys")
	}
	return "/root/.ssh/authorized_keys"
}

func (s *apiServer) listSSHKeys(w http.ResponseWriter) {
	keys, err := readSSHKeys()
	if err != nil {
		writeJSON(w, http.StatusOK, []sshKey{})
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (s *apiServer) addSSHKey(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PublicKey string `json:"publicKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	input.PublicKey = strings.TrimSpace(input.PublicKey)
	if input.PublicKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "public key is required"})
		return
	}
	parts := strings.Fields(input.PublicKey)
	if len(parts) < 2 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid public key format"})
		return
	}
	path := authorizedKeysPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer f.Close()
	line := input.PublicKey
	if len(parts) >= 3 {
		line += " " + parts[2]
	}
	if _, err := fmt.Fprintln(f, line); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.publish("settings.ssh.key_added", "info", nil, map[string]any{"comment": commentFromKey(input.PublicKey)})
	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

func (s *apiServer) removeSSHKey(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PublicKey string `json:"publicKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	input.PublicKey = strings.TrimSpace(input.PublicKey)
	if input.PublicKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "public key is required"})
		return
	}
	path := authorizedKeysPath()
	keys, err := readSSHKeys()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var filtered []string
	targetKey := strings.Fields(input.PublicKey)
	for _, k := range keys {
		existing := strings.Fields(k.PublicKey)
		if len(existing) >= 2 && len(targetKey) >= 2 && existing[0] == targetKey[0] && existing[1] == targetKey[1] {
			continue
		}
		entry := k.PublicKey
		if k.Comment != "" {
			entry += " " + k.Comment
		}
		filtered = append(filtered, entry)
	}
	content := strings.Join(filtered, "\n")
	if len(filtered) > 0 {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.publish("settings.ssh.key_removed", "info", nil, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func readSSHKeys() ([]sshKey, error) {
	path := authorizedKeysPath()
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var keys []sshKey
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		comment := ""
		if len(parts) >= 3 {
			comment = parts[2]
		}
		keys = append(keys, sshKey{
			ID:        parts[1][:sshKeyIDMin(12, len(parts[1]))],
			PublicKey: parts[0] + " " + parts[1],
			Comment:   comment,
		})
	}
	return keys, nil
}

func commentFromKey(key string) string {
	parts := strings.Fields(key)
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

func sshKeyIDMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
