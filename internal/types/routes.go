package types

import (
	"net/netip"
	"time"
)

type Route struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time `gorm:"index"`

	NodeID uint64 `gorm:"not null"`
	Node   *Node
	Prefix netip.Prefix `gorm:"serializer:text"`
	Advertised bool
	Enabled bool
	IsPrimary bool
}

type Routes []Route