package uploads

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	fileops "github.com/lumonas/lumonas/internal/files"
)

const (
	MaxUploadBytes int64 = 1 << 40
	MaxChunkBytes        = 8 << 20
)

// ErrInsufficientSpace reports that the upload volume cannot safely accept
// another chunk. Callers should surface it as a recoverable client error
// rather than retrying.
var ErrInsufficientSpace = errors.New("not enough free space on the upload volume")

type Session struct {
	ID              string    `json:"id"`
	ShareID         string    `json:"shareId"`
	ShareRoot       string    `json:"-"`
	Path            string    `json:"path"`
	Name            string    `json:"name"`
	SizeBytes       int64     `json:"sizeBytes"`
	ReceivedBytes   int64     `json:"receivedBytes"`
	ExpectedSHA256  string    `json:"expectedSha256,omitempty"`
	ReplaceExisting bool      `json:"replaceExisting,omitempty"`
	ExpectedSize    int64     `json:"expectedSizeBytes,omitempty"`
	ExpectedModTime time.Time `json:"expectedModifiedAt,omitempty"`
	FinalName       string    `json:"finalName,omitempty"`
	LastChunkOffset int64     `json:"lastChunkOffset"`
	LastChunkBytes  int64     `json:"lastChunkBytes"`
	LastChunkSHA256 string    `json:"lastChunkSha256,omitempty"`
	State           string    `json:"state"`
	CreatedAt       time.Time `json:"createdAt"`
}

type Manager struct{ Root string }

type CreateOptions struct {
	ReplaceExisting bool
	ExpectedSize    int64
	ExpectedModTime time.Time
}

type persistedSession struct {
	Session   Session `json:"session"`
	ShareRoot string  `json:"shareRoot"`
}

func (m Manager) Create(shareID, shareRoot, relativePath, name string, size int64, expectedSHA256 string) (Session, error) {
	return m.CreateWithOptions(shareID, shareRoot, relativePath, name, size, expectedSHA256, CreateOptions{})
}

func (m Manager) CreateWithOptions(shareID, shareRoot, relativePath, name string, size int64, expectedSHA256 string, options CreateOptions) (Session, error) {
	if shareID == "" || !filepath.IsAbs(shareRoot) || size < 0 || size > MaxUploadBytes {
		return Session{}, errors.New("invalid upload metadata")
	}
	if err := fileops.ValidateName(name); err != nil {
		return Session{}, err
	}
	if _, err := fileops.Resolve(shareRoot, relativePath); err != nil {
		return Session{}, err
	}
	if options.ReplaceExisting && (options.ExpectedSize < 0 || options.ExpectedModTime.IsZero()) {
		return Session{}, errors.New("replacing an existing file requires its expected size and modification time")
	}
	if !options.ReplaceExisting && (!options.ExpectedModTime.IsZero() || options.ExpectedSize != 0) {
		return Session{}, errors.New("replacement metadata requires replaceExisting")
	}
	expectedSHA256 = strings.ToLower(strings.TrimSpace(expectedSHA256))
	if expectedSHA256 != "" {
		if len(expectedSHA256) != sha256.Size*2 {
			return Session{}, errors.New("expectedSha256 must be a SHA-256 hex digest")
		}
		if _, err := hex.DecodeString(expectedSHA256); err != nil {
			return Session{}, errors.New("expectedSha256 must be a SHA-256 hex digest")
		}
	}
	if err := os.MkdirAll(m.Root, 0o700); err != nil {
		return Session{}, err
	}
	m.expireOldSessions(time.Now().UTC().Add(-24 * time.Hour))
	var idBytes [16]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return Session{}, err
	}
	id := hex.EncodeToString(idBytes[:])
	directory := filepath.Join(m.Root, id)
	if err := os.Mkdir(directory, 0o700); err != nil {
		return Session{}, err
	}
	if err := os.WriteFile(filepath.Join(directory, "data.part"), nil, 0o600); err != nil {
		_ = os.RemoveAll(directory)
		return Session{}, err
	}
	session := Session{ID: id, ShareID: shareID, ShareRoot: filepath.Clean(shareRoot), Path: relativePath, Name: name, SizeBytes: size, ExpectedSHA256: expectedSHA256, ReplaceExisting: options.ReplaceExisting, ExpectedSize: options.ExpectedSize, ExpectedModTime: options.ExpectedModTime.UTC(), LastChunkOffset: -1, State: "uploading", CreatedAt: time.Now().UTC()}
	if err := m.save(session); err != nil {
		_ = os.RemoveAll(directory)
		return Session{}, err
	}
	return session, nil
}

func (m Manager) expireOldSessions(before time.Time) {
	entries, err := os.ReadDir(m.Root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || !validID(entry.Name()) {
			continue
		}
		value, err := m.Get(entry.Name())
		if err == nil && value.CreatedAt.Before(before) {
			_ = os.RemoveAll(filepath.Join(m.Root, entry.Name()))
		}
	}
}

func (m Manager) Get(id string) (Session, error) {
	if !validID(id) {
		return Session{}, os.ErrNotExist
	}
	data, err := os.ReadFile(filepath.Join(m.Root, id, "session.json"))
	if err != nil {
		return Session{}, err
	}
	var persisted persistedSession
	if err := json.Unmarshal(data, &persisted); err != nil || persisted.Session.ID != id || !filepath.IsAbs(persisted.ShareRoot) {
		return Session{}, errors.New("upload session metadata is invalid")
	}
	value := persisted.Session
	value.ShareRoot = persisted.ShareRoot
	return value, nil
}

func (m Manager) WriteChunk(id string, offset int64, input io.Reader) (Session, bool, error) {
	value, err := m.Get(id)
	if err != nil {
		return Session{}, false, err
	}
	if value.State != "uploading" || offset < 0 {
		return Session{}, false, errors.New("upload session is not accepting chunks")
	}
	data, err := io.ReadAll(io.LimitReader(input, MaxChunkBytes+1))
	if err != nil {
		return Session{}, false, err
	}
	if len(data) == 0 || len(data) > MaxChunkBytes {
		return Session{}, false, errors.New("chunk size is invalid")
	}
	digest := sha256.Sum256(data)
	chunkHash := hex.EncodeToString(digest[:])
	if offset == value.LastChunkOffset && int64(len(data)) == value.LastChunkBytes && chunkHash == value.LastChunkSHA256 {
		return value, true, nil
	}
	if offset != value.ReceivedBytes || int64(len(data)) > value.SizeBytes-value.ReceivedBytes {
		return value, false, fmt.Errorf("upload offset mismatch: server expects %d", value.ReceivedBytes)
	}
	// Refuse to stage a chunk the filesystem cannot hold. Filling the volume
	// takes down unrelated services and can corrupt the data the user was
	// trying to protect, so fail before the write rather than after.
	if available, err := availableBytes(m.Root); err == nil {
		// Keep a margin so a nearly-full volume still has room for the
		// final rename and the store's own bookkeeping.
		const reserveBytes = 64 << 20
		if available <= reserveBytes+int64(len(data)) {
			return value, false, ErrInsufficientSpace
		}
	}
	file, err := os.OpenFile(filepath.Join(m.Root, id, "data.part"), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return Session{}, false, err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return Session{}, false, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return Session{}, false, err
	}
	if err := file.Close(); err != nil {
		return Session{}, false, err
	}
	value.LastChunkOffset, value.LastChunkBytes, value.LastChunkSHA256 = offset, int64(len(data)), chunkHash
	value.ReceivedBytes += int64(len(data))
	if err := m.save(value); err != nil {
		return Session{}, false, err
	}
	return value, false, nil
}

func (m Manager) Complete(id string) (Session, string, error) {
	value, err := m.Get(id)
	if err != nil {
		return Session{}, "", err
	}
	if value.State == "completed" {
		return value, value.FinalName, nil
	}
	if value.State != "uploading" || value.ReceivedBytes != value.SizeBytes {
		return Session{}, "", errors.New("upload is incomplete")
	}
	partPath := filepath.Join(m.Root, id, "data.part")
	file, err := os.Open(partPath)
	if err != nil {
		return Session{}, "", err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		_ = file.Close()
		return Session{}, "", err
	}
	if err := file.Close(); err != nil {
		return Session{}, "", err
	}
	if value.ExpectedSHA256 != "" && hex.EncodeToString(hash.Sum(nil)) != value.ExpectedSHA256 {
		return Session{}, "", errors.New("uploaded file checksum does not match")
	}
	file, err = os.Open(partPath)
	if err != nil {
		return Session{}, "", err
	}
	var name string
	var writeErr error
	if value.ReplaceExisting {
		name, writeErr = fileops.WriteUploadReplacing(value.ShareRoot, value.Path, value.Name, file, value.SizeBytes, value.ExpectedSize, value.ExpectedModTime)
	} else {
		name, writeErr = fileops.WriteUpload(value.ShareRoot, value.Path, value.Name, file, value.SizeBytes)
	}
	closeErr := file.Close()
	if writeErr != nil {
		return Session{}, "", writeErr
	}
	if closeErr != nil {
		return Session{}, "", closeErr
	}
	value.State, value.FinalName = "completed", name
	if err := m.save(value); err != nil {
		return Session{}, "", err
	}
	return value, name, nil
}

func (m Manager) Cancel(id string) error {
	if !validID(id) {
		return os.ErrNotExist
	}
	return os.RemoveAll(filepath.Join(m.Root, id))
}

func (m Manager) save(value Session) error {
	data, err := json.Marshal(persistedSession{Session: value, ShareRoot: value.ShareRoot})
	if err != nil {
		return err
	}
	path := filepath.Join(m.Root, value.ID, "session.json")
	temporary, err := os.CreateTemp(filepath.Dir(path), ".session-*.tmp")
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

func validID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 16 && filepath.Base(id) == id
}
