package docker

import (
	"encoding/json"
	"errors"
	"os"
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
