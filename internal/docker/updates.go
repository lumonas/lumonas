package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CheckImageUpdates compares locally present images against the registry
// manifest digest and flags the ones where a newer build exists upstream.
// It never pulls image layers: `docker manifest inspect` fetches only
// manifests.
func (s Service) CheckImageUpdates(ctx context.Context) ([]Image, error) {
	images, err := s.Images(ctx)
	if err != nil {
		return nil, err
	}
	containers, err := s.Containers(ctx)
	if err != nil {
		return nil, err
	}
	inUse := make(map[string]bool, len(containers))
	for _, container := range containers {
		inUse[container.Image] = true
	}
	for index := range images {
		image := &images[index]
		image.InUse = inUse[image.Repo+":"+image.Tag] || inUse[image.ID]
		if image.Tag == "" || image.Tag == "<none>" || strings.HasPrefix(image.Repo, "<none>") {
			continue
		}
		reference := image.Repo + ":" + image.Tag
		localDigest, localErr := s.localDigest(ctx, reference)
		if localErr != nil {
			continue
		}
		remoteDigest, remoteErr := s.remoteDigest(ctx, reference)
		if remoteErr != nil {
			continue
		}
		image.UpdateAvailable = localDigest != remoteDigest
	}
	return images, nil
}

// localDigest returns the registry digest the local image was pulled from.
func (s Service) localDigest(ctx context.Context, reference string) (string, error) {
	out, err := s.Run(ctx, "docker", "image", "inspect", "--format", "{{index .RepoDigests 0}}", reference)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(out))
	if at := strings.IndexByte(value, '@'); at >= 0 {
		return value[at+1:], nil
	}
	return value, nil
}

// remoteDigest returns the digest of the manifest currently served for the
// reference.
func (s Service) remoteDigest(ctx context.Context, reference string) (string, error) {
	out, err := s.Run(ctx, "docker", "manifest", "inspect", "--verbose", reference)
	if err != nil {
		return "", err
	}
	var payload struct {
		Descriptor struct {
			Digest string `json:"digest"`
		} `json:"Descriptor"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return "", fmt.Errorf("parse manifest inspect: %w", err)
	}
	if payload.Descriptor.Digest == "" {
		return "", fmt.Errorf("manifest inspect returned no digest for %s", reference)
	}
	return payload.Descriptor.Digest, nil
}
