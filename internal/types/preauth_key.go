package types

import (
	"time"
)

type PAKError string

func (e PAKError) Error() string { return string(e) }

// PreAuthKey describes a pre-authorization key usable in a particular user.
type PreAuthKey struct {
	ID uint64 `gorm:"primary_key"`

	Key string
	Prefix string
	Hash   []byte

	UserID *uint
	User   *User `gorm:"constraint:OnDelete:SET NULL;"`

	Reusable  bool
	Ephemeral bool `gorm:"default:false"`
	Used      bool `gorm:"default:false"`

	Tags []string `gorm:"serializer:json"`

	CreatedAt  *time.Time
	Expiration *time.Time
}

// PreAuthKeyNew is returned once when the key is created.
type PreAuthKeyNew struct {
	ID         uint64 `gorm:"primary_key"`
	Key        string
	Reusable   bool
	Ephemeral  bool
	Tags       []string
	Expiration *time.Time
	CreatedAt  *time.Time
	User       *User
}

func (key *PreAuthKey) Validate() error {
	if key == nil {
		return PAKError("invalid authkey")
	}

	if key.Expiration != nil && key.Expiration.Before(time.Now()) {
		return PAKError("authkey expired")
	}

	if key.Reusable {
		return nil
	}

	if key.Used {
		return PAKError("authkey already used")
	}

	return nil
}

func (pak *PreAuthKey) IsTagged() bool {
	return len(pak.Tags) > 0
}

func (pak *PreAuthKey) IsValid() bool {
	return pak != nil && pak.Key != ""
}

func (pak *PreAuthKey) IsExpired() bool {
	if pak == nil || pak.Expiration == nil {
		return false
	}
	return pak.Expiration.Before(time.Now())
}

func (pak *PreAuthKey) UsedCount() int {
	if pak == nil || !pak.Used {
		return 0
	}
	return 1
}

func (pak *PreAuthKey) maskedPrefix() string {
	if pak.Prefix != "" {
		return "hskey-auth-" + pak.Prefix + "-***"
	}
	return ""
}