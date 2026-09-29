package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/mail"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const tlsCertificateSourceMaxBytes = 1 << 20

var tlsCertificateIDPattern = regexp.MustCompile(`^tls-[a-f0-9]{16}$`)
var tlsACMEDomainPattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

func applyTLSCertificate(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	if !validOperationID(req.OperationID) {
		return response{Error: "operationId is required"}
	}
	certificateID := requestedString(req.RequestedState, "certificateId")
	if !tlsCertificateIDPattern.MatchString(certificateID) {
		return response{Error: "certificate import identifier is invalid"}
	}
	sourceDir := envOr("LUMONAS_GENERATED_DIR", "/var/lib/lumonas/generated")
	certificatePath := filepath.Join(sourceDir, certificateID+".crt")
	keyPath := filepath.Join(sourceDir, certificateID+".key")
	defer os.Remove(certificatePath)
	defer os.Remove(keyPath)
	certificatePEM, err := readTLSImportFile(certificatePath)
	if err != nil {
		return response{Error: "staged certificate is missing or invalid"}
	}
	privateKeyPEM, err := readTLSImportFile(keyPath)
	if err != nil {
		return response{Error: "staged private key is missing or invalid"}
	}
	if _, err := validateTLSImportPair(certificatePEM, privateKeyPEM, time.Now()); err != nil {
		return response{Error: err.Error()}
	}
	group, err := user.LookupGroup("lumonas")
	if err != nil {
		return response{Error: "LumoNAS service group is unavailable"}
	}
	groupID, err := strconv.Atoi(group.Gid)
	if err != nil {
		return response{Error: "LumoNAS service group is invalid"}
	}
	if err := installTLSCertificateFiles(certificatePEM, privateKeyPEM, "/etc/lumonas/tls", groupID, run); err != nil {
		return response{Error: err.Error()}
	}
	return response{OK: true, Data: map[string]string{"state": "installed", "service": "lumonas-web.service"}}
}

func configureTLSACME(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	if !validOperationID(req.OperationID) {
		return response{Error: "operationId is required"}
	}
	domain := strings.TrimSpace(requestedString(req.RequestedState, "domain"))
	email := strings.TrimSpace(requestedString(req.RequestedState, "email"))
	if len(domain) > 253 || !tlsACMEDomainPattern.MatchString(domain) {
		return response{Error: "enter a fully qualified DNS name without a wildcard or URL scheme"}
	}
	parsedEmail, err := mail.ParseAddress(email)
	if err != nil || parsedEmail.Address != email || len(email) > 254 {
		return response{Error: "enter a valid email address for certificate notices"}
	}
	if !requestedBool(req.RequestedState, "agreeToTerms") {
		return response{Error: "agree to the Let's Encrypt terms before continuing"}
	}
	certbotDir := envOr("LUMONAS_CERTBOT_DIR", "/etc/letsencrypt")
	renewalProfile := filepath.Join(certbotDir, "renewal", "lumonas-web.conf")
	if _, err := os.Lstat(renewalProfile); err == nil {
		return response{Error: "Let's Encrypt renewal is already configured"}
	} else if !errors.Is(err, os.ErrNotExist) {
		return response{Error: "Let's Encrypt renewal configuration could not be checked"}
	}
	if err := installACMEDeployHook(filepath.Join(certbotDir, "renewal-hooks", "deploy", "lumonas-web")); err != nil {
		return response{Error: "automatic renewal hook could not be installed"}
	}
	args := []string{"certonly", "--standalone", "--non-interactive", "--agree-tos", "--no-eff-email", "--email", email, "--domains", domain, "--cert-name", "lumonas-web"}
	if _, err := run("certbot", args...); err != nil {
		_ = os.Remove(filepath.Join(certbotDir, "renewal-hooks", "deploy", "lumonas-web"))
		return response{Error: "Let's Encrypt could not issue the certificate; verify public DNS and inbound TCP port 80"}
	}
	if err := activateACMECertificateFrom(certbotDir, run); err != nil {
		return response{Error: "certificate was issued but could not be activated: " + err.Error()}
	}
	if _, err := run("systemctl", "enable", "--now", "certbot.timer"); err != nil {
		return response{Error: "certificate is active, but the automatic renewal timer could not be enabled"}
	}
	return response{OK: true, Data: map[string]string{"state": "configured", "domain": domain, "renewal": "enabled"}}
}

func disableTLSACME(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	if !validOperationID(req.OperationID) {
		return response{Error: "operationId is required"}
	}
	certbotDir := envOr("LUMONAS_CERTBOT_DIR", "/etc/letsencrypt")
	if _, err := os.Lstat(filepath.Join(certbotDir, "renewal", "lumonas-web.conf")); errors.Is(err, os.ErrNotExist) {
		return response{Error: "Let's Encrypt renewal is not configured"}
	} else if err != nil {
		return response{Error: "Let's Encrypt renewal configuration could not be checked"}
	}
	if _, err := run("certbot", "delete", "--non-interactive", "--cert-name", "lumonas-web"); err != nil {
		return response{Error: "Let's Encrypt certificate profile could not be removed"}
	}
	_ = os.Remove(filepath.Join(certbotDir, "renewal-hooks", "deploy", "lumonas-web"))
	return response{OK: true, Data: map[string]string{"renewal": "disabled"}}
}

func installACMEDeployHook(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	contents := []byte("#!/bin/sh\nset -eu\n[ \"${RENEWED_LINEAGE:-}\" = \"/etc/letsencrypt/live/lumonas-web\" ] || exit 0\nexec /usr/lib/lumonas/lumonas-privd --activate-acme-certificate\n")
	return replaceTLSFile(path, contents, 0o755, 0)
}

func activateACMECertificate(run command) error {
	return activateACMECertificateFrom(envOr("LUMONAS_CERTBOT_DIR", "/etc/letsencrypt"), run)
}

func activateACMECertificateFrom(certbotDir string, run command) error {
	lineage := filepath.Join(certbotDir, "live", "lumonas-web")
	certificatePEM, err := os.ReadFile(filepath.Join(lineage, "fullchain.pem"))
	if err != nil {
		return errors.New("Let's Encrypt certificate files are unavailable")
	}
	privateKeyPEM, err := os.ReadFile(filepath.Join(lineage, "privkey.pem"))
	if err != nil {
		return errors.New("Let's Encrypt private key is unavailable")
	}
	if _, err := validateTLSImportPair(certificatePEM, privateKeyPEM, time.Now()); err != nil {
		return err
	}
	group, err := user.LookupGroup("lumonas")
	if err != nil {
		return errors.New("LumoNAS service group is unavailable")
	}
	groupID, err := strconv.Atoi(group.Gid)
	if err != nil {
		return errors.New("LumoNAS service group is invalid")
	}
	return installTLSCertificateFiles(certificatePEM, privateKeyPEM, "/etc/lumonas/tls", groupID, run)
}

func readTLSImportFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > tlsCertificateSourceMaxBytes || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("staged TLS material has unsafe type, size, or permissions")
	}
	return os.ReadFile(path)
}

func validateTLSImportPair(certificatePEM, privateKeyPEM []byte, now time.Time) (*x509.Certificate, error) {
	if _, err := tls.X509KeyPair(certificatePEM, privateKeyPEM); err != nil {
		return nil, errors.New("certificate and private key are invalid or do not match")
	}
	block, _ := pem.Decode(certificatePEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("certificate file must contain a PEM certificate chain")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, errors.New("the PEM certificate could not be parsed")
	}
	if now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
		return nil, errors.New("certificate is not currently valid")
	}
	return certificate, nil
}

type tlsFileBackup struct {
	contents []byte
	mode     os.FileMode
	present  bool
}

func installTLSCertificateFiles(certificatePEM, privateKeyPEM []byte, targetDir string, groupID int, run command) error {
	if run == nil {
		return errors.New("service restart runner is unavailable")
	}
	if err := os.MkdirAll(targetDir, 0o750); err != nil {
		return errors.New("TLS certificate directory could not be prepared")
	}
	certificatePath := filepath.Join(targetDir, "server.crt")
	keyPath := filepath.Join(targetDir, "server.key")
	oldCertificate, err := backupTLSFile(certificatePath)
	if err != nil {
		return errors.New("existing certificate could not be preserved")
	}
	oldKey, err := backupTLSFile(keyPath)
	if err != nil {
		return errors.New("existing private key could not be preserved")
	}
	restore := func() {
		_ = restoreTLSFile(certificatePath, oldCertificate, groupID)
		_ = restoreTLSFile(keyPath, oldKey, groupID)
	}
	if err := replaceTLSFile(certificatePath, certificatePEM, 0o644, groupID); err != nil {
		return errors.New("certificate could not be installed")
	}
	if err := replaceTLSFile(keyPath, privateKeyPEM, 0o640, groupID); err != nil {
		restore()
		return errors.New("private key could not be installed; previous certificate was restored")
	}
	if _, err := run("systemctl", "reload-or-restart", "lumonas-web.service"); err != nil {
		restore()
		_, _ = run("systemctl", "reload-or-restart", "lumonas-web.service")
		return errors.New("web service could not load the new certificate; previous certificate was restored")
	}
	return nil
}

func backupTLSFile(path string) (tlsFileBackup, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return tlsFileBackup{}, nil
	}
	if err != nil {
		return tlsFileBackup{}, err
	}
	if !info.Mode().IsRegular() {
		return tlsFileBackup{}, errors.New("TLS target is not a regular file")
	}
	contents, err := os.ReadFile(path)
	return tlsFileBackup{contents: contents, mode: info.Mode().Perm(), present: err == nil}, err
}

func restoreTLSFile(path string, backup tlsFileBackup, groupID int) error {
	if !backup.present {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	return replaceTLSFile(path, backup.contents, backup.mode, groupID)
}

func replaceTLSFile(path string, contents []byte, mode os.FileMode, groupID int) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".lumonas-tls-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if groupID >= 0 {
		if err := temporary.Chown(0, groupID); err != nil {
			_ = temporary.Close()
			return err
		}
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
