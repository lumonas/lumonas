package recovery

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestCreateRejectsRecoveryBundleShapeOverflow(t *testing.T) {
	files := make(map[string][]byte, MaxBundleEntries)
	for index := 0; index < MaxBundleEntries; index++ {
		files[fmt.Sprintf("config/entry-%04d.json", index)] = []byte("{}")
	}
	if _, err := Create(Input{Manifest: Manifest{NASUUID: "nas-1"}, DesiredState: []byte("{}"), Database: []byte("sqlite"), Files: files}, []byte("key")); err == nil || !strings.Contains(err.Error(), "too many entries") {
		t.Fatalf("expected bundle entry limit, got %v", err)
	}

	longName := "config/" + strings.Repeat("x", MaxBundleEntryNameBytes)
	if _, err := Create(Input{Manifest: Manifest{NASUUID: "nas-1"}, DesiredState: []byte("{}"), Database: []byte("sqlite"), Files: map[string][]byte{longName: []byte("{}")}}, []byte("key")); err == nil || !strings.Contains(err.Error(), "entry name") {
		t.Fatalf("expected bundle entry-name limit, got %v", err)
	}
}

func TestReadBundleFilesRejectsTooManyZipEntries(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for index := 0; index <= MaxBundleEntries; index++ {
		entry, err := writer.Create(fmt.Sprintf("config/entry-%04d.json", index))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("{}")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := readBundleFiles(buffer.Bytes()); err == nil || !strings.Contains(err.Error(), "too many entries") {
		t.Fatalf("expected ZIP entry limit, got %v", err)
	}
}
