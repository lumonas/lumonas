package recovery

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
)

const FormatVersion = 1

type Manifest struct {
	FormatVersion  int               `json:"formatVersion"`
	ConfigSchema   int               `json:"configSchema"`
	LumoNASVersion string            `json:"lumonasVersion"`
	NASUUID        string            `json:"nasUuid"`
	Generation     int64             `json:"generation"`
	CreatedAt      time.Time         `json:"createdAt"`
	DiskIDs        []string          `json:"diskIds"`
	Checksums      map[string]string `json:"checksums"`
}

type Input struct {
	Manifest      Manifest
	DesiredState  []byte
	Database      []byte
	Compose       map[string][]byte
	Files         map[string][]byte
	Appdata       []AppdataPayload
	EncryptedData []byte
}

type RestorePlan struct {
	Manifest          Manifest        `json:"manifest"`
	Files             []string        `json:"files"`
	Verified          bool            `json:"verified"`
	DatabaseValid     bool            `json:"databaseValid"`
	DesiredStateValid bool            `json:"desiredStateValid"`
	ComposeValid      bool            `json:"composeValid"`
	EncryptedSecrets  bool            `json:"encryptedSecrets"`
	Appdata           []AppdataRecord `json:"appdata,omitempty"`
	Warnings          []string        `json:"warnings,omitempty"`
}

type StageResult struct {
	Manifest  Manifest `json:"manifest"`
	Directory string   `json:"directory"`
	Files     []string `json:"files"`
	Verified  bool     `json:"verified"`
}

func Create(input Input, key []byte) ([]byte, error) {
	if input.Manifest.FormatVersion == 0 {
		input.Manifest.FormatVersion = FormatVersion
	}
	if input.Manifest.CreatedAt.IsZero() {
		input.Manifest.CreatedAt = time.Now().UTC()
	}
	files := map[string][]byte{"manifest.json": nil, "desired-state.json": input.DesiredState, "lumonas.db": input.Database}
	for name, content := range input.Compose {
		if name == "" || content == nil {
			continue
		}
		stackPath := "docker/stacks/" + name
		if !allowedPayloadEntry(stackPath) {
			return nil, fmt.Errorf("invalid recovery file name %q", stackPath)
		}
		files[stackPath] = content
	}
	for name, content := range input.Files {
		if !allowedPayloadEntry(name) || content == nil {
			return nil, fmt.Errorf("invalid recovery file name %q", name)
		}
		files[name] = content
	}
	appdataRecords := make([]AppdataRecord, 0, len(input.Appdata))
	for _, payload := range input.Appdata {
		if err := validateAppdataPayload(payload); err != nil {
			return nil, err
		}
		archivePath := appdataArchivePath(payload.Stack, payload.ContainerPath)
		if _, exists := files[archivePath]; exists {
			return nil, fmt.Errorf("duplicate appdata payload %q", archivePath)
		}
		files[archivePath] = payload.Archive
		appdataRecords = append(appdataRecords, AppdataRecord{Stack: payload.Stack, ContainerPath: payload.ContainerPath, HostPath: filepath.Clean(payload.HostPath), ArchivePath: archivePath, ArchiveBytes: int64(len(payload.Archive))})
	}
	if len(appdataRecords) > 0 {
		manifestData, err := json.Marshal(appdataRecords)
		if err != nil {
			return nil, err
		}
		files["docker/appdata/manifest.json"] = manifestData
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
	files, err := readBundleFiles(bundle)
	if err != nil {
		return Manifest{}, err
	}
	return verifyFiles(files, key)
}

func verifyFiles(files map[string][]byte, key []byte) (Manifest, error) {
	for _, name := range []string{"manifest.json", "checksums.sha256", "desired-state.json", "lumonas.db"} {
		if _, ok := files[name]; !ok {
			return Manifest{}, fmt.Errorf("bundle entry %q is missing", name)
		}
	}
	var manifest Manifest
	raw, ok := files["manifest.json"]
	if !ok || json.Unmarshal(raw, &manifest) != nil {
		return Manifest{}, errors.New("manifest is missing or invalid")
	}
	if manifest.FormatVersion != FormatVersion {
		return Manifest{}, fmt.Errorf("unsupported recovery bundle version %d", manifest.FormatVersion)
	}
	if err := validateChecksumCoverage(files, manifest.Checksums); err != nil {
		return Manifest{}, err
	}
	if err := validateChecksumFile(files["checksums.sha256"], manifest.Checksums); err != nil {
		return Manifest{}, err
	}
	if encrypted, ok := files["encrypted-secrets.bin"]; ok {
		if _, err := decrypt(encrypted, key); err != nil {
			return Manifest{}, fmt.Errorf("encrypted data verification failed: %w", err)
		}
	}
	if raw, ok := files["docker/appdata/manifest.json"]; ok {
		if _, err := parseAppdataManifest(raw, files); err != nil {
			return Manifest{}, err
		}
	}
	return manifest, nil
}

func readBundleFiles(bundle []byte) (map[string][]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		return nil, err
	}
	files := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		if _, exists := files[file.Name]; exists {
			return nil, fmt.Errorf("duplicate bundle entry %q", file.Name)
		}
		if !allowedBundleEntry(file.Name) {
			return nil, fmt.Errorf("unsupported or unsafe bundle entry %q", file.Name)
		}
		handle, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(handle)
		closeErr := handle.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		files[file.Name] = data
	}
	return files, nil
}

func validateChecksumCoverage(files map[string][]byte, checksums map[string]string) error {
	if checksums == nil {
		return errors.New("manifest checksums are missing")
	}
	for name := range files {
		if name == "manifest.json" || name == "checksums.sha256" {
			continue
		}
		if _, ok := checksums[name]; !ok {
			return fmt.Errorf("checksum is missing for bundle entry %q", name)
		}
	}
	for name, expected := range checksums {
		if name == "manifest.json" || name == "checksums.sha256" {
			return fmt.Errorf("manifest checksum contains metadata entry %q", name)
		}
		content, ok := files[name]
		if !ok {
			return fmt.Errorf("bundle entry %q is missing", name)
		}
		digest := sha256.Sum256(content)
		if hex.EncodeToString(digest[:]) != expected {
			return fmt.Errorf("checksum mismatch for %q", name)
		}
	}
	return nil
}

func validateChecksumFile(raw []byte, manifestChecksums map[string]string) error {
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return errors.New("checksums.sha256 is empty")
	}
	seen := make(map[string]bool, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || len(parts[0]) != sha256.Size*2 {
			return fmt.Errorf("invalid checksums.sha256 entry %q", line)
		}
		if _, err := hex.DecodeString(parts[0]); err != nil {
			return fmt.Errorf("invalid checksum for %q", parts[1])
		}
		name := parts[1]
		if !allowedPayloadEntry(name) {
			return fmt.Errorf("unsupported or unsafe checksum entry %q", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate checksum entry %q", name)
		}
		seen[name] = true
		expected, ok := manifestChecksums[name]
		if !ok || expected != parts[0] {
			return fmt.Errorf("checksum manifest mismatch for %q", name)
		}
	}
	if len(seen) != len(manifestChecksums) {
		return errors.New("checksums.sha256 does not cover the manifest checksum set")
	}
	return nil
}

func Plan(bundle, key []byte) (RestorePlan, error) {
	manifest, err := Verify(bundle, key)
	if err != nil {
		return RestorePlan{}, err
	}
	files, err := readBundleFiles(bundle)
	if err != nil {
		return RestorePlan{}, err
	}
	reader, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		return RestorePlan{}, err
	}
	plan := RestorePlan{Manifest: manifest, Verified: true, Files: make([]string, 0, len(reader.File)), DatabaseValid: isSQLiteDatabase(files["lumonas.db"]), DesiredStateValid: json.Valid(files["desired-state.json"]), ComposeValid: true}
	if raw, ok := files["docker/appdata/manifest.json"]; ok {
		appdata, appdataErr := parseAppdataManifest(raw, files)
		if appdataErr != nil {
			return RestorePlan{}, appdataErr
		}
		plan.Appdata = appdata
	}
	for _, file := range reader.File {
		plan.Files = append(plan.Files, file.Name)
		if file.Name == "encrypted-secrets.bin" {
			plan.EncryptedSecrets = true
		}
	}
	sort.Strings(plan.Files)
	if !contains(plan.Files, "desired-state.json") || !contains(plan.Files, "lumonas.db") {
		plan.Warnings = append(plan.Warnings, "bundle is missing desired state or database")
	}
	if !plan.DatabaseValid {
		plan.Warnings = append(plan.Warnings, "database payload does not contain a valid SQLite header")
	}
	if !plan.DesiredStateValid {
		plan.Warnings = append(plan.Warnings, "desired state is not valid JSON")
	}
	for name, content := range files {
		if !strings.HasPrefix(name, "docker/stacks/") || !strings.HasSuffix(name, "/compose.yaml") {
			continue
		}
		if err := (dockerruntime.New("", nil)).ValidateCompose(context.Background(), string(content)); err != nil {
			plan.ComposeValid = false
			plan.Warnings = append(plan.Warnings, "invalid Docker Compose payload: "+name)
		}
	}
	for _, diskID := range manifest.DiskIDs {
		if strings.TrimSpace(diskID) == "" {
			plan.Warnings = append(plan.Warnings, "manifest contains an empty disk identity")
		}
	}
	if !plan.EncryptedSecrets {
		plan.Warnings = append(plan.Warnings, "bundle contains no encrypted secret payload")
	}
	return plan, nil
}

func isSQLiteDatabase(data []byte) bool {
	return bytes.HasPrefix(data, []byte("SQLite format 3\x00"))
}

func Stage(bundle, key []byte, destination string) (StageResult, error) {
	if strings.TrimSpace(destination) == "" {
		return StageResult{}, errors.New("staging destination is required")
	}
	plan, err := Plan(bundle, key)
	if err != nil {
		return StageResult{}, err
	}
	if !plan.DatabaseValid || !plan.DesiredStateValid {
		return StageResult{}, errors.New("restore payload validation failed")
	}
	files, err := readBundleFiles(bundle)
	if err != nil {
		return StageResult{}, err
	}
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return StageResult{}, err
	}
	directory, err := os.MkdirTemp(destination, ".restore-")
	if err != nil {
		return StageResult{}, err
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !allowedBundleEntry(name) {
			cleanup()
			return StageResult{}, fmt.Errorf("unsupported restore entry %q", name)
		}
		target := filepath.Join(directory, filepath.FromSlash(name))
		relative, err := filepath.Rel(directory, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			cleanup()
			return StageResult{}, fmt.Errorf("unsafe restore entry %q", name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			cleanup()
			return StageResult{}, err
		}
		if err := os.WriteFile(target, files[name], 0o600); err != nil {
			cleanup()
			return StageResult{}, err
		}
	}
	return StageResult{Manifest: plan.Manifest, Directory: directory, Files: names, Verified: true}, nil
}

func DecryptSecrets(bundle, key []byte) ([]byte, error) {
	files, err := readBundleFiles(bundle)
	if err != nil {
		return nil, err
	}
	if _, err := verifyFiles(files, key); err != nil {
		return nil, err
	}
	data, ok := files["encrypted-secrets.bin"]
	if !ok {
		return nil, nil
	}
	return decrypt(data, key)
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
	if name == "" || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") || strings.ContainsAny(name, "\\\x00") {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func allowedBundleEntry(name string) bool {
	return name == "manifest.json" || name == "checksums.sha256" || allowedPayloadEntry(name)
}

func allowedPayloadEntry(name string) bool {
	if !safeName(name) {
		return false
	}
	switch name {
	case "desired-state.json", "lumonas.db", "encrypted-secrets.bin":
		return true
	}
	for _, prefix := range []string{"docker/stacks/", "docker/appdata/", "config/", "storage/", "acl/", "certificates/", "encrypted-secrets/"} {
		if strings.HasPrefix(name, prefix) {
			return len(strings.TrimPrefix(name, prefix)) > 0
		}
	}
	return false
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
