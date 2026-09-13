package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/privileged"
)

// needsOSProvisioning reports whether a principal maps to a real OS account:
// file users and service identities. Management users and groups live
// entirely in the LumoNAS store.
func needsOSProvisioning(principal identity.Principal) bool {
	return (principal.Kind == identity.KindUser || principal.Kind == identity.KindService) && principal.ManagementRole == identity.RoleNone
}

// fileIdentityGID is the primary group for provisioned file identities.
// It defaults to the Debian "users" group and can be relocated per deployment.
func fileIdentityGID() int {
	if value := os.Getenv("LUMONAS_FILE_USER_GID"); value != "" {
		if gid, err := strconv.Atoi(value); err == nil && gid >= 100 && gid <= 60000 {
			return gid
		}
	}
	return 100
}

func (s *apiServer) brokerExecute(ctx context.Context, request privileged.Request) error {
	// brokerExec is a test seam; production leaves it nil so requests go to
	// the real privileged broker over the unix socket.
	if s.brokerExec != nil {
		return s.brokerExec(ctx, request)
	}
	result, err := (privileged.Client{Socket: envOr("LUMONAS_PRIVD_SOCKET", "/run/lumonas/privd.sock")}).Execute(ctx, request)
	if err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

func (s *apiServer) ensureFileIdentitySystemUser(principal identity.Principal, state map[string]any) error {
	if principal.UID == nil {
		return errors.New("principal has no allocated uid")
	}
	request := privileged.Request{
		Operation:      "identity.system-user.ensure",
		OperationID:    newID("identity"),
		PlanHash:       "identity-" + principal.Name + "-" + strconv.FormatInt(s.currentGeneration(), 10),
		RequestedState: state,
		ExpiresAt:      time.Now().UTC().Add(5 * time.Minute),
		Confirmed:      true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return s.brokerExecute(ctx, request)
}

func (s *apiServer) ensureFileIdentitySambaUser(name string, state map[string]any) error {
	request := privileged.Request{
		Operation:      "samba.user.ensure",
		OperationID:    newID("samba"),
		PlanHash:       "samba-" + name + "-" + strconv.FormatInt(s.currentGeneration(), 10),
		RequestedState: state,
		ExpiresAt:      time.Now().UTC().Add(5 * time.Minute),
		Confirmed:      true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return s.brokerExecute(ctx, request)
}

// provisionNewFileIdentity creates the OS account (and the Samba account for
// file users) behind a freshly created principal. The store record remains
// the source of truth; callers roll it back when provisioning fails.
func (s *apiServer) provisionNewFileIdentity(principal identity.Principal, password string) error {
	if err := s.ensureFileIdentitySystemUser(principal, map[string]any{"name": principal.Name, "uid": int(*principal.UID), "gid": fileIdentityGID(), "create": true}); err != nil {
		return fmt.Errorf("system account: %w", err)
	}
	if principal.Kind != identity.KindUser {
		return nil
	}
	if err := s.ensureFileIdentitySambaUser(principal.Name, map[string]any{"name": principal.Name, "create": true, "password": password}); err != nil {
		return fmt.Errorf("Samba account: %w", err)
	}
	return nil
}

// synchronizeFileIdentityState propagates enable/disable transitions to the
// OS and Samba accounts. The store commit happens first; failures leave the
// authoritative record intact and are surfaced so the caller can retry.
func (s *apiServer) synchronizeFileIdentityState(principal identity.Principal, enabled bool) error {
	systemState := map[string]any{"name": principal.Name, "disabled": !enabled}
	if enabled {
		systemState = map[string]any{"name": principal.Name, "enable": true}
	}
	if err := s.ensureFileIdentitySystemUser(principal, systemState); err != nil {
		return fmt.Errorf("system account: %w", err)
	}
	if principal.Kind != identity.KindUser {
		return nil
	}
	sambaState := map[string]any{"name": principal.Name, "disabled": !enabled}
	if enabled {
		sambaState = map[string]any{"name": principal.Name, "enable": true}
	}
	if err := s.ensureFileIdentitySambaUser(principal.Name, sambaState); err != nil {
		return fmt.Errorf("Samba account: %w", err)
	}
	return nil
}

// disableFileIdentityAccounts best-effort locks the OS and Samba accounts
// after the principal was deleted. Leftover locked accounts are harmless and
// never fail the delete.
func (s *apiServer) disableFileIdentityAccounts(principal identity.Principal) {
	if err := s.synchronizeFileIdentityState(principal, false); err != nil && s.log != nil {
		s.log.Warn("deleted file identity could not be locked", "principal", principal.Name, "error", err)
	}
}
