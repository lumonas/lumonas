package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/recovery"
	"github.com/lumonas/lumonas/internal/store"
)

func TestRecoveryExportAndApplyConfiguredRuntime(t *testing.T) {
	t.Setenv("LUMONAS_AUTO_BACKUP_DISABLED", "true")
	t.Setenv("LUMONAS_NOTIFY_MIN_SEVERITY", "off")
	root := t.TempDir()
	recoveryDir := filepath.Join(root, "recovery")
	t.Setenv("LUMONAS_RECOVERY_KEY", "configured-runtime-recovery-key")
	t.Setenv("LUMONAS_RECOVERY_DIR", recoveryDir)
	secretPath := filepath.Join(root, "source-secret.bin")
	if err := os.WriteFile(secretPath, []byte("configured-runtime-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUMONAS_RECOVERY_SECRETS_FILE", secretPath)
	t.Setenv("LUMONAS_SHARES_FILE", filepath.Join(root, "shares.json"))
	t.Setenv("LUMONAS_SNAPRAID_CONFIG", filepath.Join(root, "snapraid.conf"))
	t.Setenv("LUMONAS_SAMBA_CONFIG", filepath.Join(root, "generated", "smb.conf"))
	t.Setenv("LUMONAS_NFS_EXPORTS", filepath.Join(root, "generated", "exports"))
	t.Setenv("LUMONAS_PRIVD_SOCKET", filepath.Join(root, "missing-privd.sock"))
	if err := os.WriteFile(filepath.Join(root, "snapraid.conf"), []byte("parity /srv/disks/serial_PARITY/snapraid.parity\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	server := testServer(t)
	stackRoot := filepath.Join(root, "stacks")
	server.dockerService = dockerruntime.New(stackRoot, func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, nil
	})

	postRecoveryUser(t, server, `{"name":"operator","password":"operator-password-123","managementRole":"admin"}`, "/api/v1/users", http.StatusCreated)
	postRecoveryUser(t, server, `{"id":"lan","uuid":"11111111-1111-1111-1111-111111111111","name":"LAN","interface":"eth0","enabled":true,"type":"ethernet","ipv4":{"method":"auto"},"ipv6":{"method":"disabled"},"reauthenticated":true}`, "/api/v1/network/connections", http.StatusCreated)
	postRecoveryUser(t, server, `{"id":"share-media","name":"Media","path":"/srv/pools/media","enabled":true,"protocols":[{"protocol":"smb","enabled":true},{"protocol":"nfs","enabled":true}],"access":[]}`, "/api/v1/shares", http.StatusCreated)
	postRecoveryUser(t, server, `{"name":"media","composeYaml":"services:\n  media:\n    image: example/media:latest\n    volumes:\n      - /srv/lumonas/docker/appdata/media:/config\n"}`, "/api/v1/docker/stacks", http.StatusCreated)

	exportResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(exportResponse, httptest.NewRequest(http.MethodPost, "/api/v1/recovery/export", nil))
	if exportResponse.Code != http.StatusCreated {
		t.Fatalf("recovery export status %d: %s", exportResponse.Code, exportResponse.Body.String())
	}
	bundle, err := os.ReadFile(filepath.Join(recoveryDir, "latest.mrb"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := recovery.Plan(bundle, []byte("configured-runtime-recovery-key"))
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Verified || !plan.DatabaseValid || !plan.DesiredStateValid || !plan.ComposeValid || !plan.EncryptedSecrets {
		t.Fatalf("production recovery export was not fully verified: %#v", plan)
	}
	for _, file := range []string{
		"docker/stacks/media/compose.yaml",
		"config/shares.json",
		"config/network-connections.json",
		"config/network-bindings.json",
		"config/firewall-policy.json",
		"storage/mounts.json",
	} {
		if !containsRecoveryFile(plan.Files, file) {
			t.Fatalf("configured runtime file was not exported: %s (%#v)", file, plan.Files)
		}
	}

	restoredRoot := filepath.Join(root, "blank-replacement")
	result, err := recovery.Apply(bundle, []byte("configured-runtime-recovery-key"), recovery.ApplyOptions{Root: restoredRoot})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || !result.DatabaseRestored || !result.SecretsRestored {
		t.Fatalf("unexpected recovery apply result: %#v", result)
	}
	if got, err := os.ReadFile(filepath.Join(restoredRoot, "var/lib/lumonas/secrets/recovered-secrets.bin")); err != nil || string(got) != "configured-runtime-secret" {
		t.Fatalf("restored encrypted secret = %q, err=%v", got, err)
	}
	compose, err := os.ReadFile(filepath.Join(restoredRoot, "srv/lumonas/docker/stacks/media/compose.yaml"))
	if err != nil || !strings.Contains(string(compose), "example/media:latest") {
		t.Fatalf("restored Compose file = %q, err=%v", compose, err)
	}
	networkConfig, err := os.ReadFile(filepath.Join(restoredRoot, "etc/lumonas/recovery/network-connections.json"))
	if err != nil || !strings.Contains(string(networkConfig), `"interface":"eth0"`) {
		t.Fatalf("restored network metadata = %q, err=%v", networkConfig, err)
	}

	restored, err := store.Open(filepath.Join(restoredRoot, "var/lib/lumonas/lumonas.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	principals, err := restored.ListPrincipals("")
	if err != nil || !hasPrincipalNamed(principals, "operator") {
		t.Fatalf("restored principals = %#v, err=%v", principals, err)
	}
	shares, err := restored.ListManagedShares()
	if err != nil || len(shares) != 1 || shares[0].ID != "share-media" {
		t.Fatalf("restored shares = %#v, err=%v", shares, err)
	}
	connections, err := restored.ListNetworkConnections()
	if err != nil || len(connections) != 1 || connections[0].ID != "lan" {
		t.Fatalf("restored network connections = %#v, err=%v", connections, err)
	}
}

func postRecoveryUser(t *testing.T, server *apiServer, body, endpoint string, want int) {
	t.Helper()
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(body)))
	if response.Code != want {
		t.Fatalf("POST %s status %d: %s", endpoint, response.Code, response.Body.String())
	}
}

func containsRecoveryFile(files []string, want string) bool {
	for _, file := range files {
		if file == want {
			return true
		}
	}
	return false
}

func hasPrincipalNamed(values []identity.Principal, name string) bool {
	for _, value := range values {
		if value.Name == name {
			return true
		}
	}
	return false
}
