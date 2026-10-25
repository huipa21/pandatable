package acl

import (
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/apperr"
)

// Level is an ordered permission level; higher values include lower ones.
type Level int

const (
	None Level = iota
	Read
	Write
	Owner
)

func (level Level) String() string {
	switch level {
	case Read:
		return "read"
	case Write:
		return "write"
	case Owner:
		return "owner"
	default:
		return "none"
	}
}

// ParseLevel accepts only the three grantable levels.
func ParseLevel(value string) (Level, bool) {
	switch value {
	case "read":
		return Read, true
	case "write":
		return Write, true
	case "owner":
		return Owner, true
	default:
		return None, false
	}
}

type ResourceType string

const (
	Workspace ResourceType = "workspace"
	Book      ResourceType = "book"
	Table     ResourceType = "table"
)

func ParseResourceType(value string) (ResourceType, bool) {
	switch ResourceType(value) {
	case Workspace, Book, Table:
		return ResourceType(value), true
	default:
		return "", false
	}
}

// Resource identifies a workspace, book or table by canonical UUID.
type Resource struct {
	Type ResourceType
	ID   pgtype.UUID
}

func NewResource(resourceType, id string) (Resource, error) {
	parsed, ok := ParseResourceType(resourceType)
	if !ok {
		return Resource{}, ErrValidation()
	}
	uuid, ok := ParseUUID(id)
	if !ok {
		return Resource{}, ErrValidation()
	}
	return Resource{Type: parsed, ID: uuid}, nil
}

func (resource Resource) String() string { return string(resource.Type) + ":" + resource.ID.String() }

// ParseResourceRef parses the "type:uuid" query form used by the grants API.
func ParseResourceRef(value string) (Resource, error) {
	kind, id, found := strings.Cut(value, ":")
	if !found {
		return Resource{}, ErrValidation()
	}
	return NewResource(kind, id)
}

// ParseUUID accepts only the canonical 8-4-4-4-12 hexadecimal form.
func ParseUUID(value string) (pgtype.UUID, bool) {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return pgtype.UUID{}, false
	}
	var uuid pgtype.UUID
	if err := uuid.Scan(value); err != nil || !uuid.Valid {
		return pgtype.UUID{}, false
	}
	return uuid, true
}

// Access is the caller's effective level on a resource. Visible also covers
// path-only visibility through a grant on a descendant (06-permissions).
type Access struct {
	Level   Level
	Visible bool
}

// Check returns nil when the level suffices, 404 when the resource is hidden
// from the caller and 403 when it is visible but the level is too low.
func (access Access) Check(required Level) error {
	if required < Read {
		required = Read
	}
	if access.Level >= required {
		return nil
	}
	if !access.Visible && access.Level == None {
		return ErrNotFound()
	}
	return ErrPermissionDenied()
}

func ErrValidation() *apperr.Error {
	return apperr.New(400, "ERR_VALIDATION", "Invalid request")
}

func ErrNotFound() *apperr.Error {
	return apperr.New(404, "ERR_NOT_FOUND", "Not found")
}

func ErrPermissionDenied() *apperr.Error {
	return apperr.New(403, "ERR_PERMISSION_DENIED", "Permission denied")
}

func errLastOwner() *apperr.Error {
	return apperr.New(409, "ERR_LAST_OWNER", "A workspace must keep at least one active direct user owner")
}

func errPrincipalNotFound() *apperr.Error {
	return apperr.New(404, "ERR_PRINCIPAL_NOT_FOUND", "Principal not found")
}

func errPrincipalDisabled() *apperr.Error {
	return apperr.New(409, "ERR_PRINCIPAL_DISABLED", "Disabled users cannot receive grants")
}
