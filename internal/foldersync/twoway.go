package foldersync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BuildTwoWayPlan merges two managed roots against the last successfully
// synchronized baseline. Divergent edits are retained in each endpoint's
// .lumonas-versions tree before the newer edit is copied to both sides. A
// delete is propagated only when the other side still matches the baseline.
func BuildTwoWayPlan(left, right, baseline map[string]Entry, initialized, deep bool) Plan {
	plan := Plan{Changes: []Change{}}
	paths := make(map[string]struct{}, len(left)+len(right)+len(baseline))
	for name := range left {
		paths[name] = struct{}{}
	}
	for name := range right {
		paths[name] = struct{}{}
	}
	for name := range baseline {
		paths[name] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for name := range paths {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	for _, name := range ordered {
		l, lok := left[name]
		r, rok := right[name]
		base, bok := baseline[name]
		if !initialized {
			if lok && rok {
				if sameEntry(l, r, deep) {
					continue
				}
				plan.Conflicts++
				archiveBoth(&plan, "left", name, l, "initial-left", deep)
				archiveBoth(&plan, "right", name, r, "initial-right", deep)
				newer, from := newerEntry(l, r, "left")
				_ = newer
				copyToOther(&plan, name, from, l, r, deep)
				continue
			}
			if lok {
				addCopy(&plan, name, "left", "right", l, "initial sync", deep)
			} else if rok {
				addCopy(&plan, name, "right", "left", r, "initial sync", deep)
			}
			continue
		}

		leftChanged := !entryMatchesBaseline(l, lok, base, bok, deep)
		rightChanged := !entryMatchesBaseline(r, rok, base, bok, deep)
		if !leftChanged && !rightChanged {
			continue
		}
		if leftChanged && !rightChanged {
			propagateChange(&plan, name, "left", "right", l, lok, r, rok, deep)
			continue
		}
		if rightChanged && !leftChanged {
			propagateChange(&plan, name, "right", "left", r, rok, l, lok, deep)
			continue
		}
		if lok && rok && sameEntry(l, r, deep) {
			continue
		}
		if !lok && !rok {
			// A bilateral delete needs no conflict entry or additional action.
			continue
		}

		plan.Conflicts++
		if lok {
			archiveBoth(&plan, "left", name, l, "conflict-left", deep)
		}
		if rok {
			archiveBoth(&plan, "right", name, r, "conflict-right", deep)
		}
		if lok && rok {
			_, from := newerEntry(l, r, "left")
			copyToOther(&plan, name, from, l, r, deep)
		} else if lok {
			// A concurrent delete/edit conflict favors deletion, while the edit
			// remains recoverable in .lumonas-versions on both endpoints.
			addDelete(&plan, name, "left")
		} else if rok {
			addDelete(&plan, name, "right")
		}
	}
	return plan
}

func entryMatchesBaseline(current Entry, exists bool, base Entry, baseExists, deep bool) bool {
	if exists != baseExists {
		return false
	}
	return !exists || sameEntry(current, base, deep)
}

func sameEntry(a, b Entry, deep bool) bool {
	if a.Size != b.Size || !a.ModTime.Equal(b.ModTime) {
		return false
	}
	return !deep || a.Hash != "" && b.Hash != "" && strings.EqualFold(a.Hash, b.Hash)
}

func newerEntry(left, right Entry, leftSide string) (Entry, string) {
	if right.ModTime.After(left.ModTime) {
		if leftSide == "left" {
			return right, "right"
		}
		return left, "left"
	}
	if leftSide == "left" {
		return left, "left"
	}
	return right, "right"
}

func copyToOther(plan *Plan, name, from string, left, right Entry, deep bool) {
	if from == "left" {
		addCopy(plan, name, "left", "right", left, "conflict winner", deep)
	} else {
		addCopy(plan, name, "right", "left", right, "conflict winner", deep)
	}
}

func propagateChange(plan *Plan, name, from, to string, current Entry, currentExists bool, other Entry, otherExists, deep bool) {
	if !currentExists {
		if otherExists {
			archiveBoth(plan, to, name, other, "deleted", deep)
			addDelete(plan, name, to)
		}
		return
	}
	if otherExists {
		archiveBoth(plan, to, name, other, "replaced", deep)
	}
	addCopy(plan, name, from, to, current, "one side changed", deep)
}

func archiveBoth(plan *Plan, from, name string, entry Entry, reason string, deep bool) {
	stamp := entry.ModTime.UTC().Format("20060102T150405.000000000Z")
	if entry.Hash != "" {
		stamp += "-" + entry.Hash[:min(12, len(entry.Hash))]
	}
	archivePath := filepath.ToSlash(filepath.Join(".lumonas-versions", filepath.FromSlash(name)+"."+stamp+"."+reason))
	for _, to := range []string{"left", "right"} {
		plan.Changes = append(plan.Changes, Change{Path: name, ArchivePath: archivePath, Action: "archive", From: from, To: to, Bytes: entry.Size, Hash: entry.Hash, Reason: reason})
		plan.Files++
		plan.Bytes += entry.Size
	}
	_ = deep
}

func addCopy(plan *Plan, name, from, to string, entry Entry, reason string, deep bool) {
	action := "copy"
	if reason != "initial sync" {
		action = "update"
	}
	plan.Changes = append(plan.Changes, Change{Path: name, Action: action, From: from, To: to, Bytes: entry.Size, Hash: entry.Hash, Reason: reason})
	plan.Files++
	plan.Bytes += entry.Size
	_ = deep
}

func addDelete(plan *Plan, name, side string) {
	plan.Changes = append(plan.Changes, Change{Path: name, Action: "delete", To: side})
	plan.Deletes++
}

// ApplyTwoWay executes a reviewed bilateral plan. All reads and writes stay
// inside opened roots. Archives complete before any overwrite or deletion.
func ApplyTwoWay(ctx context.Context, leftRoot, rightRoot string, plan Plan, progress func(done int, bytes int64)) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := safeRoots(leftRoot, rightRoot); err != nil {
		return err
	}
	left, err := os.OpenRoot(leftRoot)
	if err != nil {
		return err
	}
	defer left.Close()
	right, err := os.OpenRoot(rightRoot)
	if err != nil {
		return err
	}
	defer right.Close()
	roots := map[string]*os.Root{"left": left, "right": right}
	var done int
	var transferred int64
	copyChange := func(change Change, target string) error {
		from := roots[change.From]
		to := roots[target]
		if from == nil || to == nil {
			return errors.New("two-way plan references an unknown endpoint")
		}
		relative, err := cleanRelativePath(change.Path)
		if err != nil {
			return err
		}
		copyTo := relative
		if change.Action == "archive" {
			copyTo, err = cleanRelativePath(change.ArchivePath)
			if err != nil {
				return err
			}
		}
		input, err := from.Open(relative)
		if err != nil {
			return err
		}
		info, err := input.Stat()
		if err != nil || !info.Mode().IsRegular() {
			_ = input.Close()
			return errors.New("sync source changed and is no longer a regular file")
		}
		if err := copyVerifiedIntoRoot(ctx, to, copyTo, input); err != nil {
			_ = input.Close()
			return err
		}
		if err := input.Close(); err != nil {
			return err
		}
		if err := to.Chtimes(copyTo, info.ModTime(), info.ModTime()); err != nil {
			return err
		}
		done++
		transferred += change.Bytes
		if progress != nil {
			progress(done, transferred)
		}
		return nil
	}

	// Create version archives first so an interruption never destroys the only
	// retained copy of an overwritten or deleted file.
	for _, change := range plan.Changes {
		if change.Action != "archive" {
			continue
		}
		if err := copyChange(change, change.To); err != nil {
			return fmt.Errorf("archive %s: %w", change.Path, err)
		}
	}
	for _, change := range plan.Changes {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		switch change.Action {
		case "copy", "update":
			if err := copyChange(change, change.To); err != nil {
				return fmt.Errorf("sync %s: %w", change.Path, err)
			}
		case "delete":
			relative, err := cleanRelativePath(change.Path)
			if err != nil {
				return err
			}
			if err := roots[change.To].Remove(relative); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("delete %s: %w", change.Path, err)
			}
		}
	}
	return nil
}
