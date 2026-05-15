package types

import (
	"net/netip"
	"time"

	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

type RegistrationData struct {
	MachineKey key.MachinePublic
	NodeKey    key.NodePublic
	DiscoKey   key.DiscoPublic
	Hostname   string
	Hostinfo   *tailcfg.Hostinfo
	Endpoints  []netip.AddrPort
	Expiry     *time.Time
}