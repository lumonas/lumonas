package backup

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestSparsePackRoundTripsHolesWithoutTransportingThem(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.qcow2")
	packed := filepath.Join(root, "source.qcow2.sparse")
	restored := filepath.Join(root, "restored.qcow2")
	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(64 << 20); err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte("qcow2 header"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte("guest data at tail"), (64<<20)-32); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	checksum, packedBytes, err := PackSparseFile(source, packed)
	if err != nil {
		t.Fatal(err)
	}
	if packedBytes >= 1<<20 {
		t.Fatalf("sparse archive retained too many zero bytes: %d", packedBytes)
	}
	if err := UnpackSparseFile(packed, restored); err != nil {
		t.Fatal(err)
	}
	originalDigest, originalBytes, err := SHA256File(source)
	if err != nil {
		t.Fatal(err)
	}
	restoredDigest, restoredBytes, err := SHA256File(restored)
	if err != nil {
		t.Fatal(err)
	}
	if originalBytes != restoredBytes || originalDigest != restoredDigest {
		t.Fatal("unpacked sparse file differs from its source")
	}
	packedDigest, actualPackedBytes, err := SHA256File(packed)
	if err != nil || packedDigest != checksum || actualPackedBytes != packedBytes {
		t.Fatal("sparse pack did not return integrity metadata")
	}
}

func TestSparseUnpackRejectsInvalidExtentsAndTrailingBytes(t *testing.T) {
	for _, test := range []struct {
		name string
		body func(*os.File) error
	}{
		{name: "out of range", body: func(file *os.File) error {
			if _, err := file.Write(sparseMagic[:]); err != nil {
				return err
			}
			if err := binary.Write(file, binary.BigEndian, uint64(8)); err != nil {
				return err
			}
			if err := binary.Write(file, binary.BigEndian, uint64(7)); err != nil {
				return err
			}
			if err := binary.Write(file, binary.BigEndian, uint64(2)); err != nil {
				return err
			}
			_, err := file.Write([]byte{0})
			return err
		}},
		{name: "trailing bytes", body: func(file *os.File) error {
			if _, err := file.Write(sparseMagic[:]); err != nil {
				return err
			}
			if err := binary.Write(file, binary.BigEndian, uint64(0)); err != nil {
				return err
			}
			if err := binary.Write(file, binary.BigEndian, uint64(0)); err != nil {
				return err
			}
			if err := binary.Write(file, binary.BigEndian, uint64(0)); err != nil {
				return err
			}
			_, err := file.Write([]byte{1})
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "invalid.sparse")
			file, err := os.Create(source)
			if err != nil {
				t.Fatal(err)
			}
			if err := test.body(file); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if err := UnpackSparseFile(source, filepath.Join(root, "output.qcow2")); err == nil {
				t.Fatal("expected malformed sparse data to be rejected")
			}
		})
	}
}
