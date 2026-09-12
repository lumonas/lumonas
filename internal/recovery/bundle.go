package recovery

import (
	"archive/zip"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const FormatVersion = 1

type Manifest struct {
	FormatVersion int               `json:"formatVersion"`
	ConfigSchema  int               `json:"configSchema"`
	MyNASVersion  string            `json:"mynasVersion"`
	NASUUID       string            `json:"nasUuid"`
	Generation    int64             `json:"generation"`
	CreatedAt     time.Time         `json:"createdAt"`
	DiskIDs       []string          `json:"diskIds"`
	Checksums     map[string]string `json:"checksums"`
}

type Input struct {
	Manifest      Manifest
	DesiredState  []byte
	Database      []byte
	Compose       map[string][]byte
	Files         map[string][]byte
	EncryptedData []byte
}

type RestorePlan struct {
	Manifest         Manifest `json:"manifest"`
	Files            []string `json:"files"`
	Verified         bool     `json:"verified"`
	EncryptedSecrets bool     `json:"encryptedSecrets"`
	Warnings         []string `json:"warnings,omitempty"`
}

func Create(input Input, key []byte) ([]byte, error) {
	if input.Manifest.FormatVersion == 0 {
		input.Manifest.FormatVersion = FormatVersion
	}
	if input.Manifest.CreatedAt.IsZero() {
		input.Manifest.CreatedAt = time.Now().UTC()
	}
	files := map[string][]byte{"manifest.json": nil, "desired-state.json": input.DesiredState, "mynas.db": input.Database}
	for name, content := range input.Compose {
		if name == "" || content == nil {
			continue
		}
		files["docker/stacks/"+name] = content
	}
	for name, content := range input.Files {
		if !safeName(name) || content == nil {
			return nil, fmt.Errorf("invalid recovery file name %q", name)
		}
		files[name] = content
	}
	if len(input.EncryptedData) > 0 {
		encrypted, err := encrypt(input.EncryptedData, key)
		if err != nil {
			return nil, err
		}
		files["encrypted-secrets.bin"] = encrypted
	}
	input.Manifest.Checksums = map[string]string{}
	for name, content := range files {
		if name == "manifest.json" {
			continue
		}
		digest := sha256.Sum256(content)
		input.Manifest.Checksums[name] = hex.EncodeToString(digest[:])
	}
	manifest, err := json.MarshalIndent(input.Manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	files["manifest.json"] = manifest
	checksumLines := make([]string, 0, len(input.Manifest.Checksums))
	names := make([]string, 0, len(input.Manifest.Checksums))
	for name := range input.Manifest.Checksums {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		checksumLines = append(checksumLines, input.Manifest.Checksums[name]+"  "+name)
	}
	files["checksums.sha256"] = []byte(strings.Join(checksumLines, "\n") + "\n")
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	names = names[:0]
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	names = append(names, "checksums.sha256")
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		entry, err := writer.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(files[name]); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func Verify(bundle, key []byte) (Manifest, error) {
	reader, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		return Manifest{}, err
	}
	files := map[string][]byte{}
	for _, file := range reader.File {
		if !safeName(file.Name) && file.Name != "manifest.json" && file.Name != "checksums.sha256" {
			return Manifest{}, fmt.Errorf("unsafe bundle entry %q", file.Name)
		}
		handle, err := file.Open()
		if err != nil {
			return Manifest{}, err
		}
		data, err := io.ReadAll(handle)
		handle.Close()
		if err != nil {
			return Manifest{}, err
		}
		files[file.Name] = data
	}
	var manifest Manifest
	raw, ok := files["manifest.json"]
	if !ok || json.Unmarshal(raw, &manifest) != nil {
		return Manifest{}, errors.New("manifest is missing or invalid")
	}
	if manifest.FormatVersion != FormatVersion {
		return Manifest{}, fmt.Errorf("unsupported recovery bundle version %d", manifest.FormatVersion)
	}
	for name, expected := range manifest.Checksums {
		content, ok := files[name]
		if !ok {
			return Manifest{}, fmt.Errorf("bundle entry %q is missing", name)
		}
		digest := sha256.Sum256(content)
		if hex.EncodeToString(digest[:]) != expected {
			return Manifest{}, fmt.Errorf("checksum mismatch for %q", name)
		}
	}
	if encrypted, ok := files["encrypted-secrets.bin"]; ok {
		if _, err := decrypt(encrypted, key); err != nil {
			return Manifest{}, fmt.Errorf("encrypted data verification failed: %w", err)
		}
	}
	return manifest, nil
}

func Plan(bundle, key []byte) (RestorePlan, error) {
	manifest, err := Verify(bundle, key)
	if err != nil {
		return RestorePlan{}, err
	}
	reader, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		return RestorePlan{}, err
	}
	plan := RestorePlan{Manifest: manifest, Verified: true, Files: make([]string, 0, len(reader.File))}
	for _, file := range reader.File {
		plan.Files = append(plan.Files, file.Name)
		if file.Name == "encrypted-secrets.bin" {
			plan.EncryptedSecrets = true
		}
	}
	sort.Strings(plan.Files)
	if !contains(plan.Files, "desired-state.json") || !contains(plan.Files, "mynas.db") {
		plan.Warnings = append(plan.Warnings, "bundle is missing desired state or database")
	}
	if !plan.EncryptedSecrets {
		plan.Warnings = append(plan.Warnings, "bundle contains no encrypted secret payload")
	}
	return plan, nil
}

func DecryptSecrets(bundle, key []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		return nil, err
	}
	for _, file := range reader.File {
		if file.Name != "encrypted-secrets.bin" {
			continue
		}
		handle, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(handle)
		handle.Close()
		if err != nil {
			return nil, err
		}
		return decrypt(data, key)
	}
	return nil, nil
}

func encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(normalizeKey(key))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}
func decrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(normalizeKey(key))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("encrypted payload is too short")
	}
	return gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
}
func normalizeKey(key []byte) []byte { digest := sha256.Sum256(key); return digest[:] }

func safeName(name string) bool {
	return name != "" && !strings.HasPrefix(name, "/") && !strings.Contains(name, "..")
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
