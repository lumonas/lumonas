package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
)

var packNameRequestPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

func (s *apiServer) imagePackRoot() string {
	return envOr("LUMONAS_IMAGE_PACK_ROOT", "/usr/share/lumonas/image-packs")
}

func (s *apiServer) dockerImagePacks(w http.ResponseWriter, _ *http.Request) {
	packs, err := dockerruntime.AvailableImagePacks(s.imagePackRoot())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, packs)
}

func (s *apiServer) dockerImagePackImport(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !packNameRequestPattern.MatchString(input.Name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pack name"})
		return
	}
	root := filepath.Clean(s.imagePackRoot())
	directory := filepath.Join(root, input.Name)
	if filepath.Dir(directory) != root {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pack name"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	result, err := s.dockerService.ImportImagePack(ctx, directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "image pack not found"})
			return
		}
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "docker.image.pack.import", "images", map[string]any{"pack": result.Pack, "imported": len(result.Imported), "failed": len(result.Failed)})
	s.publish("docker.image.pack.imported", "info", nil, map[string]any{"pack": result.Pack, "imported": strings.Join(result.Imported, ","), "failed": len(result.Failed)})
	writeJSON(w, http.StatusOK, result)
}
