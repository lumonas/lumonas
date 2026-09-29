package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/network"
	"github.com/lumonas/lumonas/internal/privileged"
)

const tlsCertificateImportMaxBytes = 1 << 20

type tlsCertificateImportRequest struct {
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"privateKey"`
}

type tlsCertificateStatus struct {
	State                  string    `json:"state"`
	Configured             bool      `json:"configured"`
	Issuer                 string    `json:"issuer,omitempty"`
	Subject                string    `json:"subject,omitempty"`
	DNSNames               []string  `json:"dnsNames,omitempty"`
	NotBefore              time.Time `json:"notBefore,omitempty"`
	NotAfter               time.Time `json:"notAfter,omitempty"`
	DaysRemaining          int       `json:"daysRemaining,omitempty"`
	Detail                 string    `json:"detail,omitempty"`
	ManagedBy              string    `json:"managedBy,omitempty"`
	AutoRenewalConfigured  bool      `json:"autoRenewalConfigured"`
	ACMEFirewallConfigured bool      `json:"acmeFirewallConfigured"`
}

type tlsACMERequest struct {
	Domain       string `json:"domain"`
	Email        string `json:"email"`
	AgreeToTerms bool   `json:"agreeToTerms"`
	AllowPort80  bool   `json:"allowPort80"`
}

var tlsDomainPattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

func acmeRenewalProfileExists() bool {
	path := filepath.Join(envOr("LUMONAS_CERTBOT_DIR", "/etc/letsencrypt"), "renewal", "lumonas-web.conf")
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (s *apiServer) acmeFirewallConfigured() bool {
	bindings, err := s.store.ListNetworkBindings()
	if err != nil {
		return false
	}
	for _, binding := range bindings {
		if binding.Service == "acme-http" && binding.Port == 80 && binding.Enabled {
			return true
		}
	}
	return false
}

func (s *apiServer) setACMEFirewall(enabled bool) error {
	bindings, err := s.store.ListNetworkBindings()
	if err != nil {
		return err
	}
	policy, err := s.store.NetworkFirewallPolicy()
	if err != nil {
		return err
	}
	if policy.Services == nil {
		policy.Services = make(map[string]network.FirewallService)
	}
	services := make(map[string]network.FirewallService, len(policy.Services))
	for name, service := range policy.Services {
		services[name] = service
	}
	policy.Services = services
	next := make([]network.Binding, 0, len(bindings)+1)
	changed := false
	for _, binding := range bindings {
		if binding.Service == "acme-http" {
			changed = true
			continue
		}
		next = append(next, binding)
	}
	if enabled {
		next = append(next, network.Binding{Service: "acme-http", Address: "0.0.0.0", Port: 80, Enabled: true, Scopes: []string{"lan"}})
		policy.Services["acme-http"] = network.FirewallService{LAN: true}
		changed = true
	} else if _, exists := policy.Services["acme-http"]; exists {
		delete(policy.Services, "acme-http")
		changed = true
	}
	if !changed {
		return nil
	}
	rules, err := network.RenderNftables(policy, next)
	if err != nil {
		return err
	}
	policy.GeneratedRules = rules
	rollbackFirewall, err := s.activateFirewallRules(context.Background(), rules)
	if err != nil {
		return err
	}
	previous, err := s.store.ListNetworkBindings()
	if err != nil {
		rollbackFirewall()
		return err
	}
	if _, err := s.store.ReplaceNetworkBindings(next); err != nil {
		rollbackFirewall()
		return err
	}
	if _, err := s.store.SaveNetworkFirewallPolicy(policy); err != nil {
		_, _ = s.store.ReplaceNetworkBindings(previous)
		rollbackFirewall()
		return err
	}
	s.advanceGeneration("security.certificate.acme.firewall")
	s.publish("network.firewall.updated", "warning", &model.ResourceRef{Type: "service-binding", ID: "acme-http"}, map[string]any{"port": 80, "enabled": enabled})
	return nil
}

func (s *apiServer) tlsCertificateStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	path := os.Getenv("LUMONAS_WEB_TLS_CERT")
	keyPath := os.Getenv("LUMONAS_WEB_TLS_KEY")
	if path == "" && keyPath == "" {
		writeJSON(w, http.StatusOK, tlsCertificateStatus{State: "disabled", Configured: false, ManagedBy: "none", AutoRenewalConfigured: acmeRenewalProfileExists(), ACMEFirewallConfigured: s.acmeFirewallConfigured(), Detail: "The web service does not have a certificate path configured."})
		return
	}
	if path == "" || keyPath == "" {
		writeJSON(w, http.StatusOK, tlsCertificateStatus{State: "incomplete", Configured: false, ManagedBy: "none", AutoRenewalConfigured: acmeRenewalProfileExists(), ACMEFirewallConfigured: s.acmeFirewallConfigured(), Detail: "The web service certificate and key paths must both be configured."})
		return
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		writeJSON(w, http.StatusOK, tlsCertificateStatus{State: "missing", Configured: true, Detail: "The configured certificate file could not be read."})
		return
	}
	block, _ := pem.Decode(contents)
	if block == nil || block.Type != "CERTIFICATE" {
		writeJSON(w, http.StatusOK, tlsCertificateStatus{State: "invalid", Configured: true, Detail: "The configured file does not contain a PEM certificate."})
		return
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		writeJSON(w, http.StatusOK, tlsCertificateStatus{State: "invalid", Configured: true, Detail: "The configured PEM certificate could not be parsed."})
		return
	}
	if _, err := tls.LoadX509KeyPair(path, keyPath); err != nil {
		writeJSON(w, http.StatusOK, tlsCertificateStatus{State: "invalid", Configured: true, Detail: "The configured certificate and private key are invalid or do not match."})
		return
	}
	now := time.Now().UTC()
	days := int(math.Floor(certificate.NotAfter.Sub(now).Hours() / 24))
	state, detail := "valid", "Certificate is currently within its validity period."
	if now.Before(certificate.NotBefore) {
		state, detail = "not-yet-valid", "Certificate validity begins in the future."
	} else if days < 0 {
		state, detail = "expired", "Certificate has expired."
	} else if days <= 30 {
		state, detail = "expiring", "Certificate expires within 30 days."
	}
	issuer, subject := certificate.Issuer.CommonName, certificate.Subject.CommonName
	if issuer == "" {
		issuer = certificate.Issuer.String()
	}
	if subject == "" {
		subject = certificate.Subject.String()
	}
	managedBy := "manual"
	autoRenewal := acmeRenewalProfileExists()
	if autoRenewal {
		managedBy = "letsencrypt"
	}
	writeJSON(w, http.StatusOK, tlsCertificateStatus{State: state, Configured: true, Issuer: issuer, Subject: subject, DNSNames: certificate.DNSNames, NotBefore: certificate.NotBefore.UTC(), NotAfter: certificate.NotAfter.UTC(), DaysRemaining: days, Detail: detail, ManagedBy: managedBy, AutoRenewalConfigured: autoRenewal, ACMEFirewallConfigured: s.acmeFirewallConfigured()})
}

func (s *apiServer) importTLSCertificate(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input tlsCertificateImportRequest
	r.Body = http.MaxBytesReader(w, r.Body, 2*tlsCertificateImportMaxBytes+4096)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid certificate import payload"})
		return
	}
	certificate, key := []byte(strings.TrimSpace(input.Certificate)+"\n"), []byte(strings.TrimSpace(input.PrivateKey)+"\n")
	parsed, err := validateTLSCertificatePair(certificate, key, time.Now())
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if len(certificate) > tlsCertificateImportMaxBytes || len(key) > tlsCertificateImportMaxBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "certificate and key must each be 1 MiB or smaller"})
		return
	}
	if acmeRenewalProfileExists() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "disable Let's Encrypt renewal before importing a manually managed certificate"})
		return
	}
	if s.acmeFirewallConfigured() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "remove the ACME port 80 firewall binding before importing a manually managed certificate"})
		return
	}
	certificateID := newID("tls")
	generatedDir := envOr("LUMONAS_GENERATED_DIR", "/var/lib/lumonas/generated")
	certificatePath := filepath.Join(generatedDir, certificateID+".crt")
	keyPath := filepath.Join(generatedDir, certificateID+".key")
	if err := stageTLSFile(certificatePath, certificate); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not stage certificate for privileged installation"})
		return
	}
	if err := stageTLSFile(keyPath, key); err != nil {
		_ = os.Remove(certificatePath)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not stage private key for privileged installation"})
		return
	}
	now := time.Now().UTC()
	operationID := newID("tls-install")
	job := model.Job{ID: newID("job"), CorrelationID: requestCorrelationID(r), OperationID: operationID, PlanHash: operationID, Actor: actor, Generation: s.currentGeneration(), Type: "security.certificate.install", Title: "Install TLS certificate", ResourceID: "tls-certificate", State: "queued", Stage: "Waiting to replace the web certificate", CreatedAt: now}
	if err := s.store.SaveJob(job); err != nil {
		_ = os.Remove(certificatePath)
		_ = os.Remove(keyPath)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not queue certificate installation"})
		return
	}
	s.recordRequestAudit(r, actor, "security.certificate.install.queued", job.ID, map[string]any{"subject": parsed.Subject.String(), "notAfter": parsed.NotAfter.UTC(), "jobId": job.ID})
	s.publishActor(actor, "security.certificate.install.queued", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	go s.runTLSCertificateInstall(job, "tls.certificate.install", map[string]any{"certificateId": certificateID}, certificatePath, keyPath)
	writeJSON(w, http.StatusAccepted, job)
}

func (s *apiServer) configureTLSACME(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input tlsACMERequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ACME configuration"})
		return
	}
	input.Domain, input.Email = strings.TrimSpace(strings.ToLower(input.Domain)), strings.TrimSpace(input.Email)
	if err := validateTLSACMERequest(input); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if acmeRenewalProfileExists() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Let's Encrypt renewal is already configured; disable it before changing the domain"})
		return
	}
	if !input.AllowPort80 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "authorize the LumoNAS firewall to allow TCP port 80 for HTTP-01 validation and renewal"})
		return
	}
	if err := s.setACMEFirewall(true); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "could not allow the ACME HTTP challenge through the local firewall: " + err.Error()})
		return
	}
	operationID := newID("tls-acme")
	now := time.Now().UTC()
	job := model.Job{ID: newID("job"), CorrelationID: requestCorrelationID(r), OperationID: operationID, PlanHash: operationID, Actor: actor, Generation: s.currentGeneration(), Type: "security.certificate.acme", Title: "Configure Let's Encrypt", ResourceID: "tls-certificate", State: "queued", Stage: "Waiting to request a public certificate", CreatedAt: now}
	if err := s.store.SaveJob(job); err != nil {
		_ = s.setACMEFirewall(false)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not queue ACME configuration"})
		return
	}
	s.recordRequestAudit(r, actor, "security.certificate.acme.queued", job.ID, map[string]any{"domain": input.Domain, "jobId": job.ID})
	s.publishActor(actor, "security.certificate.acme.queued", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job, "domain": input.Domain, "port80Allowed": true})
	go s.runTLSCertificateInstall(job, "tls.acme.configure", map[string]any{"domain": input.Domain, "email": input.Email, "agreeToTerms": true}, "", "")
	writeJSON(w, http.StatusAccepted, job)
}

func validateTLSACMERequest(input tlsACMERequest) error {
	if !input.AgreeToTerms {
		return errors.New("agree to the Let's Encrypt terms before continuing")
	}
	if !input.AllowPort80 {
		return errors.New("authorize the LumoNAS firewall to allow TCP port 80 for HTTP-01 validation and renewal")
	}
	if len(input.Domain) > 253 || !tlsDomainPattern.MatchString(input.Domain) {
		return errors.New("enter a fully qualified DNS name without a wildcard or URL scheme")
	}
	parsedEmail, err := mail.ParseAddress(input.Email)
	if err != nil || parsedEmail.Address != input.Email || len(input.Email) > 254 {
		return errors.New("enter a valid email address for certificate notices")
	}
	return nil
}

func (s *apiServer) disableTLSACME(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	profileExists := acmeRenewalProfileExists()
	firewallExists := s.acmeFirewallConfigured()
	if !profileExists && !firewallExists {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Let's Encrypt renewal is not configured"})
		return
	}
	operationID := newID("tls-acme-disable")
	if profileExists {
		request := privileged.Request{Operation: "tls.acme.disable", OperationID: operationID, PlanHash: operationID, Confirmed: true, ExpiresAt: time.Now().UTC().Add(2 * time.Minute)}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		err := s.brokerExecute(ctx, request)
		cancel()
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "could not disable certificate renewal: " + err.Error()})
			return
		}
	}
	if firewallExists {
		if err := s.setACMEFirewall(false); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "certificate renewal is disabled but the local port 80 firewall rule could not be removed: " + err.Error()})
			return
		}
	}
	s.recordRequestAudit(r, actor, "security.certificate.acme.disabled", "tls-certificate", map[string]any{"operationId": operationID})
	s.publishActor(actor, "security.certificate.acme.disabled", "warning", &model.ResourceRef{Type: "tls-certificate", ID: "tls-certificate"}, nil)
	writeJSON(w, http.StatusOK, map[string]any{"autoRenewalConfigured": false, "acmeFirewallConfigured": false, "managedBy": "manual"})
}

func validateTLSCertificatePair(certificatePEM, privateKeyPEM []byte, now time.Time) (*x509.Certificate, error) {
	if len(certificatePEM) == 0 || len(privateKeyPEM) == 0 {
		return nil, errors.New("both a PEM certificate and private key are required")
	}
	if len(certificatePEM) > tlsCertificateImportMaxBytes || len(privateKeyPEM) > tlsCertificateImportMaxBytes {
		return nil, errors.New("certificate and key must each be 1 MiB or smaller")
	}
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

func stageTLSFile(path string, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(contents)
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

func (s *apiServer) runTLSCertificateInstall(job model.Job, operation string, requestedState map[string]any, certificatePath, keyPath string) {
	if certificatePath != "" {
		defer os.Remove(certificatePath)
	}
	if keyPath != "" {
		defer os.Remove(keyPath)
	}
	started := time.Now().UTC()
	progress := 20.0
	job.State, job.Stage, job.StartedAt, job.Progress = "running", "Validating and installing certificate pair", &started, &progress
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	request := privileged.Request{Operation: operation, OperationID: job.OperationID, PlanHash: job.PlanHash, RequestedState: requestedState, ExpiresAt: time.Now().UTC().Add(10 * time.Minute), Confirmed: true}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	err := s.brokerExecute(ctx, request)
	cancel()
	finished := time.Now().UTC()
	progress = 100
	job.FinishedAt, job.Progress = &finished, &progress
	if err != nil {
		job.State, job.Stage, job.Error = "failed", "Certificate installation failed", err.Error()
		if operation == "tls.acme.configure" && !acmeRenewalProfileExists() {
			if cleanupErr := s.setACMEFirewall(false); cleanupErr != nil {
				job.Error += "; could not remove the ACME firewall rule: " + cleanupErr.Error()
			}
		}
		_ = s.store.SaveJob(job)
		s.publish("job.state_changed", "warning", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
		return
	}
	job.State, job.Stage = "successful", "Certificate installed and web service restarted"
	if operation == "tls.acme.configure" {
		job.Stage = "Let's Encrypt certificate installed and automatic renewal enabled"
	}
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	action := "security.certificate.install"
	metadata := map[string]any{"operationId": job.OperationID}
	if operation == "tls.acme.configure" {
		action = "security.certificate.acme.configured"
		metadata["domain"] = requestedState["domain"]
	}
	s.recordIdentityAudit(job.Actor, action, job.ID, metadata)
}
