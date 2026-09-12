package files

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const trashDirectory = ".mynas-trash"

type Entry struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

type RecycleEntry struct {
	ID           string    `json:"id"`
	ShareID      string    `json:"shareId"`
	Name         string    `json:"name"`
	OriginalPath string    `json:"originalPath"`
	DeletedAt    time.Time `json:"deletedAt"`
	SizeBytes    int64     `json:"sizeBytes"`
}

type TransferInput struct {
	SourceRoot string
	SourcePath string
	Names      []string
	TargetRoot string
	TargetPath string
	Operation  string
	Conflict   string
}

type ConflictError struct{ Names []string }

func (e *ConflictError) Error() string { return "one or more target names already exist" }

type trashMetadata struct {
	RecycleEntry
	ParentPath string `json:"parentPath"`
}

func Resolve(root, relative string) (string, error) {
	root = filepath.Clean(root)
	if root == "." || !filepath.IsAbs(root) {
		return "", errors.New("share root must be an absolute path")
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve share root: %w", err)
	}
	relative, err = cleanRelative(relative)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(root, relative)
	if info, statErr := os.Lstat(candidate); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("symbolic links are not permitted in the file API")
		}
		real, evalErr := filepath.EvalSymlinks(candidate)
		if evalErr != nil {
			return "", fmt.Errorf("resolve file path: %w", evalErr)
		}
		if !within(rootReal, real) {
			return "", errors.New("path escapes the share root")
		}
		return real, nil
	} else if !os.IsNotExist(statErr) {
		return "", statErr
	}
	parent := filepath.Dir(candidate)
	parentReal, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", fmt.Errorf("resolve parent path: %w", err)
	}
	if !within(rootReal, parentReal) {
		return "", errors.New("path escapes the share root")
	}
	return filepath.Join(parentReal, filepath.Base(candidate)), nil
}

func List(root, relative string) ([]Entry, error) {
	directory, err := Resolve(root, relative)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("requested path is not a directory")
	}
	items, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	result := make([]Entry, 0, len(items))
	for _, item := range items {
		if item.Name() == trashDirectory {
			continue
		}
		entryInfo, err := item.Info()
		if err != nil {
			return nil, err
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			continue
		}
		kind := "file"
		if entryInfo.IsDir() {
			kind = "dir"
		}
		result = append(result, Entry{
			ID: entryID(root, filepath.Join(relative, item.Name())), Name: item.Name(), Type: kind,
			SizeBytes: size(entryInfo, filepath.Join(directory, item.Name())), ModifiedAt: entryInfo.ModTime().UTC(),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Type != result[j].Type {
			return result[i].Type == "dir"
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

func MakeDir(root, relative, name string) error {
	if err := validName(name); err != nil {
		return err
	}
	parent, err := Resolve(root, relative)
	if err != nil {
		return err
	}
	info, err := os.Stat(parent)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("parent path is not a directory")
	}
	path := filepath.Join(parent, name)
	if _, err := os.Lstat(path); err == nil {
		return os.ErrExist
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Mkdir(path, 0o770)
}

func Rename(root, relative, oldName, newName string) error {
	if err := validName(oldName); err != nil {
		return err
	}
	if err := validName(newName); err != nil {
		return err
	}
	parent, err := Resolve(root, relative)
	if err != nil {
		return err
	}
	source := filepath.Join(parent, oldName)
	if info, err := os.Lstat(source); err != nil {
		return err
	} else if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("symbolic links are not permitted in the file API")
	}
	target := filepath.Join(parent, newName)
	if _, err := os.Lstat(target); err == nil {
		return os.ErrExist
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(source, target)
}

func Delete(root, shareID, relative string, names []string) (int, error) {
	parent, err := Resolve(root, relative)
	if err != nil {
		return 0, err
	}
	trashRoot := filepath.Join(root, trashDirectory)
	if err := os.MkdirAll(trashRoot, 0o700); err != nil {
		return 0, err
	}
	deleted := 0
	for _, name := range names {
		if err := validName(name); err != nil {
			return deleted, err
		}
		source := filepath.Join(parent, name)
		info, err := os.Lstat(source)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return deleted, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return deleted, errors.New("symbolic links are not permitted in the file API")
		}
		id, err := newID()
		if err != nil {
			return deleted, err
		}
		itemRoot := filepath.Join(trashRoot, id)
		if err := os.Mkdir(itemRoot, 0o700); err != nil {
			return deleted, err
		}
		metadata := trashMetadata{RecycleEntry: RecycleEntry{
			ID: id, ShareID: shareID, Name: name, OriginalPath: normalizeRelative(relative),
			DeletedAt: time.Now().UTC(), SizeBytes: size(info, source),
		}, ParentPath: normalizeRelative(relative)}
		if err := os.Rename(source, filepath.Join(itemRoot, "data")); err != nil {
			_ = os.RemoveAll(itemRoot)
			return deleted, err
		}
		encoded, err := json.Marshal(metadata)
		if err != nil {
			_ = os.Rename(filepath.Join(itemRoot, "data"), source)
			_ = os.RemoveAll(itemRoot)
			return deleted, err
		}
		if err := os.WriteFile(filepath.Join(itemRoot, "metadata.json"), encoded, 0o600); err != nil {
			_ = os.Rename(filepath.Join(itemRoot, "data"), source)
			_ = os.RemoveAll(itemRoot)
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func ListTrash(root, shareID string) ([]RecycleEntry, error) {
	entries, err := os.ReadDir(filepath.Join(root, trashDirectory))
	if os.IsNotExist(err) {
		return []RecycleEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]RecycleEntry, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var metadata trashMetadata
		data, err := os.ReadFile(filepath.Join(root, trashDirectory, entry.Name(), "metadata.json"))
		if err != nil {
			continue
		}
		if json.Unmarshal(data, &metadata) != nil || metadata.ID != entry.Name() || (shareID != "" && metadata.ShareID != shareID) {
			continue
		}
		result = append(result, metadata.RecycleEntry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].DeletedAt.After(result[j].DeletedAt) })
	return result, nil
}

func Restore(root, id string) (bool, error) {
	metadata, itemRoot, err := loadTrash(root, id)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	parent, err := Resolve(root, metadata.ParentPath)
	if err != nil {
		return false, err
	}
	target := filepath.Join(parent, metadata.Name)
	if _, err := os.Lstat(target); err == nil {
		metadata.Name = uniqueName(parent, metadata.Name)
		target = filepath.Join(parent, metadata.Name)
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.Rename(filepath.Join(itemRoot, "data"), target); err != nil {
		return false, err
	}
	return true, os.RemoveAll(itemRoot)
}

func Purge(root, id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	_, itemRoot, err := loadTrash(root, id)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, os.RemoveAll(itemRoot)
}

func PurgeAll(root, shareID string) (int, error) {
	entries, err := ListTrash(root, shareID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		removed, err := Purge(root, entry.ID)
		if err != nil {
			return count, err
		}
		if removed {
			count++
		}
	}
	return count, nil
}

func Transfer(input TransferInput) (int, error) {
	sourceParent, err := Resolve(input.SourceRoot, input.SourcePath)
	if err != nil {
		return 0, err
	}
	targetParent, err := Resolve(input.TargetRoot, input.TargetPath)
	if err != nil {
		return 0, err
	}
	if sourceParent == targetParent && input.Operation == "move" {
		return 0, errors.New("source and target directories are identical")
	}
	if info, err := os.Stat(sourceParent); err != nil || !info.IsDir() {
		return 0, errors.New("source path is not a directory")
	}
	if info, err := os.Stat(targetParent); err != nil || !info.IsDir() {
		return 0, errors.New("target path is not a directory")
	}
	existing := map[string]bool{}
	for _, item := range input.Names {
		if err := validName(item); err != nil {
			return 0, err
		}
		if _, err := os.Lstat(filepath.Join(sourceParent, item)); err == nil {
			if _, targetErr := os.Lstat(filepath.Join(targetParent, item)); targetErr == nil {
				existing[item] = true
			}
		} else if !os.IsNotExist(err) {
			return 0, err
		}
	}
	if len(existing) > 0 && input.Conflict == "" {
		names := make([]string, 0, len(existing))
		for name := range existing {
			names = append(names, name)
		}
		sort.Strings(names)
		return 0, &ConflictError{Names: names}
	}
	transferred := 0
	for _, name := range input.Names {
		source := filepath.Join(sourceParent, name)
		if _, err := os.Lstat(source); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return transferred, err
		}
		targetName := name
		if existing[name] {
			switch input.Conflict {
			case "skip":
				continue
			case "rename":
				targetName = uniqueName(targetParent, name)
			case "overwrite":
				if err := os.RemoveAll(filepath.Join(targetParent, name)); err != nil {
					return transferred, err
				}
			default:
				return transferred, &ConflictError{Names: []string{name}}
			}
		}
		target := filepath.Join(targetParent, targetName)
		if input.Operation == "move" {
			if err := os.Rename(source, target); err != nil {
				return transferred, err
			}
		} else if err := copyTree(source, target); err != nil {
			return transferred, err
		}
		transferred++
	}
	return transferred, nil
}

func Conflicts(input TransferInput) ([]string, error) {
	sourceParent, err := Resolve(input.SourceRoot, input.SourcePath)
	if err != nil {
		return nil, err
	}
	targetParent, err := Resolve(input.TargetRoot, input.TargetPath)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(sourceParent); err != nil || !info.IsDir() {
		return nil, errors.New("source path is not a directory")
	}
	if info, err := os.Stat(targetParent); err != nil || !info.IsDir() {
		return nil, errors.New("target path is not a directory")
	}
	conflicts := make([]string, 0)
	for _, name := range input.Names {
		if err := validName(name); err != nil {
			return nil, err
		}
		if _, err := os.Lstat(filepath.Join(sourceParent, name)); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if _, err := os.Lstat(filepath.Join(targetParent, name)); err == nil {
			conflicts = append(conflicts, name)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	sort.Strings(conflicts)
	return conflicts, nil
}

func Upload(root, relative, name string, sizeBytes int64) (string, error) {
	if err := validName(name); err != nil {
		return "", err
	}
	if sizeBytes < 0 || sizeBytes > 1<<40 {
		return "", errors.New("upload size is outside the supported range")
	}
	parent, err := Resolve(root, relative)
	if err != nil {
		return "", err
	}
	finalName := uniqueName(parent, name)
	file, err := os.OpenFile(filepath.Join(parent, finalName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o660)
	if err != nil {
		return "", err
	}
	if err := file.Truncate(sizeBytes); err != nil {
		_ = file.Close()
		_ = os.Remove(filepath.Join(parent, finalName))
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return finalName, nil
}

func cleanRelative(value string) (string, error) {
	if strings.ContainsAny(value, "\x00\r\n\\") {
		return "", errors.New("invalid file path")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = "/"
	}
	for _, segment := range strings.Split(strings.TrimPrefix(value, "/"), string(os.PathSeparator)) {
		if segment == ".." {
			return "", errors.New("path traversal is not permitted")
		}
	}
	clean := filepath.Clean("/" + strings.TrimPrefix(value, "/"))
	if clean == "/"+trashDirectory || strings.HasPrefix(clean, "/"+trashDirectory+string(os.PathSeparator)) {
		return "", errors.New("recycle-bin internals are not addressable")
	}
	return strings.TrimPrefix(clean, string(os.PathSeparator)), nil
}

func normalizeRelative(value string) string {
	clean, err := cleanRelative(value)
	if err != nil || clean == "" {
		return "/"
	}
	return "/" + filepath.ToSlash(clean)
}

func validName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "\x00\r\n\\/") {
		return errors.New("invalid file name")
	}
	return nil
}

func within(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func entryID(root, relative string) string {
	return "file-" + hex.EncodeToString([]byte(filepath.Clean(root + "\x00" + relative)))[:12]
}

func size(info os.FileInfo, path string) int64 {
	if !info.IsDir() {
		return info.Size()
	}
	var total int64
	_ = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if value, infoErr := entry.Info(); infoErr == nil {
			total += value.Size()
		}
		return nil
	})
	return total
}

func newID() (string, error) {
	bytes := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", err
	}
	return "trash-" + hex.EncodeToString(bytes), nil
}

func loadTrash(root, id string) (trashMetadata, string, error) {
	if err := validName(id); err != nil {
		return trashMetadata{}, "", err
	}
	itemRoot := filepath.Join(root, trashDirectory, id)
	data, err := os.ReadFile(filepath.Join(itemRoot, "metadata.json"))
	if err != nil {
		return trashMetadata{}, itemRoot, err
	}
	var metadata trashMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return trashMetadata{}, itemRoot, err
	}
	if metadata.ID != id {
		return trashMetadata{}, itemRoot, errors.New("recycle-bin metadata mismatch")
	}
	return metadata, itemRoot, nil
}

func uniqueName(parent, name string) string {
	if _, err := os.Lstat(filepath.Join(parent, name)); os.IsNotExist(err) {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, index, ext)
		if _, err := os.Lstat(filepath.Join(parent, candidate)); os.IsNotExist(err) {
			return candidate
		}
	}
}

func copyTree(source, target string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("symbolic links are not permitted in file transfers")
	}
	if info.IsDir() {
		if err := os.Mkdir(target, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyTree(filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		_ = os.Remove(target)
		return copyErr
	}
	return closeErr
}
