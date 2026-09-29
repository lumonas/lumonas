package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	fileops "github.com/lumonas/lumonas/internal/files"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func (s *apiServer) listFileShareLinks(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	share, err := s.fileShare(r.URL.Query().Get("shareId"))
	if err != nil {
		writeFileError(w, err)
		return
	}
	values, err := s.store.FileShareLinks(share.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	for i := range values {
		values[i].TokenHash = ""
		values[i].PasswordHash = ""
	}
	writeJSON(w, 200, values)
}

func (s *apiServer) createFileShareLink(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ShareID        string `json:"shareId"`
		Path           string `json:"path"`
		ExpiresInHours int    `json:"expiresInHours"`
		Password       string `json:"password"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.ExpiresInHours < 1 || input.ExpiresInHours > 720 || len(input.Password) > 128 || (input.Password != "" && len(input.Password) < 12) {
		writeJSON(w, 422, map[string]string{"error": "expiration must be 1–720 hours; optional passwords must be 12–128 characters"})
		return
	}
	share, err := s.fileShare(input.ShareID)
	if err != nil {
		writeFileError(w, err)
		return
	}
	directory, err := fileops.Resolve(share.Path, input.Path)
	if err != nil {
		writeFileError(w, err)
		return
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		writeJSON(w, 422, map[string]string{"error": "share links must target an existing directory"})
		return
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not create secure link"})
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	passwordHash := ""
	if input.Password != "" {
		encoded, hashErr := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			writeJSON(w, 500, map[string]string{"error": "could not protect the link password"})
			return
		}
		passwordHash = string(encoded)
	}
	value := store.FileShareLink{ID: newID("file-share"), ShareID: share.ID, Path: pathOrRoot(input.Path), TokenHash: hashFileRequestToken(token), PasswordHash: passwordHash, ExpiresAt: time.Now().UTC().Add(time.Duration(input.ExpiresInHours) * time.Hour)}
	if err := s.store.CreateFileShareLink(value); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	value.TokenHash, value.PasswordHash = "", ""
	link := "/share/" + token
	s.recordRequestAudit(r, actor, "file.share-link.create", value.ID, map[string]any{"shareId": share.ID, "expiresAt": value.ExpiresAt, "passwordProtected": input.Password != ""})
	s.publishActor(actor, "file.share-link.created", "info", &model.ResourceRef{Type: "file-share-link", ID: value.ID}, nil)
	writeJSON(w, 201, map[string]any{"link": value, "url": link})
}

func (s *apiServer) revokeFileShareLink(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	if err := s.store.RevokeFileShareLink(id, time.Now()); err != nil {
		writeJSON(w, 404, map[string]string{"error": "share link not found or already revoked"})
		return
	}
	s.recordRequestAudit(r, actor, "file.share-link.revoke", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *apiServer) lookupFileShareLink(w http.ResponseWriter, r *http.Request, token string) (store.FileShareLink, bool) {
	if len(token) < 40 || len(token) > 64 {
		writeJSON(w, 404, map[string]string{"error": "share link not found"})
		return store.FileShareLink{}, false
	}
	value, err := s.store.FileShareLinkByTokenHash(hashFileRequestToken(token))
	if err != nil || value.RevokedAt != nil || !value.ExpiresAt.After(time.Now()) {
		writeJSON(w, 404, map[string]string{"error": "share link not found or expired"})
		return store.FileShareLink{}, false
	}
	if value.PasswordHash != "" {
		ip := clientIP(r)
		if !s.allowSharePasswordAttempt(value.TokenHash, ip, false) {
			w.Header().Set("Retry-After", "900")
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "too many password attempts; try again later", "requiresPassword": true})
			return store.FileShareLink{}, false
		}
		if bcrypt.CompareHashAndPassword([]byte(value.PasswordHash), []byte(r.Header.Get("X-Share-Password"))) != nil {
			_ = s.allowSharePasswordAttempt(value.TokenHash, ip, true)
			writeJSON(w, 401, map[string]any{"error": "a valid share-link password is required", "requiresPassword": true})
			return store.FileShareLink{}, false
		}
	}
	return value, true
}

func (s *apiServer) allowSharePasswordAttempt(tokenHash, ip string, recordFailure bool) bool {
	const window = 15 * time.Minute
	s.sharePasswordMu.Lock()
	defer s.sharePasswordMu.Unlock()
	if s.sharePasswordAttempts == nil {
		s.sharePasswordAttempts = make(map[string][]time.Time)
	}
	now := time.Now()
	keys := []struct {
		key   string
		limit int
	}{{"token:" + tokenHash, 100}, {"ip:" + tokenHash + ":" + ip, 10}}
	pruned := make(map[string][]time.Time, len(keys))
	allowed := true
	for _, item := range keys {
		for _, attempt := range s.sharePasswordAttempts[item.key] {
			if now.Sub(attempt) < window {
				pruned[item.key] = append(pruned[item.key], attempt)
			}
		}
		if len(pruned[item.key]) >= item.limit {
			allowed = false
		}
	}
	for _, item := range keys {
		if recordFailure && allowed {
			pruned[item.key] = append(pruned[item.key], now)
		}
		s.sharePasswordAttempts[item.key] = pruned[item.key]
	}
	if len(s.sharePasswordAttempts) > 10_000 {
		for key, attempts := range s.sharePasswordAttempts {
			if len(attempts) == 0 || now.Sub(attempts[len(attempts)-1]) >= window {
				delete(s.sharePasswordAttempts, key)
			}
		}
	}
	return allowed
}

func scopedSharePath(linkPath, requested string) (string, bool) {
	requested = strings.ReplaceAll(requested, `\`, `/`)
	if strings.HasPrefix(requested, "/") {
		return "", false
	}
	for _, segment := range strings.Split(requested, "/") {
		if segment == ".." {
			return "", false
		}
	}
	base := strings.Trim(path.Clean("/"+linkPath), "/")
	rel := strings.Trim(path.Clean("/"+requested), "/")
	combined := path.Clean(path.Join(base, rel))
	if combined == "." {
		combined = ""
	}
	if base != "" && combined != base && !strings.HasPrefix(combined, base+"/") {
		return "", false
	}
	return combined, true
}

func (s *apiServer) publicFileShareLink(w http.ResponseWriter, r *http.Request, token string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	value, ok := s.lookupFileShareLink(w, r, token)
	if !ok {
		return
	}
	share, err := s.fileShare(value.ShareID)
	if err != nil {
		writeJSON(w, 410, map[string]string{"error": "the shared folder is no longer available"})
		return
	}
	writeJSON(w, 200, map[string]any{"shareName": share.Name, "path": value.Path, "expiresAt": value.ExpiresAt, "passwordProtected": value.PasswordHash != ""})
}

func (s *apiServer) publicFileShareEntries(w http.ResponseWriter, r *http.Request, token string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	value, ok := s.lookupFileShareLink(w, r, token)
	if !ok {
		return
	}
	rel, valid := scopedSharePath(value.Path, r.URL.Query().Get("path"))
	if !valid {
		writeJSON(w, 403, map[string]string{"error": "requested path is outside the shared folder"})
		return
	}
	share, err := s.fileShare(value.ShareID)
	if err != nil {
		writeJSON(w, 410, map[string]string{"error": "the shared folder is no longer available"})
		return
	}
	entries, err := fileops.List(share.Path, rel)
	if err != nil {
		writeFileError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"shareName": share.Name, "path": pathOrRoot(rel), "entries": entries})
}

func (s *apiServer) publicFileShareDownload(w http.ResponseWriter, r *http.Request, token string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	value, ok := s.lookupFileShareLink(w, r, token)
	if !ok {
		return
	}
	rel, valid := scopedSharePath(value.Path, r.URL.Query().Get("path"))
	if !valid {
		writeJSON(w, 403, map[string]string{"error": "requested path is outside the shared folder"})
		return
	}
	share, err := s.fileShare(value.ShareID)
	if err != nil {
		writeJSON(w, 410, map[string]string{"error": "the shared folder is no longer available"})
		return
	}
	filePath, info, err := fileops.ResolveEntry(share.Path, rel, r.URL.Query().Get("name"))
	if err != nil {
		writeFileError(w, err)
		return
	}
	if info.IsDir() {
		writeJSON(w, 422, map[string]string{"error": "choose a file to download"})
		return
	}
	if err := s.store.RecordFileShareDownload(value.TokenHash, time.Now()); err != nil {
		writeJSON(w, 410, map[string]string{"error": "share link expired or revoked"})
		return
	}
	file, err := os.Open(filePath)
	if err != nil {
		writeFileError(w, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(info.Name(), `"`, "")+`"`)
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}
