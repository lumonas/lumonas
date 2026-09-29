package workstationbackup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	archiveMagic  = "LWB1"
	archiveChunk  = 4 << 20
	archiveTagLen = 16
)

func createEncryptedArchive(source, output string, masterKey []byte) error {
	return createEncryptedArchiveSelected(source, output, masterKey, Selection{})
}

func createEncryptedArchiveSelected(source, output string, masterKey []byte, selection Selection) error {
	if len(masterKey) != 32 {
		return errors.New("backup encryption key must be 32 bytes")
	}
	if err := selection.Validate(); err != nil {
		return err
	}
	root, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("backup source must be an accessible directory")
	}
	encrypted, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	reader, writer := io.Pipe()
	writeDone := make(chan error, 1)
	go func() {
		writeErr := writeTarGzip(root, writer, selection)
		_ = writer.CloseWithError(writeErr)
		writeDone <- writeErr
	}()
	encryptErr := encryptChunks(reader, encrypted, masterKey)
	_ = reader.Close()
	archiveErr := <-writeDone
	if encryptErr != nil {
		_ = encrypted.Close()
		_ = os.Remove(output)
		return encryptErr
	}
	if archiveErr != nil {
		_ = encrypted.Close()
		_ = os.Remove(output)
		return archiveErr
	}
	if err := encrypted.Sync(); err != nil {
		_ = encrypted.Close()
		_ = os.Remove(output)
		return err
	}
	return encrypted.Close()
}

func writeTarGzip(root string, output io.Writer, selection Selection) error {
	compressed := gzip.NewWriter(output)
	archive := tar.NewWriter(compressed)
	err := filepath.WalkDir(root, func(pathname string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if pathname == root {
			return nil
		}
		relative, err := filepath.Rel(root, pathname)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if selection.excluded(relative) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !selection.includes(relative) {
			if entry.IsDir() && selection.mayContainIncluded(relative) {
				return nil
			}
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := os.Lstat(pathname)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil
		}
		tarName := relative
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = tarName
		header.Uid, header.Gid, header.Uname, header.Gname = 0, 0, "", ""
		header.Mode &= 0o777
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(pathname)
		if err != nil {
			return err
		}
		before, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return err
		}
		if !os.SameFile(info, before) || before.Size() != info.Size() || !before.ModTime().Equal(info.ModTime()) {
			_ = file.Close()
			return fmt.Errorf("source changed while backing up: %s", relative)
		}
		_, copyErr := io.CopyN(archive, file, info.Size())
		after, statErr := file.Stat()
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if statErr != nil {
			return statErr
		}
		if closeErr != nil {
			return closeErr
		}
		if after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
			return fmt.Errorf("source changed while backing up: %s", relative)
		}
		return nil
	})
	closeTarErr := archive.Close()
	closeGzipErr := compressed.Close()
	if err != nil {
		return err
	}
	if closeTarErr != nil {
		return closeTarErr
	}
	return closeGzipErr
}

func deriveArchiveKey(masterKey, salt []byte) []byte {
	mac := hmac.New(sha256.New, masterKey)
	_, _ = mac.Write([]byte("LumoNAS workstation backup v1\x00"))
	_, _ = mac.Write(salt)
	return mac.Sum(nil)
}

func archiveNonce(prefix []byte, counter uint64) []byte {
	nonce := make([]byte, 12)
	copy(nonce[:4], prefix)
	binary.BigEndian.PutUint64(nonce[4:], counter)
	return nonce
}

func chunkAAD(salt []byte, counter uint64) []byte {
	aad := make([]byte, 4+len(salt)+8)
	copy(aad, archiveMagic)
	copy(aad[4:], salt)
	binary.BigEndian.PutUint64(aad[4+len(salt):], counter)
	return aad
}

func encryptChunks(input io.Reader, output io.Writer, masterKey []byte) error {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	block, err := aes.NewCipher(deriveArchiveKey(masterKey, salt))
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(output, archiveMagic); err != nil {
		return err
	}
	if _, err := output.Write(salt); err != nil {
		return err
	}
	buffer := make([]byte, archiveChunk)
	var counter uint64
	for {
		count, readErr := io.ReadFull(input, buffer)
		if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
			return readErr
		}
		if count > 0 {
			ciphertext := aead.Seal(nil, archiveNonce(salt[:4], counter), buffer[:count], chunkAAD(salt, counter))
			if err := binary.Write(output, binary.BigEndian, uint32(len(ciphertext))); err != nil {
				return err
			}
			if _, err := output.Write(ciphertext); err != nil {
				return err
			}
			counter++
		}
		if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
			break
		}
	}
	return nil
}

func decryptChunks(input io.Reader, output io.Writer, masterKey []byte) error {
	magic := make([]byte, len(archiveMagic))
	if _, err := io.ReadFull(input, magic); err != nil || string(magic) != archiveMagic {
		return errors.New("not a LumoNAS workstation backup archive")
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(input, salt); err != nil {
		return errors.New("workstation backup archive header is incomplete")
	}
	block, err := aes.NewCipher(deriveArchiveKey(masterKey, salt))
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	var counter uint64
	for {
		var length uint32
		if err := binary.Read(input, binary.BigEndian, &length); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return errors.New("workstation backup archive is truncated")
		}
		if length < archiveTagLen || length > archiveChunk+archiveTagLen {
			return errors.New("workstation backup archive chunk size is invalid")
		}
		ciphertext := make([]byte, int(length))
		if _, err := io.ReadFull(input, ciphertext); err != nil {
			return errors.New("workstation backup archive chunk is truncated")
		}
		plaintext, err := aead.Open(nil, archiveNonce(salt[:4], counter), ciphertext, chunkAAD(salt, counter))
		if err != nil {
			return errors.New("workstation backup archive authentication failed; check the key and file integrity")
		}
		if _, err := output.Write(plaintext); err != nil {
			return err
		}
		counter++
	}
}

func extractArchive(encryptedPath, destination string, masterKey []byte, overwrite bool) error {
	if len(masterKey) != 32 {
		return errors.New("backup encryption key must be 32 bytes")
	}
	input, err := os.Open(encryptedPath)
	if err != nil {
		return err
	}
	defer input.Close()
	plain, err := os.CreateTemp("", "lumonas-restore-*.tar.gz")
	if err != nil {
		return err
	}
	plainPath := plain.Name()
	defer os.Remove(plainPath)
	if err := decryptChunks(input, plain, masterKey); err != nil {
		_ = plain.Close()
		return err
	}
	if err := plain.Close(); err != nil {
		return err
	}
	compressed, err := os.Open(plainPath)
	if err != nil {
		return err
	}
	defer compressed.Close()
	reader, err := gzip.NewReader(compressed)
	if err != nil {
		return errors.New("decrypted archive is not a valid gzip stream")
	}
	defer reader.Close()
	root, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	archive := tar.NewReader(reader)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		target, err := safeArchiveTarget(root, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := ensureSafeDirectory(root, target); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 {
				return errors.New("archive contains an invalid file size")
			}
			if err := ensureSafeDirectory(root, filepath.Dir(target)); err != nil {
				return err
			}
			if !overwrite {
				if _, err := os.Lstat(target); err == nil {
					return fmt.Errorf("restore would overwrite existing file: %s", header.Name)
				} else if !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			mode := os.FileMode(header.Mode) & 0o777
			temporary, err := os.CreateTemp(filepath.Dir(target), ".lumonas-restore-*")
			if err != nil {
				return err
			}
			written, copyErr := io.CopyN(temporary, archive, header.Size)
			if copyErr == nil && written != header.Size {
				copyErr = io.ErrUnexpectedEOF
			}
			if copyErr == nil {
				copyErr = temporary.Chmod(mode)
			}
			if copyErr == nil {
				copyErr = temporary.Sync()
			}
			temporaryPath := temporary.Name()
			closeErr := temporary.Close()
			if copyErr != nil {
				_ = os.Remove(temporaryPath)
				return copyErr
			}
			if closeErr != nil {
				_ = os.Remove(temporaryPath)
				return closeErr
			}
			if overwrite && runtime.GOOS == "windows" {
				if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
					_ = os.Remove(temporaryPath)
					return err
				}
			}
			if err := os.Rename(temporaryPath, target); err != nil {
				_ = os.Remove(temporaryPath)
				return err
			}
			if header.ModTime.After(time.Time{}) {
				_ = os.Chtimes(target, header.ModTime, header.ModTime)
			}
		default:
			return fmt.Errorf("unsupported entry type in workstation backup: %s", header.Name)
		}
	}
	return nil
}

func safeArchiveTarget(root, name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || strings.Contains(name, ":") || strings.HasPrefix(name, "/") {
		return "", errors.New("archive contains an unsafe path")
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("archive contains an unsafe path")
	}
	target := filepath.Join(root, filepath.FromSlash(clean))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("archive path escapes the restore directory")
	}
	return target, nil
}

func ensureSafeDirectory(root, target string) error {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("archive path escapes the restore directory")
	}
	if relative == "." {
		return nil
	}
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("restore path contains a symlink or non-directory parent")
		}
	}
	return nil
}

func verifyKey(key []byte) error {
	if len(key) != 32 {
		return errors.New("backup encryption key must be 32 bytes")
	}
	return nil
}

func encryptedFileSHA256(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), size, nil
}
