package main

import (
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/lumonas/lumonas/internal/identity"
)

type sharePathAccess struct {
	ShareID          string `json:"shareId"`
	PrincipalID      string `json:"principalId"`
	Path             string `json:"path"`
	ShareEnabled     bool   `json:"shareEnabled"`
	AccountEnabled   bool   `json:"accountEnabled"`
	ShareLevel       string `json:"shareLevel"`
	FilesystemAccess string `json:"filesystemAccess"`
	Allowed          bool   `json:"allowed"`
	Explanation      string `json:"explanation"`
	Approximate      bool   `json:"approximate"`
}

// shareAccessCheck diagnoses configured share access together with the
// underlying Unix mode bits. Samba/NFS ACL modules can add behavior beyond
// mode bits, so the response explicitly labels the filesystem result as an
// approximation instead of presenting it as an authoritative protocol test.
func (s *apiServer) shareAccessCheck(w http.ResponseWriter, r *http.Request, shareID string) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	share, err := s.store.ManagedShare(shareID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "share not found"})
		return
	}
	principalID := strings.TrimSpace(r.URL.Query().Get("principal"))
	principal, err := s.store.Principal(principalID)
	if err != nil || principal.Kind != identity.KindUser {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "select an existing user account"})
		return
	}
	relative := strings.TrimSpace(r.URL.Query().Get("path"))
	if relative == "" {
		relative = "."
	}
	relative = filepath.Clean(filepath.FromSlash(relative))
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || strings.ContainsRune(relative, '\x00') {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "path must stay inside the selected share"})
		return
	}
	root, err := filepath.EvalSymlinks(share.Path)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "share path is unavailable"})
		return
	}
	target := filepath.Join(root, relative)
	resolved, resolveErr := filepath.EvalSymlinks(target)
	if resolveErr == nil {
		target = resolved
	} else if os.IsNotExist(resolveErr) {
		resolvedParent, parentErr := filepath.EvalSymlinks(filepath.Dir(target))
		if parentErr == nil {
			target = filepath.Join(resolvedParent, filepath.Base(target))
		} else if !os.IsNotExist(parentErr) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "path could not be safely resolved"})
			return
		}
	} else {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "path could not be safely resolved"})
		return
	}
	inside, err := filepath.Rel(root, target)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "path resolves outside the selected share"})
		return
	}
	level, err := s.store.ResolveShareAccess(shareID, principalID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not resolve share permissions"})
		return
	}
	fsAccess := unixPathAccess(root, target, principal)
	allowed := share.Enabled && principal.Enabled && level != identity.AccessNone && (fsAccess == "read" || fsAccess == "write")
	explanation := "The user can access this path based on the configured share rule and Unix mode bits."
	switch {
	case !share.Enabled:
		explanation = "The share is disabled."
	case !principal.Enabled:
		explanation = "The user account is disabled."
	case level == identity.AccessNone:
		explanation = "No direct or group share rule grants this user access."
	case fsAccess == "none":
		explanation = "Unix ownership or mode bits deny access to this path or one of its parent directories."
	case fsAccess == "unknown":
		explanation = "The account has no mapped Unix identity, so filesystem access could not be determined."
	}
	writeJSON(w, http.StatusOK, sharePathAccess{ShareID: share.ID, PrincipalID: principal.ID, Path: filepath.ToSlash(relative), ShareEnabled: share.Enabled, AccountEnabled: principal.Enabled, ShareLevel: string(level), FilesystemAccess: fsAccess, Allowed: allowed, Explanation: explanation, Approximate: true})
}

func unixPathAccess(root, target string, principal identity.Principal) string {
	if principal.UID == nil {
		return "unknown"
	}
	groups := map[uint64]bool{}
	for _, groupName := range principal.Groups {
		group, err := user.LookupGroup(groupName)
		if err == nil {
			if gid, parseErr := strconv.ParseUint(group.Gid, 10, 64); parseErr == nil {
				groups[gid] = true
			}
		}
	}
	if principal.GID != nil {
		groups[uint64(*principal.GID)] = true
	}
	for current := target; ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		if err != nil {
			if !os.IsNotExist(err) {
				return "none"
			}
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return "unknown"
		}
		mode := info.Mode().Perm()
		var bits os.FileMode
		if uint64(*principal.UID) == uint64(stat.Uid) {
			bits = (mode >> 6) & 7
		} else if groups[uint64(stat.Gid)] {
			bits = (mode >> 3) & 7
		} else {
			bits = mode & 7
		}
		if current != target && bits&1 == 0 {
			return "none"
		}
		if current == target {
			if info.IsDir() {
				if bits&3 == 3 {
					return "write"
				}
				if bits&5 == 5 {
					return "read"
				}
				return "none"
			}
			if bits&2 != 0 {
				return "write"
			}
			if bits&4 != 0 {
				return "read"
			}
			return "none"
		}
		if current == root {
			break
		}
	}
	return "none"
}
