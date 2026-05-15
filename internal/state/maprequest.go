package state

import (
	"github.com/juanfont/headscale-v2/internal/types"
	"github.com/rs/zerolog/log"
	"tailscale.com/tailcfg"
)

// netInfoFromMapRequest determines the correct NetInfo to use.
func netInfoFromMapRequest(
	nodeID types.NodeID,
	currentHostinfo *tailcfg.Hostinfo,
	reqHostinfo *tailcfg.Hostinfo,
) *tailcfg.NetInfo {
	if reqHostinfo != nil && reqHostinfo.NetInfo != nil {
		return reqHostinfo.NetInfo
	}

	if currentHostinfo != nil && currentHostinfo.NetInfo != nil {
		log.Debug().
			Uint64("node.id", nodeID.Uint64()).
			Int("preferredDERP", currentHostinfo.NetInfo.PreferredDERP).
			Msg("using NetInfo from previous Hostinfo")
		return currentHostinfo.NetInfo
	}

	return nil
}