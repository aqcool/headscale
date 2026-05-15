package types

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var (
	ErrUserHasNodes        = errors.New("user has nodes")
	ErrUserAlreadyExists   = errors.New("user already exists")
	ErrUserNotFound        = errors.New("user not found")
)

// TaggedDevicesUserID is the special user ID for tagged devices.
const TaggedDevicesUserID = UserID(2147455555)

// TaggedDevicesUser is a special user used in MapResponse for tagged nodes.
var TaggedDevicesUser = User{
	ID:   TaggedDevicesUserID,
	Name: "tagged-devices",
}

type (
	UserID  uint64
	UserIDs []UserID
)

type User struct {
	ID                 UserID         `gorm:"primary_key"`
	Name               string         `gorm:"unique"`
	DisplayName        string
	Email              string
	ProfileURL         string
	ProviderIdentifier sql.NullString
	Provider           string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt         *time.Time
}

type Users []User

func (u Users) String() string {
	var sb strings.Builder
	sb.WriteString("[ ")
	for _, user := range u {
		fmt.Fprintf(&sb, "%d: %s, ", user.ID, user.Name)
	}
	sb.WriteString(" ]")
	return sb.String()
}

func (u *User) Username() string {
	if u.Name != "" {
		return u.Name
	}
	return "unknown"
}

func (u *User) Display() string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Username()
}

func (u *User) String() string {
	return u.Name
}

func (u *User) TypedID() *UserID {
	uid := UserID(u.ID)
	return &uid
}

func (u *User) UintID() *uint {
	id := uint(u.ID)
	return &id
}

// CleanIdentifier cleans and normalizes an identifier string.
func CleanIdentifier(identifier string) string {
	if identifier == "" {
		return identifier
	}

	identifier = strings.TrimSpace(identifier)

	u, err := url.Parse(identifier)
	if err == nil && u.Scheme != "" {
		parts := strings.FieldsFunc(u.Path, func(c rune) bool { return c == '/' })
		for i, part := range parts {
			parts[i] = strings.TrimSpace(part)
		}
		cleanParts := make([]string, 0, len(parts))
		for _, part := range parts {
			if part != "" {
				cleanParts = append(cleanParts, part)
			}
		}

		if len(cleanParts) == 0 {
			u.Path = ""
		} else {
			u.Path = "/" + strings.Join(cleanParts, "/")
		}
		u.Scheme = strings.ToLower(u.Scheme)

		return u.String()
	}

	parts := strings.FieldsFunc(identifier, func(c rune) bool { return c == '/' })
	cleanParts := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			cleanParts = append(cleanParts, trimmed)
		}
	}

	if len(cleanParts) == 0 {
		return ""
	}

	return strings.Join(cleanParts, "/")
}
