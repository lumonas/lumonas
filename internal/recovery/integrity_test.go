package recovery

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"testing"
)

type testZipEntry struct {
	name string
	data []byte
}

func validBundle(t *testing.T, encrypted bool) []byte {
	t.Helper()
	input := Input{
		Manifest:     Manifest{LumoNASVersion: "test", NASUUID: "nas-1", Generation: 4},
		DesiredState: []byte(`{"hostname":"nas"}`),
		Database:     []byte("sqlite snapshot"),
		Compose:      map[string][]byte{"media/compose.yaml": []byte("services:\n  media:\n    image: example/media:latest\n")},
	}
	if encrypted {
		input.EncryptedData = []byte("secret payload")
	}
	bundle, err := Create(input, []byte("recovery-key"))
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func readTestZip(t *testing.T, bundle []byte) []testZipEntry {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		t.Fatal(err)
	}
	entries := make([]testZipEntry, 0, len(reader.File))
	for _, file := range reader.File {
		handle, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(handle)
		closeErr := handle.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		entries = append(entries, testZipEntry{name: file.Name, data: data})
	}
	return entries
}

func writeTestZip(t *testing.T, entries []testZipEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		file, err := writer.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func rewriteTestZip(t *testing.T, bundle []byte, mutate func([]testZipEntry) []testZipEntry) []byte {
	t.Helper()
	return writeTestZip(t, mutate(readTestZip(t, bundle)))
}

func replaceTestEntry(entries []testZipEntry, name string, data []byte) []testZipEntry {
	for index := range entries {
		if entries[index].name == name {
			entries[index].data = data
			return entries
		}
	}
	return append(entries, testZipEntry{name: name, data: data})
}

func removeTestEntry(entries []testZipEntry, name string) []testZipEntry {
	result := entries[:0]
	for _, entry := range entries {
		if entry.name != name {
			result = append(result, entry)
		}
	}
	return result
}

func manifestWithChecksum(t *testing.T, bundle []byte, name string, data []byte) []byte {
	t.Helper()
	entries := readTestZip(t, bundle)
	var manifest Manifest
	for _, entry := range entries {
		if entry.name == "manifest.json" {
			if err := json.Unmarshal(entry.data, &manifest); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	digest := sha256.Sum256(data)
	manifest.Checksums[name] = hex.EncodeToString(digest[:])
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	entries = replaceTestEntry(entries, name, data)
	entries = replaceTestEntry(entries, "manifest.json", manifestData)
	entries = replaceTestEntry(entries, "checksums.sha256", checksumFile(manifest.Checksums))
	return writeTestZip(t, entries)
}

func checksumFile(checksums map[string]string) []byte {
	names := make([]string, 0, len(checksums))
	for name := range checksums {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		lines = append(lines, checksums[name]+"  "+name)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func TestCompleteBundleWithComposeAndSecretsVerifies(t *testing.T) {
	bundle := validBundle(t, true)
	manifest, err := Verify(bundle, []byte("recovery-key"))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Generation != 4 || manifest.FormatVersion != FormatVersion {
		t.Fatalf("unexpected manifest %#v", manifest)
	}
	secret, err := DecryptSecrets(bundle, []byte("recovery-key"))
	if err != nil {
		t.Fatal(err)
	}
	if string(secret) != "secret payload" {
		t.Fatalf("unexpected secret %q", secret)
	}
}

func TestBundleIntegrityRejectsMalformedArchives(t *testing.T) {
	base := validBundle(t, true)
	tests := []struct {
		name    string
		wantErr string
		mutate  func([]testZipEntry) []testZipEntry
	}{
		{
			name:    "missing required entry",
			wantErr: `bundle entry "lumonas.db" is missing`,
			mutate:  func(entries []testZipEntry) []testZipEntry { return removeTestEntry(entries, "lumonas.db") },
		},
		{
			name:    "missing checksum entry",
			wantErr: "checksums.sha256 does not cover",
			mutate: func(entries []testZipEntry) []testZipEntry {
				for index := range entries {
					if entries[index].name == "checksums.sha256" {
						lines := strings.Split(string(entries[index].data), "\n")
						filtered := make([]string, 0, len(lines))
						for _, line := range lines {
							if !strings.HasSuffix(line, "  desired-state.json") {
								filtered = append(filtered, line)
							}
						}
						entries[index].data = []byte(strings.Join(filtered, "\n"))
					}
				}
				return entries
			},
		},
		{
			name:    "extra payload without checksum",
			wantErr: `checksum is missing for bundle entry "config/extra.json"`,
			mutate: func(entries []testZipEntry) []testZipEntry {
				return append(entries, testZipEntry{name: "config/extra.json", data: []byte("{}")})
			},
		},
		{
			name:    "manifest checksum points to missing entry",
			wantErr: `bundle entry "config/missing.json" is missing`,
			mutate: func(entries []testZipEntry) []testZipEntry {
				for index := range entries {
					if entries[index].name != "manifest.json" {
						continue
					}
					var manifest Manifest
					if err := json.Unmarshal(entries[index].data, &manifest); err != nil {
						t.Fatal(err)
					}
					manifest.Checksums["config/missing.json"] = strings.Repeat("0", sha256.Size*2)
					entries[index].data, _ = json.MarshalIndent(manifest, "", "  ")
				}
				return entries
			},
		},
		{
			name:    "duplicate checksum line",
			wantErr: `duplicate checksum entry "desired-state.json"`,
			mutate: func(entries []testZipEntry) []testZipEntry {
				for index := range entries {
					if entries[index].name == "checksums.sha256" {
						line := strings.Split(string(entries[index].data), "\n")[0]
						entries[index].data = append(entries[index].data, []byte(line+"\n")...)
						break
					}
				}
				return entries
			},
		},
		{
			name:    "malformed checksum line",
			wantErr: "invalid checksums.sha256 entry",
			mutate: func(entries []testZipEntry) []testZipEntry {
				return replaceTestEntry(entries, "checksums.sha256", []byte("not-a-checksum\n"))
			},
		},
		{
			name:    "duplicate ZIP entry",
			wantErr: `duplicate bundle entry "desired-state.json"`,
			mutate: func(entries []testZipEntry) []testZipEntry {
				for _, entry := range entries {
					if entry.name == "desired-state.json" {
						return append(entries, entry)
					}
				}
				return entries
			},
		},
		{
			name:    "path traversal",
			wantErr: `unsupported or unsafe bundle entry "docker/stacks/../../escape"`,
			mutate: func(entries []testZipEntry) []testZipEntry {
				return append(entries, testZipEntry{name: "docker/stacks/../../escape", data: []byte("x")})
			},
		},
		{
			name:    "absolute path",
			wantErr: `unsupported or unsafe bundle entry "/etc/passwd"`,
			mutate: func(entries []testZipEntry) []testZipEntry {
				return append(entries, testZipEntry{name: "/etc/passwd", data: []byte("x")})
			},
		},
		{
			name:    "backslash path",
			wantErr: `unsupported or unsafe bundle entry "docker\\stacks\\escape"`,
			mutate: func(entries []testZipEntry) []testZipEntry {
				return append(entries, testZipEntry{name: `docker\stacks\escape`, data: []byte("x")})
			},
		},
		{
			name:    "unknown path",
			wantErr: `unsupported or unsafe bundle entry "unexpected.txt"`,
			mutate: func(entries []testZipEntry) []testZipEntry {
				return append(entries, testZipEntry{name: "unexpected.txt", data: []byte("x")})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Verify(rewriteTestZip(t, base, test.mutate), []byte("recovery-key"))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected error containing %q, got %v", test.wantErr, err)
			}
		})
	}
}

func TestDecryptSecretsSharesStrictBundleValidation(t *testing.T) {
	bundle := validBundle(t, true)
	if _, err := DecryptSecrets(bundle, []byte("wrong-key")); err == nil {
		t.Fatal("wrong key unexpectedly decrypted bundle")
	}

	malformed := manifestWithChecksum(t, bundle, "encrypted-secrets.bin", []byte("too short"))
	if _, err := DecryptSecrets(malformed, []byte("recovery-key")); err == nil || !strings.Contains(err.Error(), "encrypted payload is too short") {
		t.Fatalf("expected malformed encrypted payload error, got %v", err)
	}
}

func TestBundleWithoutSecretsRemainsValid(t *testing.T) {
	bundle := validBundle(t, false)
	if _, err := Verify(bundle, []byte("recovery-key")); err != nil {
		t.Fatal(err)
	}
	secret, err := DecryptSecrets(bundle, []byte("recovery-key"))
	if err != nil {
		t.Fatal(err)
	}
	if secret != nil {
		t.Fatalf("expected no secret payload, got %q", secret)
	}
}

func TestCreateRejectsUnsupportedAndUnsafePayloadNames(t *testing.T) {
	for name, input := range map[string]Input{
		"unsupported file":    {Files: map[string][]byte{"unexpected.txt": []byte("x")}},
		"unsafe compose name": {Compose: map[string][]byte{"../escape": []byte("x")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Create(input, []byte("recovery-key")); err == nil {
				t.Fatal("expected invalid payload name to be rejected")
			}
		})
	}
}
