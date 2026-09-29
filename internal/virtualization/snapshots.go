package virtualization

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Snapshot struct {
	Name         string `json:"name"`
	State        string `json:"state,omitempty"`
	CreationTime string `json:"creationTime,omitempty"`
}

func (s Service) Snapshots(ctx context.Context, domain string) ([]Snapshot, error) {
	if !domainNamePattern.MatchString(domain) {
		return nil, errors.New("VM name is invalid")
	}
	output, err := s.command(ctx, "snapshot-list", domain, "--name")
	if err != nil {
		return nil, fmt.Errorf("list snapshots for VM %q: %w", domain, err)
	}
	snapshots := make([]Snapshot, 0)
	for _, name := range strings.Split(string(output), "\n") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !domainNamePattern.MatchString(name) {
			return nil, errors.New("libvirt returned an invalid snapshot name")
		}
		info, infoErr := s.command(ctx, "snapshot-info", domain, "--snapshotname", name)
		if infoErr != nil {
			return nil, fmt.Errorf("read snapshot %q: %w", name, infoErr)
		}
		snapshots = append(snapshots, parseSnapshot(name, string(info)))
	}
	return snapshots, nil
}

func (s Service) CreateSnapshot(ctx context.Context, domain, name string) (Snapshot, error) {
	if !domainNamePattern.MatchString(domain) || !domainNamePattern.MatchString(name) {
		return Snapshot{}, errors.New("VM and snapshot names must use 1–63 letters, numbers, dots, underscores, or hyphens")
	}
	if _, err := s.command(ctx, "snapshot-create-as", domain, "--name", name, "--description", "Created by LumoNAS", "--atomic"); err != nil {
		return Snapshot{}, fmt.Errorf("create snapshot for VM %q: %w", domain, err)
	}
	info, err := s.command(ctx, "snapshot-info", domain, "--snapshotname", name)
	if err != nil {
		return Snapshot{Name: name}, nil
	}
	return parseSnapshot(name, string(info)), nil
}

func (s Service) RevertSnapshot(ctx context.Context, domain, name string) error {
	if !domainNamePattern.MatchString(domain) || !domainNamePattern.MatchString(name) {
		return errors.New("VM or snapshot name is invalid")
	}
	if _, err := s.command(ctx, "snapshot-revert", domain, "--snapshotname", name); err != nil {
		return fmt.Errorf("restore snapshot %q for VM %q: %w", name, domain, err)
	}
	return nil
}

func (s Service) DeleteSnapshot(ctx context.Context, domain, name string) error {
	if !domainNamePattern.MatchString(domain) || !domainNamePattern.MatchString(name) {
		return errors.New("VM or snapshot name is invalid")
	}
	if _, err := s.command(ctx, "snapshot-delete", domain, "--snapshotname", name); err != nil {
		return fmt.Errorf("delete snapshot %q for VM %q: %w", name, domain, err)
	}
	return nil
}

func parseSnapshot(name, raw string) Snapshot {
	snapshot := Snapshot{Name: name}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.SplitN(line, ":", 2)
		if len(fields) != 2 {
			continue
		}
		key, value := strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1])
		switch key {
		case "State":
			snapshot.State = strings.ToLower(value)
		case "Creation Time":
			snapshot.CreationTime = value
		}
	}
	return snapshot
}
