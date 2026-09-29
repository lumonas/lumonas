package backup

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var sparseMagic = [8]byte{'L', 'U', 'M', 'O', 'S', 'P', '1', 0}

const maxSparseLogicalSize uint64 = 16 << 40

// PackSparseFile stores only data extents in a portable stream. It avoids
// sending qcow2 filesystem holes as network zeros, while preserving the exact
// image bytes for qemu-img verification after unpacking.
func PackSparseFile(source, target string) (checksum string, bytes int64, err error) {
	input, err := os.Open(source)
	if err != nil {
		return "", 0, err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || uint64(info.Size()) > maxSparseLogicalSize {
		return "", 0, errors.New("sparse source must be a regular file within the supported size limit")
	}
	output, err := os.CreateTemp(filepath.Dir(target), ".sparse-pack-*")
	if err != nil {
		return "", 0, err
	}
	temporary := output.Name()
	defer os.Remove(temporary)
	if err := output.Chmod(0600); err != nil {
		_ = output.Close()
		return "", 0, err
	}
	digest := sha256.New()
	writer := &countingHashWriter{writer: io.MultiWriter(output, digest)}
	if _, err := writer.Write(sparseMagic[:]); err != nil {
		_ = output.Close()
		return "", 0, err
	}
	if err := binary.Write(writer, binary.BigEndian, uint64(info.Size())); err != nil {
		_ = output.Close()
		return "", 0, err
	}
	rangeErr := walkSparseData(input, uint64(info.Size()), func(offset, length uint64) error {
		if length == 0 || offset > uint64(info.Size()) || length > uint64(info.Size())-offset {
			return errors.New("filesystem returned an invalid sparse data range")
		}
		if err := binary.Write(writer, binary.BigEndian, offset); err != nil {
			return err
		}
		if err := binary.Write(writer, binary.BigEndian, length); err != nil {
			return err
		}
		_, err := io.CopyN(writer, io.NewSectionReader(input, int64(offset), int64(length)), int64(length))
		return err
	})
	if rangeErr != nil {
		_ = output.Close()
		return "", 0, rangeErr
	}
	if err := binary.Write(writer, binary.BigEndian, uint64(0)); err != nil {
		_ = output.Close()
		return "", 0, err
	}
	if err := binary.Write(writer, binary.BigEndian, uint64(0)); err != nil {
		_ = output.Close()
		return "", 0, err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return "", 0, err
	}
	if err := output.Close(); err != nil {
		return "", 0, err
	}
	if err := os.Rename(temporary, target); err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), writer.count, nil
}

type countingHashWriter struct {
	writer io.Writer
	count  int64
}

func (w *countingHashWriter) Write(data []byte) (int, error) {
	n, err := w.writer.Write(data)
	w.count += int64(n)
	return n, err
}

// UnpackSparseFile validates every offset and length before writing and
// recreates holes with seeks instead of allocating their zero bytes.
func UnpackSparseFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	var magic [8]byte
	if _, err := io.ReadFull(input, magic[:]); err != nil || magic != sparseMagic {
		return errors.New("sparse backup format is invalid")
	}
	var logicalSize uint64
	if err := binary.Read(input, binary.BigEndian, &logicalSize); err != nil || logicalSize > maxSparseLogicalSize {
		return errors.New("sparse backup logical size is invalid")
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		_ = output.Close()
		if cleanup {
			_ = os.Remove(target)
		}
	}()
	var previousEnd uint64
	for {
		var offset, length uint64
		if err := binary.Read(input, binary.BigEndian, &offset); err != nil {
			return errors.New("sparse backup extent list is truncated")
		}
		if err := binary.Read(input, binary.BigEndian, &length); err != nil {
			return errors.New("sparse backup extent list is truncated")
		}
		if offset == 0 && length == 0 {
			break
		}
		if length == 0 || offset < previousEnd || offset > logicalSize || length > logicalSize-offset {
			return errors.New("sparse backup contains an invalid or overlapping extent")
		}
		if _, err := output.Seek(int64(offset), io.SeekStart); err != nil {
			return err
		}
		if _, err := io.CopyN(output, input, int64(length)); err != nil {
			return errors.New("sparse backup extent data is truncated")
		}
		previousEnd = offset + length
	}
	var trailing [1]byte
	if count, err := input.Read(trailing[:]); count != 0 || err != io.EOF {
		return errors.New("sparse backup contains trailing data")
	}
	if err := output.Truncate(int64(logicalSize)); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	cleanup = false
	return nil
}
