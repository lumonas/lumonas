package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/updates"
)

func serveFeed(t *testing.T, version string) (*httptest.Server, func()) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest := updates.Manifest{
		FormatVersion: updates.ManifestFormatVersion,
		Version:       version,
		PackageSHA256: "6a4b1c9e51d2c7a6f0b3e5d8c2a19f7e4b6d3a5c8e1f2b4d7a9c6e3f5b8d2a1c",
		PackageSize:   2048,
		PublishedAt:   time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
	}
	canonical, err := updates.CanonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(private, canonical))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(updates.FeedDocument{Manifest: manifest, Signature: signature})
	}))
	publicHex := base64.StdEncoding.EncodeToString(public)
	t.Setenv("LUMONAS_UPDATE_PUBLIC_KEY", publicHex)
	return server, func() { server.Close() }
}

func persistFeedSettings(t *testing.T, server *apiServer, feedURL string) {
	t.Helper()
	settings, err := server.loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	updatesSection, _ := settings["updates"].(map[string]any)
	core, _ := updatesSection["core"].(map[string]any)
	core["channelUrl"] = feedURL
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.store.SetMeta(settingsMetaKey, string(encoded)); err != nil {
		t.Fatal(err)
	}
}

func readCoreUpdates(t *testing.T, server *apiServer) map[string]any {
	t.Helper()
	settings, err := server.loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	updatesSection, _ := settings["updates"].(map[string]any)
	core, _ := updatesSection["core"].(map[string]any)
	return core
}

func TestUpdateFeedCheckAdvertisesNewerVersion(t *testing.T) {
	server := testServer(t)
	server.version = "1.0.0"
	feed, closeFeed := serveFeed(t, "9.9.9")
	defer closeFeed()
	persistFeedSettings(t, server, feed.URL)

	job := model.Job{ID: "job-feed-newer", Type: "updates.check", Title: "Check for updates", State: "queued", CreatedAt: time.Now().UTC()}
	job = server.runUpdateCheck(job)

	core := readCoreUpdates(t, server)
	if core["available"] != "9.9.9" {
		t.Fatalf("expected available 9.9.9, got %#v", core["available"])
	}
	if _, hasError := core["lastError"]; hasError {
		t.Fatalf("unexpected lastError: %#v", core["lastError"])
	}
	if job.State != "successful" || job.Stage != "Update channels checked" {
		t.Fatalf("unexpected job outcome %q / %q", job.State, job.Stage)
	}
}

func TestUpdateFeedCheckClearsAvailabilityWhenCurrent(t *testing.T) {
	server := testServer(t)
	server.version = "5.0.0"
	feed, closeFeed := serveFeed(t, "0.9.0")
	defer closeFeed()
	persistFeedSettings(t, server, feed.URL)

	job := model.Job{ID: "job-feed-current", Type: "updates.check", Title: "Check for updates", State: "queued", CreatedAt: time.Now().UTC()}
	job = server.runUpdateCheck(job)

	core := readCoreUpdates(t, server)
	if value, hasAvailable := core["available"]; hasAvailable && value != nil {
		t.Fatalf("unexpected availability without a channel: %#v", core["available"])
	}
}

func TestUpdateFeedCheckSurvivesUnreachableFeed(t *testing.T) {
	server := testServer(t)
	server.version = "1.0.0"
	feed, closeFeed := serveFeed(t, "9.9.9")
	closeFeed() // stop immediately so the URL is unreachable
	persistFeedSettings(t, server, feed.URL)

	job := model.Job{ID: "job-feed-down", Type: "updates.check", Title: "Check for updates", State: "queued", CreatedAt: time.Now().UTC()}
	job = server.runUpdateCheck(job)

	core := readCoreUpdates(t, server)
	if feedError, ok := core["lastError"].(string); !ok || feedError == "" {
		t.Fatalf("expected persisted feed error, got %#v", core["lastError"])
	}
	if job.State != "successful" {
		t.Fatalf("expected offline-safe successful job, got %q", job.State)
	}
}

func TestUpdateFeedCheckWithoutChannelKeepsTimestampsOnly(t *testing.T) {
	server := testServer(t)
	job := model.Job{ID: "job-feed-empty", Type: "updates.check", Title: "Check for updates", State: "queued", CreatedAt: time.Now().UTC()}
	job = server.runUpdateCheck(job)
	core := readCoreUpdates(t, server)
	if _, hasError := core["lastError"]; hasError {
		t.Fatalf("unexpected lastError without a channel: %#v", core["lastError"])
	}
	if value, hasAvailable := core["available"]; hasAvailable && value != nil {
		t.Fatalf("unexpected availability without a channel: %#v", core["available"])
	}
}
