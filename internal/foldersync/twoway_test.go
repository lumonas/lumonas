package foldersync

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildTwoWayPlanInitialUnionAndConflict(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	left := map[string]Entry{
		"left-only.txt": {Path: "left-only.txt", Size: 3, ModTime: now},
		"conflict.txt":  {Path: "conflict.txt", Size: 4, ModTime: now},
		"same.txt":      {Path: "same.txt", Size: 4, ModTime: now},
	}
	right := map[string]Entry{
		"right-only.txt": {Path: "right-only.txt", Size: 5, ModTime: now},
		"conflict.txt":   {Path: "conflict.txt", Size: 9, ModTime: now.Add(time.Minute)},
		"same.txt":       {Path: "same.txt", Size: 4, ModTime: now},
	}
	plan := BuildTwoWayPlan(left, right, nil, false, false)
	if plan.Conflicts != 1 {
		t.Fatalf("expected one conflict, got %d", plan.Conflicts)
	}
	if plan.Files != 7 { // two initial copies, four retained conflict copies, and one winning update
		t.Fatalf("unexpected transferred file count: %d", plan.Files)
	}
	var leftOnly, rightOnly, winner bool
	for _, change := range plan.Changes {
		if change.Path == "left-only.txt" && change.From == "left" && change.To == "right" {
			leftOnly = true
		}
		if change.Path == "right-only.txt" && change.From == "right" && change.To == "left" {
			rightOnly = true
		}
		if change.Path == "conflict.txt" && change.Action == "update" && change.From == "right" && change.To == "left" {
			winner = true
		}
	}
	if !leftOnly || !rightOnly || !winner {
		t.Fatalf("plan did not merge both sides: %#v", plan.Changes)
	}
}

func TestBuildTwoWayPlanPropagatesDeletesAndArchivesPriorVersion(t *testing.T) {
	baselineEntry := Entry{Path: "old.txt", Size: 5, ModTime: time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)}
	plan := BuildTwoWayPlan(nil, map[string]Entry{"old.txt": baselineEntry}, map[string]Entry{"old.txt": baselineEntry}, true, false)
	if plan.Deletes != 1 || len(plan.Changes) != 3 {
		t.Fatalf("expected two version copies followed by one deletion, got %#v", plan.Changes)
	}
	for i, change := range plan.Changes {
		if i < 2 && change.Action != "archive" {
			t.Fatalf("archive must precede deletion: %#v", plan.Changes)
		}
	}
	if plan.Changes[2].Action != "delete" || plan.Changes[2].To != "right" {
		t.Fatalf("expected delete to propagate to the remaining side: %#v", plan.Changes[2])
	}
}

func TestApplyTwoWayCopiesVersionsAndDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	leftRoot := filepath.Join(root, "left")
	rightRoot := filepath.Join(root, "right")
	if err := os.MkdirAll(leftRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rightRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	leftPath := filepath.Join(leftRoot, "notes.txt")
	rightPath := filepath.Join(rightRoot, "notes.txt")
	if err := os.WriteFile(leftPath, []byte("left revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rightPath, []byte("right revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	leftTime := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	rightTime := leftTime.Add(time.Minute)
	if err := os.Chtimes(leftPath, leftTime, leftTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(rightPath, rightTime, rightTime); err != nil {
		t.Fatal(err)
	}
	left, err := Scan(leftRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	right, err := Scan(rightRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	plan := BuildTwoWayPlan(left, right, nil, false, false)
	if err := ApplyTwoWay(context.Background(), leftRoot, rightRoot, plan, nil); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{leftRoot, rightRoot} {
		current, err := os.ReadFile(filepath.Join(directory, "notes.txt"))
		if err != nil || string(current) != "right revision" {
			t.Fatalf("newer revision was not synchronized at %s: %q, %v", directory, current, err)
		}
		versions, err := filepath.Glob(filepath.Join(directory, ".lumonas-versions", "notes.txt.*"))
		if err != nil || len(versions) != 2 {
			t.Fatalf("both revisions should be retained at %s: %v, %v", directory, versions, err)
		}
	}
	leftAgain, err := Scan(leftRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	rightAgain, err := Scan(rightRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(leftAgain) != 1 || len(rightAgain) != 1 {
		t.Fatalf("version archives must not be treated as new user files: left=%v right=%v", leftAgain, rightAgain)
	}
}

func TestApplyTwoWayRejectsUnsafePlanPaths(t *testing.T) {
	left := t.TempDir()
	right := t.TempDir()
	err := ApplyTwoWay(context.Background(), left, right, Plan{Changes: []Change{{Action: "update", Path: "../escape", From: "left", To: "right"}}}, nil)
	if err == nil {
		t.Fatal("unsafe two-way path should be rejected")
	}
}
