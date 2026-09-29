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
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const uploadChunkBytes = 8 << 20

type Config struct {
	ServerURL string
	Token     string
	ShareID   string
	Key       []byte
	StateDir  string
	HTTP      *http.Client
}

type Client struct {
	config Config
	base   string
	http   *http.Client
}

type uploadSession struct {
	ID            string `json:"id"`
	ShareID       string `json:"shareId"`
	Path          string `json:"path"`
	Name          string `json:"name"`
	SizeBytes     int64  `json:"sizeBytes"`
	ReceivedBytes int64  `json:"receivedBytes"`
	ExpectedSHA   string `json:"expectedSha256"`
	State         string `json:"state"`
	FinalName     string `json:"finalName"`
}

type pendingUpload struct {
	Version        int    `json:"version"`
	Server         string `json:"server"`
	ShareID        string `json:"shareId"`
	Source         string `json:"source"`
	RemotePath     string `json:"remotePath"`
	SelectionHash  string `json:"selectionHash,omitempty"`
	Name           string `json:"name"`
	ArchivePath    string `json:"archivePath"`
	SizeBytes      int64  `json:"sizeBytes"`
	SHA256         string `json:"sha256"`
	KeyFingerprint string `json:"keyFingerprint"`
	UploadID       string `json:"uploadId"`
}

func NewClient(config Config) (*Client, error) {
	if strings.TrimSpace(config.Token) == "" || !regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`).MatchString(config.ShareID) {
		return nil, errors.New("a scoped API token and backup share ID are required")
	}
	parsed, err := url.Parse(strings.TrimSpace(config.ServerURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost" || parsed.Hostname() == "::1"))) {
		return nil, errors.New("server URL must use HTTPS (HTTP is allowed only for loopback development)")
	}
	if config.StateDir == "" {
		root, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		config.StateDir = filepath.Join(root, "LumoNAS", "workstation-backup")
	}
	config.StateDir, err = filepath.Abs(config.StateDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(config.StateDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(config.StateDir, 0o700); err != nil {
		return nil, err
	}
	client := config.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Minute, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &Client{config: config, base: strings.TrimRight(parsed.String(), "/"), http: client}, nil
}

func (c *Client) Backup(ctx context.Context, source, remotePath string) (string, error) {
	return c.BackupSelected(ctx, source, remotePath, Selection{})
}

func (c *Client) BackupSelected(ctx context.Context, source, remotePath string, selection Selection) (string, error) {
	if err := verifyKey(c.config.Key); err != nil {
		return "", err
	}
	if remotePath == "" {
		remotePath = "/"
	}
	if err := selection.Validate(); err != nil {
		return "", err
	}
	source, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(source)
	if err != nil || !info.IsDir() {
		return "", errors.New("backup source must be an accessible directory")
	}
	statePath := c.statePath(source, remotePath)
	lock, err := c.lock(statePath)
	if err != nil {
		return "", err
	}
	defer lock()
	pending, session, err := c.resumePending(ctx, statePath, source, remotePath, selection.hash())
	if err != nil {
		return "", err
	}
	if pending == nil {
		pending, err = c.prepareUpload(ctx, statePath, source, remotePath, selection)
		if err != nil {
			return "", err
		}
	}
	if pending.UploadID == "" {
		session, err = c.createSession(ctx, pending)
		if err != nil {
			return "", err
		}
		pending.UploadID = session.ID
		if err := writePending(statePath, pending); err != nil {
			_ = c.cancelUpload(context.Background(), session.ID)
			return "", err
		}
	}
	if session.State == "completed" {
		name := session.FinalName
		if name == "" {
			name = pending.Name
		}
		_ = os.Remove(pending.ArchivePath)
		_ = os.Remove(statePath)
		return name, nil
	}
	if err := c.transfer(ctx, pending, session); err != nil {
		return "", err
	}
	var result struct {
		Name string `json:"name"`
	}
	if err := c.requestJSON(ctx, http.MethodPost, "/api/v1/files/uploads/"+url.PathEscape(pending.UploadID)+"/complete", nil, &result, nil); err != nil {
		return "", err
	}
	if result.Name == "" {
		result.Name = pending.Name
	}
	_ = os.Remove(pending.ArchivePath)
	_ = os.Remove(statePath)
	return result.Name, nil
}

func (c *Client) prepareUpload(ctx context.Context, statePath, source, remotePath string, selection Selection) (*pendingUpload, error) {
	if err := os.MkdirAll(c.config.StateDir, 0o700); err != nil {
		return nil, err
	}
	temporary, err := os.CreateTemp(c.config.StateDir, ".workstation-*.lwb")
	if err != nil {
		return nil, err
	}
	archivePath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(archivePath)
		return nil, err
	}
	if err := os.Remove(archivePath); err != nil {
		return nil, err
	}
	if err := createEncryptedArchiveSelected(source, archivePath, c.config.Key, selection); err != nil {
		_ = os.Remove(archivePath)
		return nil, err
	}
	hash, size, err := encryptedFileSHA256(archivePath)
	if err != nil {
		_ = os.Remove(archivePath)
		return nil, err
	}
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "workstation"
	}
	hostname = regexp.MustCompile(`[^A-Za-z0-9_-]+`).ReplaceAllString(hostname, "-")
	name := fmt.Sprintf("lumo-%s-%s.lwb", strings.Trim(hostname, "-"), time.Now().UTC().Format("20060102T150405Z"))
	fingerprint := sha256.Sum256(c.config.Key)
	pending := &pendingUpload{Version: 1, Server: c.base, ShareID: c.config.ShareID, Source: source, RemotePath: remotePath, SelectionHash: selection.hash(), Name: name, ArchivePath: archivePath, SizeBytes: size, SHA256: hash, KeyFingerprint: hex.EncodeToString(fingerprint[:])}
	if err := writePending(statePath, pending); err != nil {
		_ = os.Remove(archivePath)
		return nil, err
	}
	return pending, nil
}

func (c *Client) resumePending(ctx context.Context, statePath, source, remotePath, selectionHash string) (*pendingUpload, uploadSession, error) {
	data, err := os.ReadFile(statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, uploadSession{}, nil
	}
	if err != nil {
		return nil, uploadSession{}, err
	}
	var pending pendingUpload
	if json.Unmarshal(data, &pending) != nil || pending.Version != 1 || pending.Server != c.base || pending.ShareID != c.config.ShareID || pending.Source != source || pending.RemotePath != remotePath || pending.SelectionHash != selectionHash || filepath.Dir(pending.ArchivePath) != c.config.StateDir {
		return nil, uploadSession{}, errors.New("saved backup upload state is invalid or belongs to a different configuration; remove the local state file to start fresh")
	}
	fingerprint := sha256.Sum256(c.config.Key)
	if pending.KeyFingerprint != hex.EncodeToString(fingerprint[:]) {
		return nil, uploadSession{}, errors.New("backup encryption key changed while an upload was pending")
	}
	archiveInfo, err := os.Lstat(pending.ArchivePath)
	if err != nil || !archiveInfo.Mode().IsRegular() || archiveInfo.Mode()&os.ModeSymlink != 0 {
		return nil, uploadSession{}, errors.New("pending encrypted archive is missing; remove the local state file to start fresh")
	}
	if pending.UploadID == "" {
		return &pending, uploadSession{}, nil
	}
	var session uploadSession
	if err := c.requestJSON(ctx, http.MethodGet, "/api/v1/files/uploads/"+url.PathEscape(pending.UploadID), nil, &session, nil); err != nil {
		if errors.Is(err, errNotFound) {
			_ = os.Remove(pending.ArchivePath)
			_ = os.Remove(statePath)
			return nil, uploadSession{}, nil
		}
		return nil, uploadSession{}, err
	}
	if session.ShareID != pending.ShareID || session.Path != pending.RemotePath || session.Name != pending.Name || session.SizeBytes != pending.SizeBytes || session.ExpectedSHA != pending.SHA256 || session.ReceivedBytes < 0 || session.ReceivedBytes > session.SizeBytes || (session.State != "uploading" && session.State != "completed") {
		return nil, uploadSession{}, errors.New("server upload state does not match the saved encrypted archive")
	}
	return &pending, session, nil
}

func (c *Client) createSession(ctx context.Context, pending *pendingUpload) (uploadSession, error) {
	body, err := json.Marshal(map[string]any{"shareId": pending.ShareID, "path": pending.RemotePath, "name": pending.Name, "sizeBytes": pending.SizeBytes, "expectedSha256": pending.SHA256})
	if err != nil {
		return uploadSession{}, err
	}
	var session uploadSession
	err = c.requestJSON(ctx, http.MethodPost, "/api/v1/files/uploads", body, &session, nil)
	return session, err
}

func (c *Client) transfer(ctx context.Context, pending *pendingUpload, session uploadSession) error {
	file, err := os.Open(pending.ArchivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Seek(session.ReceivedBytes, io.SeekStart); err != nil {
		return err
	}
	offset := session.ReceivedBytes
	buffer := make([]byte, uploadChunkBytes)
	for offset < pending.SizeBytes {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		limit := int64(len(buffer))
		if remaining := pending.SizeBytes - offset; remaining < limit {
			limit = remaining
		}
		count, err := io.ReadFull(file, buffer[:limit])
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return err
		}
		if count == 0 {
			return io.ErrUnexpectedEOF
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, c.base+"/api/v1/files/uploads/"+url.PathEscape(pending.UploadID), bytes.NewReader(buffer[:count]))
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+c.config.Token)
		request.Header.Set("Upload-Offset", fmt.Sprintf("%d", offset))
		response, err := c.http.Do(request)
		if err != nil {
			return err
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		nextOffset := response.Header.Get("Upload-Offset")
		status := response.StatusCode
		_ = response.Body.Close()
		if readErr != nil {
			return readErr
		}
		if status != http.StatusNoContent && status != http.StatusOK {
			return fmt.Errorf("NAS rejected upload chunk: HTTP %d: %s", status, strings.TrimSpace(string(responseBody)))
		}
		offset += int64(count)
		if nextOffset != "" {
			parsedOffset, err := strconv.ParseInt(nextOffset, 10, 64)
			if err != nil || parsedOffset != offset {
				return errors.New("NAS returned an invalid upload offset")
			}
		}
		if err := writePending(c.statePath(pending.Source, pending.RemotePath), pending); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Restore(ctx context.Context, filename, remotePath, destination string, overwrite bool) error {
	if err := verifyKey(c.config.Key); err != nil {
		return err
	}
	if filename == "" || filepath.Base(filename) != filename {
		return errors.New("restore filename must name a single archive in the selected share folder")
	}
	if remotePath == "" {
		remotePath = "/"
	}
	query := url.Values{}
	query.Set("share", c.config.ShareID)
	query.Set("path", remotePath)
	query.Set("name", filename)
	reference, err := url.Parse(c.base + "/api/v1/files/download")
	if err != nil {
		return err
	}
	reference.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, reference.String(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.config.Token)
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return fmt.Errorf("NAS download failed: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	temporary, err := os.CreateTemp("", "lumonas-workstation-*.lwb")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := io.Copy(temporary, response.Body); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return extractArchive(temporaryPath, destination, c.config.Key, overwrite)
}

func (c *Client) ListArchives(ctx context.Context, remotePath string) ([]string, error) {
	if remotePath == "" {
		remotePath = "/"
	}
	query := url.Values{}
	query.Set("share", c.config.ShareID)
	query.Set("path", remotePath)
	reference, err := url.Parse(c.base + "/api/v1/files")
	if err != nil {
		return nil, err
	}
	reference.RawQuery = query.Encode()
	var result struct {
		Entries []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"entries"`
	}
	if err := c.requestJSON(ctx, http.MethodGet, reference.RequestURI(), nil, &result, nil); err != nil {
		return nil, err
	}
	archives := make([]string, 0, len(result.Entries))
	for _, entry := range result.Entries {
		if entry.Type == "file" && strings.HasSuffix(strings.ToLower(entry.Name), ".lwb") {
			archives = append(archives, entry.Name)
		}
	}
	return archives, nil
}

var errNotFound = errors.New("upload session does not exist")

func (c *Client) requestJSON(ctx context.Context, method, endpoint string, body []byte, output any, headers http.Header) error {
	request, err := http.NewRequestWithContext(ctx, method, c.base+endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.config.Token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return fmt.Errorf("NAS request failed: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if output != nil {
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output); err != nil {
			return fmt.Errorf("invalid NAS response: %w", err)
		}
	}
	return nil
}

func (c *Client) cancelUpload(ctx context.Context, id string) error {
	return c.requestJSON(ctx, http.MethodDelete, "/api/v1/files/uploads/"+url.PathEscape(id), nil, nil, nil)
}

func (c *Client) statePath(source, remotePath string) string {
	digest := sha256.Sum256([]byte(c.base + "\x00" + c.config.ShareID + "\x00" + source + "\x00" + remotePath))
	return filepath.Join(c.config.StateDir, "pending-"+hex.EncodeToString(digest[:8])+".json")
}

func (c *Client) lock(statePath string) (func(), error) {
	path := statePath + ".lock"
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			info, statErr := os.Stat(path)
			if statErr == nil && time.Since(info.ModTime()) > 24*time.Hour {
				_ = os.Remove(path)
				return c.lock(statePath)
			}
			return nil, errors.New("another workstation backup is already running for this source; if the previous process crashed, remove its stale lock file after 24 hours")
		}
		return nil, err
	}
	_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
	_ = file.Close()
	return func() { _ = os.Remove(path) }, nil
}

func writePending(path string, pending *pendingUpload) error {
	data, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pending-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
