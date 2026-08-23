package domain

import (
	"net/mail"
	"strings"
	"time"
)

type Role string

const (
	RoleResidentLiaison Role = "resident_liaison"
	RoleOfficer         Role = "officer"
	RoleInspector       Role = "inspector"
	RoleOperator        Role = "operator"
	RoleAdmin           Role = "admin"
)

func (r Role) Valid() bool {
	switch r {
	case RoleResidentLiaison, RoleOfficer, RoleInspector, RoleOperator, RoleAdmin:
		return true
	default:
		return false
	}
}

type User struct {
	ID           ID
	Email        string
	DisplayName  string
	PasswordHash []byte
	Role         Role
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func NewUser(id ID, email, displayName string, passwordHash []byte, role Role, now time.Time) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if _, err := mail.ParseAddress(email); err != nil {
		return User{}, FieldError{Field: "email", Message: "must be a valid address"}
	}
	displayName = strings.TrimSpace(displayName)
	if len(displayName) < 2 || len(displayName) > 80 {
		return User{}, FieldError{Field: "display_name", Message: "must contain 2 to 80 characters"}
	}
	if len(passwordHash) == 0 {
		return User{}, FieldError{Field: "password", Message: "hash is required"}
	}
	if !role.Valid() {
		return User{}, FieldError{Field: "role", Message: "is unsupported"}
	}
	now = now.UTC()
	return User{ID: id, Email: email, DisplayName: displayName, PasswordHash: append([]byte(nil), passwordHash...), Role: role, Active: true, CreatedAt: now, UpdatedAt: now}, nil
}

func (u User) Can(action string) bool {
	if !u.Active {
		return false
	}
	switch action {
	case "case:create", "case:view":
		return true
	case "case:triage", "case:assign", "verification:close":
		return u.Role == RoleOfficer || u.Role == RoleAdmin
	case "forest_survey:create", "forest_survey:publish", "verification:measure":
		return u.Role == RoleInspector || u.Role == RoleAdmin
	case "plan:propose", "forest_asset:complete":
		return u.Role == RoleOperator || u.Role == RoleAdmin
	case "plan:approve":
		return u.Role == RoleOfficer || u.Role == RoleAdmin
	case "admin:manage":
		return u.Role == RoleAdmin
	default:
		return false
	}
}

type Session struct {
	ID        ID
	UserID    ID
	TokenHash []byte
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
	UserAgent string
	IP        string
}

func (s Session) ActiveAt(now time.Time) bool {
	return s.RevokedAt == nil && now.UTC().Before(s.ExpiresAt.UTC())
}

func (s Session) Clone() Session {
	copySession := s
	copySession.TokenHash = append([]byte(nil), s.TokenHash...)
	if s.RevokedAt != nil {
		value := *s.RevokedAt
		copySession.RevokedAt = &value
	}
	return copySession
}
