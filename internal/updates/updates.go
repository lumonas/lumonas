package updates

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const ManifestFormatVersion = 1

const DefaultMaxBootAttempts = 3

type Manifest struct {
	FormatVersion int       `json:"formatVersion"`
	Version       string    `json:"version"`
	PackageSHA256 string    `json:"packageSha256"`
	PackageSize   int64     `json:"packageSize"`
	ReleaseURL    string    `json:"releaseUrl,omitempty"`
	PublishedAt   time.Time `json:"publishedAt"`
	Notes         string    `json:"notes,omitempty"`
}

type Signature struct {
	Manifest string `json:"manifest"`
	Value    string `json:"value"`
}

type SlotState struct {
	ActiveSlot     string    `json:"activeSlot"`
	PreviousSlot   string    `json:"previousSlot,omitempty"`
	PendingSlot    string    `json:"pendingSlot,omitempty"`
	ActiveVersion  string    `json:"activeVersion"`
	PendingVersion string    `json:"pendingVersion,omitempty"`
	BootAttempts   int       `json:"bootAttempts"`
	LastError      string    `json:"lastError,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Manager struct {
	Root string
	mu   sync.Mutex
}

func CanonicalManifest(manifest Manifest) ([]byte, error) {
	if err := ValidateManifest(manifest); err != nil {
		return nil, err
	}
	return json.Marshal(manifest)
}

func ValidateManifest(manifest Manifest) error {
	if manifest.FormatVersion != ManifestFormatVersion {
		return fmt.Errorf("unsupported update manifest format %d", manifest.FormatVersion)
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return errors.New("update manifest version is required")
	}
	if len(manifest.PackageSHA256) != sha256.Size*2 {
		return errors.New("update packageSha256 must be a SHA-256 hex digest")
	}
	if _, err := hex.DecodeString(manifest.PackageSHA256); err != nil {
		return errors.New("update packageSha256 must be a SHA-256 hex digest")
	}
	if manifest.PackageSize < 1 {
		return errors.New("update packageSize must be positive")
	}
	if manifest.PublishedAt.IsZero() {
		return errors.New("update publishedAt is required")
	}
	return nil
}

func VerifyManifest(publicKey ed25519.PublicKey, manifest Manifest, signature []byte) error {
	encoded, err := CanonicalManifest(manifest)
	if err != nil {
		return err
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return errors.New("update verification key is invalid")
	}
	if len(signature) != ed25519.SignatureSize || !ed25519.Verify(publicKey, encoded, signature) {
		return errors.New("update manifest signature is invalid")
	}
	return nil
}

func ParseSignature(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("update signature is required")
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) != ed25519.SignatureSize {
		return nil, errors.New("update signature is not valid base64")
	}
	return decoded, nil
}

func ParsePublicKey(value string) (ed25519.PublicKey, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("update verification key is not configured")
	}
	if decoded, err := hex.DecodeString(value); err == nil && len(decoded) == ed25519.PublicKeySize {
		return ed25519.PublicKey(decoded), nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, errors.New("update verification key is invalid")
	}
	return ed25519.PublicKey(decoded), nil
}

func VerifyPackage(path string, manifest Manifest) error {
	if err := ValidateManifest(manifest); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open update package: %w", err)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat update package: %w", err)
	}
	if stat.Size() != manifest.PackageSize {
		return fmt.Errorf("update package size mismatch: expected %d, got %d", manifest.PackageSize, stat.Size())
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash update package: %w", err)
	}
	if !bytes.Equal(hash.Sum(nil), mustDecodeDigest(manifest.PackageSHA256)) {
		return errors.New("update package checksum mismatch")
	}
	return nil
}

func (m *Manager) Load() (SlotState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadLocked()
}

func (m *Manager) StageAndActivate(packagePath string, manifest Manifest, signature []byte, publicKey ed25519.PublicKey) (SlotState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := VerifyManifest(publicKey, manifest, signature); err != nil {
		return SlotState{}, err
	}
	if err := VerifyPackage(packagePath, manifest); err != nil {
		return SlotState{}, err
	}
	if err := os.MkdirAll(m.Root, 0o750); err != nil {
		return SlotState{}, fmt.Errorf("create update root: %w", err)
	}
	state, err := m.loadLocked()
	if err != nil {
		return SlotState{}, err
	}
	if state.ActiveSlot == "" {
		state.ActiveSlot = "a"
	}
	inactive := "b"
	if state.ActiveSlot == "b" {
		inactive = "a"
	}
	slotDir := filepath.Join(m.Root, "slot-"+inactive)
	if err := os.RemoveAll(slotDir); err != nil {
		return SlotState{}, fmt.Errorf("clear inactive update slot: %w", err)
	}
	if err := os.MkdirAll(slotDir, 0o750); err != nil {
		return SlotState{}, fmt.Errorf("create inactive update slot: %w", err)
	}
	packageTarget := filepath.Join(slotDir, "package")
	if err := copyFile(packagePath, packageTarget); err != nil {
		return SlotState{}, fmt.Errorf("stage update package: %w", err)
	}
	encoded, _ := json.Marshal(manifest)
	if err := atomicWrite(filepath.Join(slotDir, "manifest.json"), encoded, 0o640); err != nil {
		return SlotState{}, fmt.Errorf("write staged manifest: %w", err)
	}
	state.PreviousSlot = state.ActiveSlot
	state.PendingSlot = inactive
	state.PendingVersion = manifest.Version
	state.BootAttempts = 0
	state.LastError = ""
	state.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(state); err != nil {
		return SlotState{}, err
	}
	return state, nil
}

func (m *Manager) MarkHealthy(version string) (SlotState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.loadLocked()
	if err != nil {
		return SlotState{}, err
	}
	if state.PendingSlot == "" || (version != "" && version != state.PendingVersion) {
		return SlotState{}, errors.New("no matching pending update is awaiting health confirmation")
	}
	state.ActiveSlot = state.PendingSlot
	state.ActiveVersion = state.PendingVersion
	state.PendingSlot = ""
	state.PendingVersion = ""
	state.BootAttempts = 0
	state.LastError = ""
	state.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(state); err != nil {
		return SlotState{}, err
	}
	return state, nil
}

// RecordBoot records a boot of the pending candidate. The bootloader or
// service supervisor calls this only after starting the pending version. A
// candidate that does not confirm health within maxAttempts is failed closed:
// the old active slot remains selected and the pending slot is cleared.
func (m *Manager) RecordBoot(version string, maxAttempts int) (SlotState, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.loadLocked()
	if err != nil {
		return SlotState{}, false, err
	}
	if state.PendingSlot == "" || state.PendingVersion == "" || strings.TrimSpace(version) != state.PendingVersion {
		return state, false, nil
	}
	if maxAttempts < 1 {
		maxAttempts = DefaultMaxBootAttempts
	}
	state.BootAttempts++
	if state.BootAttempts >= maxAttempts {
		state.PendingSlot = ""
		state.PendingVersion = ""
		state.BootAttempts = 0
		state.LastError = fmt.Sprintf("automatic rollback after %d failed boot attempts", maxAttempts)
		state.UpdatedAt = time.Now().UTC()
		if err := m.saveLocked(state); err != nil {
			return SlotState{}, false, err
		}
		return state, true, nil
	}
	state.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(state); err != nil {
		return SlotState{}, false, err
	}
	return state, false, nil
}

func (m *Manager) Rollback(reason string) (SlotState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.loadLocked()
	if err != nil {
		return SlotState{}, err
	}
	if state.PreviousSlot == "" {
		return SlotState{}, errors.New("no previous update slot is available")
	}
	state.ActiveSlot, state.PreviousSlot = state.PreviousSlot, state.ActiveSlot
	state.PendingSlot = ""
	state.PendingVersion = ""
	state.BootAttempts = 0
	state.LastError = strings.TrimSpace(reason)
	state.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(state); err != nil {
		return SlotState{}, err
	}
	return state, nil
}

func (m *Manager) loadLocked() (SlotState, error) {
	data, err := os.ReadFile(filepath.Join(m.Root, "state.json"))
	if os.IsNotExist(err) {
		return SlotState{ActiveSlot: "a", UpdatedAt: time.Now().UTC()}, nil
	}
	if err != nil {
		return SlotState{}, fmt.Errorf("read update state: %w", err)
	}
	var state SlotState
	if err := json.Unmarshal(data, &state); err != nil {
		return SlotState{}, fmt.Errorf("parse update state: %w", err)
	}
	if state.ActiveSlot != "a" && state.ActiveSlot != "b" {
		return SlotState{}, errors.New("update state has an invalid active slot")
	}
	return state, nil
}

func (m *Manager) saveLocked(state SlotState) error {
	encoded, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode update state: %w", err)
	}
	return atomicWrite(filepath.Join(m.Root, "state.json"), encoded, 0o600)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".lumonas-update-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
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
	return os.Rename(temporaryPath, path)
}

func copyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func mustDecodeDigest(value string) []byte {
	decoded, _ := hex.DecodeString(value)
	return decoded
}
