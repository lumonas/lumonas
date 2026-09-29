package workstationbackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type SyncOptions struct {
	Source     string
	RemotePath string
	Selection  Selection
	DryRun     bool
	Overwrite  bool
}

type SyncReport struct {
	Added           []string `json:"added"`
	Updated         []string `json:"updated"`
	Unchanged       []string `json:"unchanged"`
	Conflicts       []string `json:"conflicts"`
	SkippedSymlinks int      `json:"skippedSymlinks"`
	BytesUploaded   int64    `json:"bytesUploaded"`
}

type syncBaseline struct {
	Version    int                 `json:"version"`
	Server     string              `json:"server"`
	ShareID    string              `json:"shareId"`
	Source     string              `json:"source"`
	RemotePath string              `json:"remotePath"`
	Files      map[string]syncFile `json:"files"`
}

type syncFile struct {
	SHA256         string    `json:"sha256"`
	SizeBytes      int64     `json:"sizeBytes"`
	RemoteSize     int64     `json:"remoteSizeBytes"`
	RemoteModified time.Time `json:"remoteModifiedAt"`
}

type remoteEntry struct {
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

// SyncPush copies selected workstation files to one NAS share. It never
// removes NAS files; changed files are replaced only when their remote
// size/mtime still match the last successful client sync. Existing files that
// have no baseline are conflicts unless Overwrite is explicitly requested.
func (c *Client) SyncPush(ctx context.Context, options SyncOptions) (SyncReport, error) {
	report := SyncReport{Added: []string{}, Updated: []string{}, Unchanged: []string{}, Conflicts: []string{}}
	if err := options.Selection.Validate(); err != nil {
		return report, err
	}
	source, err := filepath.Abs(options.Source)
	if err != nil {
		return report, err
	}
	rootInfo, err := os.Lstat(source)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return report, errors.New("sync source must be an accessible real directory")
	}
	remotePath, err := cleanSyncPath(options.RemotePath)
	if err != nil {
		return report, err
	}
	statePath := c.syncStatePath(source, remotePath)
	unlock, err := c.lock(statePath)
	if err != nil {
		return report, err
	}
	defer unlock()
	state := syncBaseline{Version: 1, Server: c.base, ShareID: c.config.ShareID, Source: source, RemotePath: remotePath, Files: map[string]syncFile{}}
	if encoded, readErr := os.ReadFile(statePath); readErr == nil {
		if json.Unmarshal(encoded, &state) != nil || state.Version != 1 || state.Server != c.base || state.ShareID != c.config.ShareID || state.Source != source || state.RemotePath != remotePath || state.Files == nil {
			return report, errors.New("saved sync state is invalid or belongs to a different NAS, share, or folder")
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return report, readErr
	}
	localFiles, localDirs, skipped, err := scanSyncSource(source, options.Selection)
	if err != nil {
		return report, err
	}
	report.SkippedSymlinks = skipped
	baseExists, err := c.ensureRemoteBase(ctx, remotePath, !options.DryRun)
	if err != nil {
		return report, err
	}
	remoteDirs, remoteFiles := map[string]bool{".": true}, map[string]remoteEntry{}
	if baseExists {
		remoteDirs, remoteFiles, err = c.remoteSyncTree(ctx, remotePath)
		if err != nil {
			return report, err
		}
	}
	if !options.DryRun {
		for _, relative := range sortedDirs(localDirs) {
			if remoteDirs[relative] {
				continue
			}
			if err := c.createRemoteSyncDir(ctx, remotePath, relative); err != nil {
				return report, fmt.Errorf("create remote folder %q: %w", relative, err)
			}
			remoteDirs[relative] = true
		}
	}

	paths := make([]string, 0, len(localFiles))
	for relative := range localFiles {
		paths = append(paths, relative)
	}
	sort.Strings(paths)
	for _, relative := range paths {
		local := localFiles[relative]
		remote, exists := remoteFiles[relative]
		baseline, hasBaseline := state.Files[relative]
		if exists && !hasBaseline && !options.Overwrite {
			report.Conflicts = append(report.Conflicts, relative)
			continue
		}
		if exists && hasBaseline {
			localUnchanged := local.hash == baseline.SHA256 && local.info.Size() == baseline.SizeBytes
			remoteUnchanged := remote.SizeBytes == baseline.RemoteSize && remote.ModifiedAt.UTC().Equal(baseline.RemoteModified.UTC())
			if localUnchanged && remoteUnchanged {
				report.Unchanged = append(report.Unchanged, relative)
				continue
			}
			if !remoteUnchanged && !options.Overwrite {
				report.Conflicts = append(report.Conflicts, relative)
				continue
			}
		}
		if options.DryRun {
			if exists {
				report.Updated = append(report.Updated, relative)
			} else {
				report.Added = append(report.Added, relative)
			}
			continue
		}
		var expected *remoteEntry
		if exists {
			copy := remote
			expected = &copy
		}
		if err := c.uploadSyncFile(ctx, filepath.Join(source, filepath.FromSlash(relative)), remotePathFor(remotePath, path.Dir(relative)), path.Base(relative), local.hash, local.info.Size(), expected); err != nil {
			return report, fmt.Errorf("sync %q: %w", relative, err)
		}
		verified, err := c.remoteFileProperties(ctx, remotePathFor(remotePath, path.Dir(relative)), path.Base(relative))
		if err != nil {
			return report, fmt.Errorf("verify uploaded file %q: %w", relative, err)
		}
		state.Files[relative] = syncFile{SHA256: local.hash, SizeBytes: local.info.Size(), RemoteSize: verified.SizeBytes, RemoteModified: verified.ModifiedAt.UTC()}
		if err := writeSyncBaseline(statePath, state); err != nil {
			return report, err
		}
		report.BytesUploaded += local.info.Size()
		if exists {
			report.Updated = append(report.Updated, relative)
		} else {
			report.Added = append(report.Added, relative)
		}
	}
	return report, nil
}

type localSyncFile struct {
	info os.FileInfo
	hash string
}

func scanSyncSource(root string, selection Selection) (map[string]localSyncFile, map[string]bool, int, error) {
	files := map[string]localSyncFile{}
	dirs := map[string]bool{}
	skipped := 0
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == root {
			return nil
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.Type()&os.ModeSymlink != 0 {
			skipped++
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if selection.excluded(rel) || (len(selection.Include) > 0 && !selection.mayContainIncluded(rel)) {
				return filepath.SkipDir
			}
			dirs[rel] = true
			return nil
		}
		if !entry.Type().IsRegular() || selection.excluded(rel) || !selection.includes(rel) {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		linkInfo, err := os.Lstat(current)
		if err != nil || !linkInfo.Mode().IsRegular() || linkInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("sync source changed while scanning %q", rel)
		}
		file, err := os.Open(current)
		if err != nil {
			return err
		}
		openedInfo, err := file.Stat()
		if err != nil || !os.SameFile(linkInfo, openedInfo) || !openedInfo.Mode().IsRegular() {
			_ = file.Close()
			return fmt.Errorf("sync source changed while opening %q", rel)
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		finishedInfo, statErr := file.Stat()
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		latestInfo, lstatErr := os.Lstat(current)
		if statErr != nil || lstatErr != nil || !os.SameFile(openedInfo, finishedInfo) || !os.SameFile(finishedInfo, latestInfo) || finishedInfo.Size() != fileInfo.Size() || !finishedInfo.ModTime().Equal(fileInfo.ModTime()) {
			return fmt.Errorf("sync source changed while reading %q", rel)
		}
		files[rel] = localSyncFile{info: fileInfo, hash: hex.EncodeToString(hash.Sum(nil))}
		return nil
	})
	return files, dirs, skipped, err
}

func cleanSyncPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.ContainsAny(value, "\\\x00\r\n") {
		return "", errors.New("NAS path contains an unsupported character")
	}
	for _, component := range strings.Split(value, "/") {
		if component == ".." {
			return "", errors.New("NAS path cannot traverse outside the selected share")
		}
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(value, "/"))
	if cleaned == "/." || cleaned == "/" {
		return ".", nil
	}
	return strings.TrimPrefix(cleaned, "/"), nil
}

func remotePathFor(base, relative string) string {
	if relative == "." || relative == "" {
		return base
	}
	if base == "." {
		return relative
	}
	return path.Join(base, relative)
}

func sortedDirs(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		di, dj := strings.Count(result[i], "/"), strings.Count(result[j], "/")
		if di == dj {
			return result[i] < result[j]
		}
		return di < dj
	})
	return result
}

func (c *Client) remoteSyncTree(ctx context.Context, root string) (map[string]bool, map[string]remoteEntry, error) {
	dirs := map[string]bool{".": true}
	files := map[string]remoteEntry{}
	queue := []string{"."}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		entries, err := c.listRemoteSyncDir(ctx, remotePathFor(root, current))
		if err != nil {
			return nil, nil, err
		}
		for _, entry := range entries {
			relative := entry.Name
			if current != "." {
				relative = path.Join(current, entry.Name)
			}
			if entry.Type == "dir" {
				dirs[relative] = true
				queue = append(queue, relative)
			} else if entry.Type == "file" {
				files[relative] = entry
			}
		}
	}
	return dirs, files, nil
}

func (c *Client) ensureRemoteBase(ctx context.Context, root string, create bool) (bool, error) {
	if root == "." {
		return true, nil
	}
	current := "."
	for _, component := range strings.Split(root, "/") {
		entries, err := c.listRemoteSyncDir(ctx, current)
		if err != nil {
			return false, err
		}
		found := false
		for _, entry := range entries {
			if entry.Name != component {
				continue
			}
			if entry.Type != "dir" {
				return false, fmt.Errorf("NAS path component %q is not a directory", component)
			}
			found = true
			break
		}
		if !found {
			if !create {
				return false, nil
			}
			body, _ := json.Marshal(map[string]string{"shareId": c.config.ShareID, "path": current, "name": component})
			if err := c.requestJSON(ctx, http.MethodPost, "/api/v1/files/mkdir", body, nil, nil); err != nil {
				return false, err
			}
		}
		current = remotePathFor(current, component)
	}
	return true, nil
}

func (c *Client) listRemoteSyncDir(ctx context.Context, remote string) ([]remoteEntry, error) {
	query := url.Values{"share": []string{c.config.ShareID}, "path": []string{remote}}
	var result struct {
		Entries []remoteEntry `json:"entries"`
	}
	if err := c.requestJSON(ctx, http.MethodGet, "/api/v1/files?"+query.Encode(), nil, &result, nil); err != nil {
		return nil, err
	}
	return result.Entries, nil
}

func (c *Client) createRemoteSyncDir(ctx context.Context, base, relative string) error {
	parent := path.Dir(relative)
	if parent == "." {
		parent = "."
	}
	body, _ := json.Marshal(map[string]string{"shareId": c.config.ShareID, "path": remotePathFor(base, parent), "name": path.Base(relative)})
	return c.requestJSON(ctx, http.MethodPost, "/api/v1/files/mkdir", body, nil, nil)
}

func (c *Client) remoteFileProperties(ctx context.Context, directory, name string) (remoteEntry, error) {
	query := url.Values{"share": []string{c.config.ShareID}, "path": []string{directory}, "name": []string{name}}
	var result remoteEntry
	if err := c.requestJSON(ctx, http.MethodGet, "/api/v1/files/properties?"+query.Encode(), nil, &result, nil); err != nil {
		return remoteEntry{}, err
	}
	return result, nil
}

func (c *Client) uploadSyncFile(ctx context.Context, localPath, remoteDir, name, digest string, size int64, expected *remoteEntry) error {
	linkInfo, err := os.Lstat(localPath)
	if err != nil || !linkInfo.Mode().IsRegular() || linkInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("local file is no longer a regular file")
	}
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(linkInfo, info) || info.Size() != size {
		return errors.New("local file changed during sync planning")
	}
	bodyValues := map[string]any{"shareId": c.config.ShareID, "path": remoteDir, "name": name, "sizeBytes": size, "expectedSha256": digest}
	if expected != nil {
		bodyValues["replaceExisting"] = true
		bodyValues["expectedSizeBytes"] = expected.SizeBytes
		bodyValues["expectedModifiedAt"] = expected.ModifiedAt.UTC()
	}
	body, _ := json.Marshal(bodyValues)
	var session uploadSession
	if err := c.requestJSON(ctx, http.MethodPost, "/api/v1/files/uploads", body, &session, nil); err != nil {
		return err
	}
	if session.ID == "" || session.ShareID != c.config.ShareID || session.Path != remoteDir || session.Name != name || session.SizeBytes != size {
		_ = c.cancelUpload(context.Background(), session.ID)
		return errors.New("NAS returned a mismatched upload session")
	}
	if _, err := file.Seek(session.ReceivedBytes, io.SeekStart); err != nil {
		return err
	}
	offset := session.ReceivedBytes
	buffer := make([]byte, uploadChunkBytes)
	retries := 0
	for offset < size {
		if err := ctx.Err(); err != nil {
			return err
		}
		limit := int64(len(buffer))
		if remaining := size - offset; remaining < limit {
			limit = remaining
		}
		count, readErr := io.ReadFull(file, buffer[:limit])
		if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) {
			return readErr
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, c.base+"/api/v1/files/uploads/"+url.PathEscape(session.ID), bytes.NewReader(buffer[:count]))
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+c.config.Token)
		request.Header.Set("Upload-Offset", fmt.Sprint(offset))
		response, requestErr := c.http.Do(request)
		if requestErr != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
			if response != nil {
				response.Body.Close()
			}
			var latest uploadSession
			if getErr := c.requestJSON(ctx, http.MethodGet, "/api/v1/files/uploads/"+url.PathEscape(session.ID), nil, &latest, nil); getErr != nil {
				if requestErr != nil {
					return requestErr
				}
				return getErr
			}
			if latest.ReceivedBytes < offset || latest.ReceivedBytes > size {
				return errors.New("NAS returned an invalid resumed upload offset")
			}
			retries++
			if retries > 3 {
				return errors.New("upload could not continue after three recovery attempts")
			}
			offset = latest.ReceivedBytes
			if _, err := file.Seek(offset, io.SeekStart); err != nil {
				return err
			}
			continue
		}
		response.Body.Close()
		retries = 0
		next := offset + int64(count)
		if raw := response.Header.Get("Upload-Offset"); raw != "" {
			parsed, parseErr := strconv.ParseInt(raw, 10, 64)
			if parseErr != nil || parsed != next {
				return errors.New("NAS returned an invalid upload offset")
			}
		}
		offset = next
	}
	var completed struct {
		Name string `json:"name"`
	}
	return c.requestJSON(ctx, http.MethodPost, "/api/v1/files/uploads/"+url.PathEscape(session.ID)+"/complete", nil, &completed, nil)
}

func (c *Client) syncStatePath(source, remotePath string) string {
	digest := sha256.Sum256([]byte(c.base + "\x00" + c.config.ShareID + "\x00" + source + "\x00" + remotePath))
	return filepath.Join(c.config.StateDir, "sync-"+hex.EncodeToString(digest[:12])+".json")
}

func writeSyncBaseline(file string, state syncBaseline) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(file), ".sync-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, file)
}
