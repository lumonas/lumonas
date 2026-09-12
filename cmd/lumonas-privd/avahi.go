package main

import (
	"os"
	"path/filepath"
	"strings"
)

// avahiServicePath is a var so tests can relocate the announcement file.
var avahiServicePath = "/etc/avahi/services/lumonas-smb.service"

// applyAvahiConfig publishes or removes the LumoNAS-managed Avahi service
// announcement. The service path is fixed inside the broker, the content is
// bounded and must look like an Avahi service group, and the daemon reload
// is best-effort because Avahi is an optional package.
func applyAvahiConfig(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	content, _ := req.RequestedState["content"].(string)
	if content == "" {
		if err := os.Remove(avahiServicePath); err != nil && !os.IsNotExist(err) {
			return response{Error: "Avahi announcement could not be removed"}
		}
		if err := reloadAvahi(run); err != nil {
			return response{OK: true, Data: map[string]string{"state": "removed", "reload": "failed"}}
		}
		return response{OK: true, Data: map[string]string{"state": "removed", "reload": "ok"}}
	}
	if len(content) > 64*1024 || strings.ContainsRune(content, '\x00') || !strings.Contains(content, "<service-group>") {
		return response{Error: "Avahi announcement content is invalid"}
	}
	if err := os.MkdirAll(filepath.Dir(avahiServicePath), 0o755); err != nil {
		return response{Error: "Avahi service directory could not be created"}
	}
	temporary, err := os.CreateTemp(filepath.Dir(avahiServicePath), ".lumonas-avahi-*")
	if err != nil {
		return response{Error: "Avahi announcement could not be written"}
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(content); err != nil {
		_ = temporary.Close()
		return response{Error: "Avahi announcement could not be written"}
	}
	if err := temporary.Close(); err != nil {
		return response{Error: "Avahi announcement could not be written"}
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return response{Error: "Avahi announcement could not be written"}
	}
	if err := os.Rename(temporaryPath, avahiServicePath); err != nil {
		return response{Error: "Avahi announcement could not be activated"}
	}
	reloadState := "ok"
	if err := reloadAvahi(run); err != nil {
		reloadState = "failed"
	}
	return response{OK: true, Data: map[string]string{"state": "published", "reload": reloadState}}
}

func reloadAvahi(run command) error {
	_, err := run("systemctl", "reload", "avahi-daemon")
	return err
}
