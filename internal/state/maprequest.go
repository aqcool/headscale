package state

import (
	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/tailcfg"
)

func netInfoFromMapRequest(
	nodeID types.NodeID,
	currentHostinfo *tailcfg.Hostinfo,
	reqHostinfo *tailcfg.Hostinfo,
) *tailcfg.NetInfo {
	if reqHostinfo != nil && reqHostinfo.NetInfo != nil {
		return reqHostinfo.NetInfo
	}

	if currentHostinfo != nil && currentHostinfo.NetInfo != nil {
		return currentHostinfo.NetInfo
	}

	return nil
}