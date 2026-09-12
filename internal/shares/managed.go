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
	hasSMB := false
	access := make(map[string]string, len(s.Access))
	for _, protocol := range s.Protocols {
		if protocol.Name == "smb" {
			hasSMB = true
		}
		if protocol.Name != "timemachine" {
			protocols = append(protocols, protocol.Name)
		}
	}
	if !hasSMB {
		for _, protocol := range s.Protocols {
			if protocol.Name == "timemachine" {
				protocols = append(protocols, "smb")
				break
			}
		}
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
	principals := map[string]bool{}
	for _, rule := range s.Access {
		if strings.TrimSpace(rule.PrincipalID) == "" && strings.TrimSpace(rule.PrincipalName) == "" {
			return errors.New("access rule requires a principal")
		}
		principal := strings.TrimSpace(rule.PrincipalID)
		if principal == "" {
			principal = strings.TrimSpace(rule.PrincipalName)
		}
		if principals[principal] {
			return fmt.Errorf("access principal %q is listed more than once", principal)
		}
		principals[principal] = true
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
			networks, ok := networkStrings(value)
			if !ok {
				return errors.New("nfs allowedNetworks must be an array")
			}
			for _, item := range networks {
				if parseCIDR(item) == nil {
					return fmt.Errorf("nfs network %q is not a valid CIDR", item)
				}
			}
		}
		if value, ok := settings["rootSquash"]; ok {
			if _, ok := value.(bool); !ok {
				return errors.New("nfs rootSquash must be boolean")
			}
		}
	case "sftp":
		if value, ok := settings["chroot"]; ok {
			path, valid := value.(string)
			if !valid || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\x00\r\n") || strings.Contains(path, "..") {
				return errors.New("sftp chroot must be an absolute local path")
			}
		}
		if value, ok := settings["readOnly"]; ok {
			if _, ok := value.(bool); !ok {
				return errors.New("sftp readOnly must be boolean")
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
		if start, startOK := settings["passivePortStart"]; startOK {
			if end, endOK := settings["passivePortEnd"]; endOK && validPort(start) && validPort(end) && numberValue(start) > numberValue(end) {
				return fmt.Errorf("%s passive port range is inverted", protocol)
			}
		}
	case "rsync":
		if value, ok := settings["readOnly"]; ok {
			if _, ok := value.(bool); !ok {
				return errors.New("rsync readOnly must be boolean")
			}
		}
	}
	return nil
}

func networkStrings(value any) ([]string, bool) {
	switch values := value.(type) {
	case []string:
		return values, true
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, false
			}
			result = append(result, text)
		}
		return result, true
	default:
		return nil, false
	}
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
	port := numberValue(value)
	return port >= 1024 && port <= 65535
}

func numberValue(value any) int {
	var port int
	switch value := value.(type) {
	case float64:
		port = int(value)
	case int:
		port = value
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0
		}
		port = int(parsed)
	default:
		return 0
	}
	return port
}
