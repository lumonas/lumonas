package docker

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	CatalogTrustVerified   = "verified"
	CatalogTrustUnverified = "unverified"
	CatalogTrustInvalid    = "invalid"
)

type CatalogApp struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	Category        string             `json:"category"`
	Tagline         string             `json:"tagline"`
	Description     string             `json:"description"`
	Accent          string             `json:"accent"`
	Upstream        string             `json:"upstream"`
	Image           string             `json:"image"`
	Ports           []int              `json:"ports"`
	Form            []CatalogFormField `json:"form"`
	Popular         bool               `json:"popular,omitempty"`
	Recovery        *RecoveryContract  `json:"recovery,omitempty"`
	AppdataPaths    []string           `json:"appdataPaths,omitempty"`
	DBDumpContainer string             `json:"dbDumpContainer,omitempty"`
	TrustStatus     string             `json:"trustStatus,omitempty"`
	TrustMessage    string             `json:"trustMessage,omitempty"`
}
type CatalogFormField struct {
	ID              string `json:"id"`
	Label           string `json:"label"`
	Type            string `json:"type"`
	Required        bool   `json:"required,omitempty"`
	DefaultValue    string `json:"defaultValue,omitempty"`
	Description     string `json:"description,omitempty"`
	ContainerPath   string `json:"containerPath,omitempty"`
	DefaultResource string `json:"defaultResource,omitempty"`
}

func BuildCompose(app CatalogApp, name string, values map[string]string, storage []StorageMapping) (string, error) {
	if app.Image == "" || name == "" || !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`).MatchString(name) {
		return "", errors.New("catalog app or stack name is invalid")
	}
	port := 0
	for _, field := range app.Form {
		if field.Type == "port" {
			port, _ = strconv.Atoi(values[field.ID])
			if port == 0 {
				port, _ = strconv.Atoi(field.DefaultValue)
			}
			break
		}
	}
	if port == 0 && len(app.Ports) > 0 {
		port = app.Ports[0]
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "services:\n  %s:\n    image: %s\n    restart: unless-stopped\n", name, app.Image)
	if port > 0 {
		fmt.Fprintf(&builder, "    ports:\n      - \"%d:%d\"\n", port, app.Ports[0])
	}
	if len(values) > 0 {
		builder.WriteString("    environment:\n")
		for _, field := range app.Form {
			value, ok := values[field.ID]
			if !ok || value == "" || field.Type == "port" || field.Type == "storage_ref" {
				continue
			}
			if field.Type == "secret" {
				value = "${" + field.ID + "}"
			}
			fmt.Fprintf(&builder, "      %s: %q\n", field.ID, strings.ReplaceAll(value, "\"", "\\\""))
		}
	}
	if len(storage) > 0 {
		builder.WriteString("    volumes:\n")
		for _, mapping := range storage {
			if !strings.HasPrefix(mapping.ContainerPath, "/") {
				return "", fmt.Errorf("container path %q must be absolute", mapping.ContainerPath)
			}
			branch := mapping.ResourceID
			if !strings.HasPrefix(branch, "/") {
				branch = "/srv/pools/main/" + strings.ReplaceAll(branch, "..", "")
			}
			fmt.Fprintf(&builder, "      - %s:%s\n", branch, mapping.ContainerPath)
		}
	}
	return builder.String(), nil
}

func LoadCatalog(path string) ([]CatalogApp, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []CatalogApp{}, nil
	}
	if err != nil {
		return nil, err
	}
	var apps []CatalogApp
	if err := json.Unmarshal(data, &apps); err != nil {
		return nil, err
	}
	status, message := catalogTrust(path, data, os.Getenv("LUMONAS_CATALOG_PUBLIC_KEY"))
	for index := range apps {
		apps[index].TrustStatus = status
		apps[index].TrustMessage = message
	}
	return apps, nil
}

// VerifyCatalogSignature checks a detached standard-base64 Ed25519 signature
// over the exact catalog bytes using a standard-base64 public key.
func VerifyCatalogSignature(data []byte, signature, publicKey string) bool {
	key, keyErr := base64.StdEncoding.DecodeString(strings.TrimSpace(publicKey))
	sig, sigErr := base64.StdEncoding.DecodeString(strings.TrimSpace(signature))
	return keyErr == nil && sigErr == nil && len(key) == ed25519.PublicKeySize && ed25519.Verify(ed25519.PublicKey(key), data, sig)
}

func catalogTrust(path string, data []byte, publicKey string) (string, string) {
	signatureBytes, err := os.ReadFile(path + ".sig")
	if errors.Is(err, os.ErrNotExist) {
		return CatalogTrustUnverified, "Catalog signature is missing"
	}
	if err != nil {
		return CatalogTrustInvalid, "Catalog signature could not be read"
	}
	if strings.TrimSpace(publicKey) == "" {
		return CatalogTrustUnverified, "Catalog signature exists but no trusted public key is configured"
	}
	if !VerifyCatalogSignature(data, string(signatureBytes), publicKey) {
		return CatalogTrustInvalid, "Catalog signature does not match the configured trusted key"
	}
	return CatalogTrustVerified, "Catalog signature verified"
}

// CatalogInstallAllowed rejects known-tampered catalogs and optionally makes
// signatures mandatory for managed deployments. Unsigned development builds
// remain usable when the strict policy is disabled.
func CatalogInstallAllowed(app CatalogApp, requireSigned bool) bool {
	if app.TrustStatus == CatalogTrustInvalid {
		return false
	}
	return !requireSigned || app.TrustStatus == CatalogTrustVerified
}

// EnrichStack applies catalog-owned recovery metadata without changing the
// Compose source of truth. Unknown/imported stacks get the conservative
// default recovery contract so the API never implies that their mutable state
// is fully protected.
func EnrichStack(stack Stack, catalog []CatalogApp) Stack {
	for index := range catalog {
		app := &catalog[index]
		if !catalogImageMatches(app.Image, stack.Images) {
			continue
		}
		stack.CatalogID = app.ID
		stack.Category = app.Category
		contract := ContractFromCatalog(app.AppdataPaths, app.DBDumpContainer)
		stack.Recovery = MergeRecoveryContracts(contract, app.Recovery)
		stack.RecoveryCoverage = StackRecoveryCoverage(stack.Recovery)
		return stack
	}
	stack.Recovery = DefaultRecoveryContract()
	stack.RecoveryCoverage = StackRecoveryCoverage(stack.Recovery)
	return stack
}

func catalogImageMatches(catalogImage string, images []string) bool {
	catalogImage = strings.TrimSpace(catalogImage)
	if catalogImage == "" {
		return false
	}
	for _, image := range images {
		if strings.TrimSpace(image) == catalogImage {
			return true
		}
	}
	return false
}
