package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lumonas/lumonas/internal/storage"
)

const snapshotReplicationMaxStreamBytes int64 = 1 << 40

type streamCommandRunner func(context.Context, string, []string, io.Writer, io.Reader) error

var runSnapshotStreamCommand streamCommandRunner = defaultSnapshotStreamCommand
var activeSnapshotStreams sync.Map

type snapshotStreamOperation struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	canceled bool
}

func registerSnapshotStreamOperation(operationID string, cancel context.CancelFunc) *snapshotStreamOperation {
	created := &snapshotStreamOperation{}
	value, _ := activeSnapshotStreams.LoadOrStore(operationID, created)
	operation := value.(*snapshotStreamOperation)
	operation.mu.Lock()
	operation.cancel = cancel
	cancelNow := operation.canceled
	operation.mu.Unlock()
	if cancelNow {
		cancel()
	}
	return operation
}

func finishSnapshotStreamOperation(operationID string, operation *snapshotStreamOperation) {
	operation.mu.Lock()
	operation.cancel = nil
	activeSnapshotStreams.CompareAndDelete(operationID, operation)
	operation.mu.Unlock()
}

type boundedCommandOutput struct {
	buffer bytes.Buffer
	limit  int
}

func (output *boundedCommandOutput) Write(value []byte) (int, error) {
	if output.limit <= 0 {
		return len(value), nil
	}
	remaining := output.limit - output.buffer.Len()
	if remaining > 0 {
		if len(value) > remaining {
			_, _ = output.buffer.Write(value[:remaining])
		} else {
			_, _ = output.buffer.Write(value)
		}
	}
	return len(value), nil
}

func (output *boundedCommandOutput) String() string { return strings.TrimSpace(output.buffer.String()) }

func defaultSnapshotStreamCommand(ctx context.Context, command string, args []string, stdout io.Writer, stdin io.Reader) error {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout = stdout
	cmd.Stdin = stdin
	stderr := &boundedCommandOutput{limit: 64 * 1024}
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if detail := stderr.String(); detail != "" {
			return fmt.Errorf("%s: %w", detail, err)
		}
		return err
	}
	return nil
}

func executeSnapshotStreamExport(req request) response {
	if !req.Confirmed || !validOperationID(req.OperationID) {
		return response{Error: "a confirmed operation ID is required"}
	}
	kind := storage.SnapshotKind(requestedString(req.RequestedState, "kind"))
	source := requestedString(req.RequestedState, "source")
	name := requestedString(req.RequestedState, "name")
	parent := requestedString(req.RequestedState, "parent")
	if kind != storage.SnapshotBtrfs {
		return response{Error: "native snapshot streaming currently requires Btrfs"}
	}
	if err := storage.ValidateSnapshotSource(kind, source); err != nil {
		return response{Error: err.Error()}
	}
	if err := validateManagedSnapshotPath(source); err != nil {
		return response{Error: err.Error()}
	}
	if err := storage.ValidateSnapshotName(name); err != nil {
		return response{Error: err.Error()}
	}
	if parent != "" {
		if err := storage.ValidateSnapshotName(parent); err != nil {
			return response{Error: "incremental parent snapshot name is invalid"}
		}
	}
	snapshotPath := filepath.Join(source+".snapshots", name)
	if err := validateSnapshotSubvolumePath(source, snapshotPath); err != nil {
		return response{Error: err.Error()}
	}
	args := []string{"send"}
	if parent != "" {
		parentPath := filepath.Join(source+".snapshots", parent)
		if err := validateSnapshotSubvolumePath(source, parentPath); err != nil {
			return response{Error: "incremental parent snapshot is unavailable"}
		}
		args = append(args, "-p", parentPath)
	}
	args = append(args, snapshotPath)
	root := envOr("LUMONAS_REPLICATION_DIR", "/var/lib/lumonas/replication")
	if err := ensureReplicationStage(root); err != nil {
		return response{Error: "replication staging is unavailable"}
	}
	streamPath := filepath.Join(root, "out-"+req.OperationID+".stream")
	stream, err := os.OpenFile(streamPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return response{Error: "could not create snapshot transfer stream"}
	}
	if err := stream.Chmod(0o640); err != nil {
		stream.Close()
		_ = os.Remove(streamPath)
		return response{Error: "could not secure snapshot transfer stream"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Hour)
	defer cancel()
	activeOperation := registerSnapshotStreamOperation(req.OperationID, cancel)
	defer finishSnapshotStreamOperation(req.OperationID, activeOperation)
	commandErr := runSnapshotStreamCommand(ctx, "btrfs", args, stream, nil)
	closeErr := stream.Close()
	if commandErr != nil || closeErr != nil {
		_ = os.Remove(streamPath)
		return response{Error: "Btrfs snapshot export failed"}
	}
	info, err := os.Stat(streamPath)
	if err != nil || info.Size() <= 0 || info.Size() > snapshotReplicationMaxStreamBytes {
		_ = os.Remove(streamPath)
		return response{Error: "snapshot transfer stream is empty or exceeds the 1 TiB safety limit"}
	}
	digest, err := hashStreamFile(streamPath)
	if err != nil {
		_ = os.Remove(streamPath)
		return response{Error: "snapshot transfer stream could not be verified"}
	}
	return response{OK: true, Data: map[string]any{"streamId": req.OperationID, "bytes": info.Size(), "sha256": digest}}
}

func executeSnapshotStreamReceive(req request) response {
	if !req.Confirmed || !validOperationID(req.OperationID) {
		return response{Error: "a confirmed operation ID is required"}
	}
	kind := storage.SnapshotKind(requestedString(req.RequestedState, "kind"))
	target := requestedString(req.RequestedState, "target")
	name := requestedString(req.RequestedState, "name")
	parent := requestedString(req.RequestedState, "parent")
	streamID := requestedString(req.RequestedState, "streamId")
	if kind != storage.SnapshotBtrfs {
		return response{Error: "native snapshot streaming currently requires Btrfs"}
	}
	if !validOperationID(streamID) {
		return response{Error: "snapshot stream ID is invalid"}
	}
	if err := validateManagedSnapshotPath(target); err != nil {
		return response{Error: err.Error()}
	}
	if err := storage.ValidateSnapshotName(name); err != nil {
		return response{Error: err.Error()}
	}
	if parent != "" {
		if err := storage.ValidateSnapshotName(parent); err != nil {
			return response{Error: "incremental parent snapshot name is invalid"}
		}
	}
	streamRoot := envOr("LUMONAS_REPLICATION_INCOMING_DIR", "/var/lib/lumonas/replication-incoming")
	if err := validateReplicationIncomingDir(streamRoot); err != nil {
		return response{Error: "incoming snapshot transfer is unavailable"}
	}
	streamPath := filepath.Join(streamRoot, streamID+".stream")
	streamInfo, err := os.Lstat(streamPath)
	if err != nil || !streamInfo.Mode().IsRegular() || streamInfo.Size() <= 0 || streamInfo.Size() > snapshotReplicationMaxStreamBytes {
		return response{Error: "incoming snapshot stream is missing or invalid"}
	}
	snapshotDir := target + ".snapshots"
	if err := ensureSnapshotDirectory(target, snapshotDir); err != nil {
		return response{Error: "destination snapshot directory could not be prepared"}
	}
	receivedPath := filepath.Join(snapshotDir, name)
	if _, err := os.Lstat(receivedPath); err == nil {
		return response{Error: "destination already contains this snapshot name"}
	} else if !errors.Is(err, os.ErrNotExist) {
		return response{Error: "destination snapshot state could not be checked"}
	}
	if parent != "" {
		parentPath := filepath.Join(snapshotDir, parent)
		parentInfo, err := os.Lstat(parentPath)
		resolved, resolveErr := filepath.EvalSymlinks(parentPath)
		if err != nil || resolveErr != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 || resolved != parentPath {
			return response{Error: "incremental parent snapshot has not been received"}
		}
		if err := runSnapshotStreamCommand(context.Background(), "btrfs", []string{"subvolume", "show", parentPath}, io.Discard, nil); err != nil {
			return response{Error: "incremental parent is not a Btrfs subvolume"}
		}
	}
	input, err := os.Open(streamPath)
	if err != nil {
		return response{Error: "incoming snapshot stream could not be opened"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Hour)
	defer cancel()
	activeOperation := registerSnapshotStreamOperation(req.OperationID, cancel)
	defer finishSnapshotStreamOperation(req.OperationID, activeOperation)
	commandErr := runSnapshotStreamCommand(ctx, "btrfs", []string{"receive", "--", snapshotDir}, io.Discard, input)
	closeErr := input.Close()
	if commandErr != nil || closeErr != nil {
		return response{Error: "Btrfs snapshot receive failed"}
	}
	if err := runSnapshotStreamCommand(context.Background(), "btrfs", []string{"subvolume", "show", receivedPath}, io.Discard, nil); err != nil {
		return response{Error: "received snapshot did not pass Btrfs validation"}
	}
	_ = os.Remove(streamPath)
	return response{OK: true, Data: map[string]any{"snapshot": name, "parent": parent, "bytes": streamInfo.Size()}}
}

func executeSnapshotStreamRemove(req request) response {
	if !req.Confirmed || !validOperationID(req.OperationID) {
		return response{Error: "a confirmed operation ID is required"}
	}
	root := envOr("LUMONAS_REPLICATION_DIR", "/var/lib/lumonas/replication")
	streamPath := filepath.Join(root, "out-"+req.OperationID+".stream")
	if err := os.Remove(streamPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return response{Error: "snapshot transfer stream could not be removed"}
	}
	return response{OK: true, Data: map[string]string{"removed": req.OperationID}}
}

func executeSnapshotStreamCancel(req request) response {
	if !req.Confirmed || !validOperationID(req.OperationID) {
		return response{Error: "a confirmed operation ID is required"}
	}
	created := &snapshotStreamOperation{canceled: true}
	value, loaded := activeSnapshotStreams.LoadOrStore(req.OperationID, created)
	operation := value.(*snapshotStreamOperation)
	operation.mu.Lock()
	operation.canceled = true
	cancel := operation.cancel
	operation.mu.Unlock()
	if !loaded {
		time.AfterFunc(time.Minute, func() { activeSnapshotStreams.CompareAndDelete(req.OperationID, operation) })
	}
	if cancel != nil {
		cancel()
	}
	return response{OK: true, Data: map[string]any{"operationId": req.OperationID, "state": "cancellation-requested"}}
}

func validateManagedSnapshotPath(value string) error {
	if !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return errors.New("snapshot path must be a clean absolute managed path")
	}
	for _, root := range []string{"/srv/lumonas", "/srv/pools", "/srv/disks"} {
		relative, err := filepath.Rel(root, value)
		if err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			resolved, resolveErr := filepath.EvalSymlinks(value)
			if resolveErr != nil || resolved != value {
				return errors.New("managed snapshot path is unavailable or uses a symlink")
			}
			return nil
		}
	}
	return errors.New("snapshot path must be inside managed storage")
}

func validateSnapshotSubvolumePath(source, snapshotPath string) error {
	root, err := filepath.EvalSymlinks(source)
	if err != nil || root != source {
		return errors.New("snapshot source is unavailable or uses a symlink")
	}
	resolved, err := filepath.EvalSymlinks(snapshotPath)
	if err != nil || resolved != snapshotPath {
		return errors.New("snapshot does not exist or uses a symlink")
	}
	relative, err := filepath.Rel(source+".snapshots", resolved)
	if err != nil || relative != filepath.Base(snapshotPath) {
		return errors.New("snapshot path escapes its source")
	}
	return nil
}

func ensureReplicationStage(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return errors.New("replication directory must be an absolute clean path")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o007 != 0 {
		return errors.New("replication directory must be a real directory")
	}
	return nil
}

func ensureSnapshotDirectory(target, snapshotDir string) error {
	if err := validateManagedSnapshotPath(target); err != nil {
		return err
	}
	expected := target + ".snapshots"
	if snapshotDir != expected || filepath.Clean(snapshotDir) != snapshotDir {
		return errors.New("snapshot directory path is invalid")
	}
	if err := os.Mkdir(snapshotDir, 0o750); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(snapshotDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("snapshot directory must be a real directory")
	}
	resolved, err := filepath.EvalSymlinks(snapshotDir)
	if err != nil || resolved != snapshotDir {
		return errors.New("snapshot directory must not traverse symlinks")
	}
	return nil
}

func validateReplicationIncomingDir(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return errors.New("incoming directory must be an absolute clean path")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("incoming directory must be a real directory")
	}
	return nil
}

func hashStreamFile(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(file, snapshotReplicationMaxStreamBytes+1)); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
