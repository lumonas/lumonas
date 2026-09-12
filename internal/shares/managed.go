package shares

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
)

type Protocol struct {
	Name     string         `json:"name"`
	Settings map[string]any `json:"settings,omitempty"`
}

type AccessRule struct {
	PrincipalID   string `json:"principalId"`
	PrincipalName string `json:"principalName"`
	Level         string `json:"level"`
}

type ManagedShare struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Path        string       `json:"path"`
	Description string       `json:"description,omitempty"`
	Enabled     bool         `json:"enabled"`
	Guest       bool         `json:"guest"`
	Protocols   []Protocol   `json:"protocols"`
	Access      []AccessRule `json:"access"`
}

func (s ManagedShare) Legacy() Share {
	protocols := make([]string, 0, len(s.Protocols))
	access := make(map[string]string, len(s.Access))
	for _, protocol := range s.Protocols {
		protocols = append(protocols, protocol.Name)
	}
	for _, rule := range s.Access {
		name := rule.PrincipalName
		if name == "" {
			name = rule.PrincipalID
		}
		access[name] = rule.Level
	}
	return Share{ID: s.ID, Name: s.Name, Path: s.Path, Description: s.Description, Enabled: s.Enabled, Protocols: protocols, Access: access, Guest: s.Guest}
}

func (s ManagedShare) Validate() error {
	if err := Validate(s.Legacy()); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, protocol := range s.Protocols {
		if seen[protocol.Name] {
			return fmt.Errorf("protocol %q is listed more than once", protocol.Name)
		}
		seen[protocol.Name] = true
		if err := ValidateProtocolSettings(protocol.Name, protocol.Settings); err != nil {
			return err
		}
	}
	for _, rule := range s.Access {
		if strings.TrimSpace(rule.PrincipalID) == "" && strings.TrimSpace(rule.PrincipalName) == "" {
			return errors.New("access rule requires a principal")
		}
		switch rule.Level {
		case "none", "read", "write":
		default:
			return fmt.Errorf("unsupported access level %q", rule.Level)
		}
	}
	return nil
}

func ValidateProtocolSettings(protocol string, settings map[string]any) error {
	if settings == nil {
		return nil
	}
	switch protocol {
	case "nfs":
		if value, ok := settings["allowedNetworks"]; ok {
			networks, ok := value.([]any)
			if !ok {
				return errors.New("nfs allowedNetworks must be an array")
			}
			for _, item := range networks {
				value, ok := item.(string)
				if !ok || parseCIDR(value) == nil {
					return fmt.Errorf("nfs network %q is not a valid CIDR", item)
				}
			}
		}
		if value, ok := settings["rootSquash"]; ok {
			if _, ok := value.(bool); !ok {
				return errors.New("nfs rootSquash must be boolean")
			}
		}
	case "ftp", "ftps":
		if value, ok := settings["passivePortStart"]; ok {
			if !validPort(value) {
				return fmt.Errorf("%s passivePortStart must be between 1024 and 65535", protocol)
			}
		}
		if value, ok := settings["passivePortEnd"]; ok {
			if !validPort(value) {
				return fmt.Errorf("%s passivePortEnd must be between 1024 and 65535", protocol)
			}
		}
	}
	return nil
}

func parseCIDR(value string) *net.IPNet {
	_, network, err := net.ParseCIDR(strings.TrimSpace(value))
	return networkOrNil(network, err)
}

func networkOrNil(network *net.IPNet, err error) *net.IPNet {
	if err != nil {
		return nil
	}
	return network
}

func validPort(value any) bool {
	var port int
	switch value := value.(type) {
	case float64:
		port = int(value)
	case int:
		port = value
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return false
		}
		port = int(parsed)
	default:
		return false
	}
	return port >= 1024 && port <= 65535
}
