package identity

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type Kind string

const (
	KindUser    Kind = "user"
	KindGroup   Kind = "group"
	KindService Kind = "service"
)

type ManagementRole string

const (
	RoleNone     ManagementRole = "none"
	RoleOwner    ManagementRole = "owner"
	RoleAdmin    ManagementRole = "admin"
	RoleOperator ManagementRole = "operator"
	RoleReadonly ManagementRole = "readonly"
)

type AccessLevel string

const (
	AccessNone  AccessLevel = "none"
	AccessRead  AccessLevel = "read"
	AccessWrite AccessLevel = "write"
)

type Principal struct {
	ID             string         `json:"id"`
	Kind           Kind           `json:"kind"`
	Name           string         `json:"name"`
	UID            *int64         `json:"uid,omitempty"`
	GID            *int64         `json:"gid,omitempty"`
	ManagementRole ManagementRole `json:"managementRole"`
	Enabled        bool           `json:"enabled"`
	CreatedAt      string         `json:"createdAt"`
	UpdatedAt      string         `json:"updatedAt"`
	Groups         []string       `json:"groups,omitempty"`
}

type CreateInput struct {
	Kind           Kind
	Name           string
	Password       string
	ManagementRole ManagementRole
}

type UpdateInput struct {
	Name           *string
	Enabled        *bool
	ManagementRole *ManagementRole
}

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

func ValidateCreate(input CreateInput) error {
	if input.Kind != KindUser && input.Kind != KindGroup && input.Kind != KindService {
		return fmt.Errorf("unsupported principal kind %q", input.Kind)
	}
	if !namePattern.MatchString(input.Name) {
		return errors.New("principal name must start with a letter or number and contain only letters, numbers, dot, underscore, or hyphen")
	}
	if err := ValidateRole(input.ManagementRole); err != nil {
		return err
	}
	if input.Kind == KindGroup && input.Password != "" {
		return errors.New("groups cannot have passwords")
	}
	if input.Kind == KindService && input.ManagementRole != RoleNone {
		return errors.New("service identities cannot have management access")
	}
	if input.ManagementRole != RoleNone && len(input.Password) < 12 {
		return errors.New("management users require a password of at least 12 characters")
	}
	return nil
}

func ValidateRole(role ManagementRole) error {
	switch role {
	case "", RoleNone, RoleOwner, RoleAdmin, RoleOperator, RoleReadonly:
		return nil
	default:
		return fmt.Errorf("unsupported management role %q", role)
	}
}

func ValidateUpdate(kind Kind, name string, role ManagementRole) error {
	if kind != KindUser && kind != KindGroup && kind != KindService {
		return fmt.Errorf("unsupported principal kind %q", kind)
	}
	if !namePattern.MatchString(name) {
		return errors.New("principal name must start with a letter or number and contain only letters, numbers, dot, underscore, or hyphen")
	}
	if err := ValidateRole(role); err != nil {
		return err
	}
	if kind == KindGroup && role != RoleNone {
		return errors.New("groups cannot have management access")
	}
	if kind == KindService && role != RoleNone {
		return errors.New("service identities cannot have management access")
	}
	return nil
}

func NormalizeRole(role ManagementRole) ManagementRole {
	if role == "" {
		return RoleNone
	}
	return role
}

func ValidateAccess(level AccessLevel) error {
	switch level {
	case AccessNone, AccessRead, AccessWrite:
		return nil
	default:
		return fmt.Errorf("unsupported access level %q", level)
	}
}

func NormalizeName(name string) string { return strings.TrimSpace(name) }
