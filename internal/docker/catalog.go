package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type CatalogApp struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Category    string             `json:"category"`
	Tagline     string             `json:"tagline"`
	Description string             `json:"description"`
	Accent      string             `json:"accent"`
	Upstream    string             `json:"upstream"`
	Image       string             `json:"image"`
	Ports       []int              `json:"ports"`
	Form        []CatalogFormField `json:"form"`
	Popular     bool               `json:"popular,omitempty"`
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
	return apps, nil
}
