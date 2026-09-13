package store

import (
	"testing"

	"github.com/lumonas/lumonas/internal/model"
)

func TestDockerDeploymentsPersistTransactionsAcrossQueries(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	before := "services:\n  app:\n    image: example/app:1\n"
	if err := database.CreateDockerDeployment(model.DockerDeployment{
		ID:              "deployment-1",
		StackName:       "media",
		Kind:            "update",
		ComposeBefore:   &before,
		ComposeAfter:    "services:\n  app:\n    image: example/app:2\n",
		ImageBeforeJSON: `[{"Repository":"example/app","Tag":"1","ID":"sha256:old"}]`,
	}); err != nil {
		t.Fatal(err)
	}
	pending, err := database.PendingDockerDeployments()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].State != "pending" || pending[0].ComposeBefore == nil || pending[0].ImageBeforeJSON == "" {
		t.Fatalf("unexpected pending deployment: %#v", pending)
	}
	if err := database.UpdateDockerDeployment("deployment-1", "committed", ""); err != nil {
		t.Fatal(err)
	}
	items, err := database.DockerDeployments(10, "media")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].State != "committed" || items[0].ComposeAfter == "" {
		t.Fatalf("unexpected deployment history: %#v", items)
	}
}

func TestPruneDockerDeploymentsKeepsPendingAndNewestTerminalRows(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	for index := 0; index < 105; index++ {
		if err := database.CreateDockerDeployment(model.DockerDeployment{
			ID:           "deployment-" + string(rune('a'+index)),
			StackName:    "media",
			Kind:         "update",
			ComposeAfter: "services:\n  app:\n    image: example/app:latest\n",
		}); err != nil {
			t.Fatal(err)
		}
		if err := database.UpdateDockerDeployment("deployment-"+string(rune('a'+index)), "committed", ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.CreateDockerDeployment(model.DockerDeployment{
		ID:           "deployment-pending",
		StackName:    "media",
		Kind:         "install",
		ComposeAfter: "services:\n  app:\n    image: example/app:latest\n",
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.PruneDockerDeployments(100); err != nil {
		t.Fatal(err)
	}
	var terminal, pending int
	if err := database.db.QueryRow(`SELECT COUNT(*) FROM docker_deployments WHERE state <> 'pending'`).Scan(&terminal); err != nil {
		t.Fatal(err)
	}
	if err := database.db.QueryRow(`SELECT COUNT(*) FROM docker_deployments WHERE state = 'pending'`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if terminal != 100 || pending != 1 {
		t.Fatalf("unexpected retained deployment counts: terminal=%d pending=%d", terminal, pending)
	}
}
