package biz

import (
	"context"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/derp"
	"github.com/juanfont/headscale-v2/internal/poll"
	"github.com/juanfont/headscale-v2/internal/state"
	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/tailcfg"
)

type HeadscaleUsecase struct {
	state       *state.State
	batcher     *poll.Batcher
	derpManager *derp.DERPManager
	haProber    *state.HAHealthProber
	ephemeralGC *state.EphemeralGarbageCollector
	logger      *log.Helper

	wg sync.WaitGroup
}

func NewHeadscaleUsecase(
	st *state.State,
	batcher *poll.Batcher,
	derpManager *derp.DERPManager,
	haProber *state.HAHealthProber,
	ephemeralGC *state.EphemeralGarbageCollector,
	logger log.Logger,
) *HeadscaleUsecase {
	return &HeadscaleUsecase{
		state:       st,
		batcher:     batcher,
		derpManager: derpManager,
		haProber:    haProber,
		ephemeralGC: ephemeralGC,
		logger:      log.NewHelper(logger),
	}
}

func (uc *HeadscaleUsecase) Start(ctx context.Context) {
	uc.wg.Add(3)
	go uc.runScheduledTasks(ctx)
	go uc.runDERPUpdates(ctx)
	go uc.runHAProbing(ctx)

	if uc.ephemeralGC != nil {
		uc.wg.Add(1)
		go func() {
			defer uc.wg.Done()
			uc.ephemeralGC.Start(ctx)
		}()
	}

	uc.logger.Info("HeadscaleUsecase started")
}

func (uc *HeadscaleUsecase) Stop() {
	uc.wg.Wait()
	if uc.ephemeralGC != nil {
		uc.ephemeralGC.Stop()
	}
	if uc.haProber != nil {
		uc.haProber.Stop()
	}
	if uc.derpManager != nil {
		uc.derpManager.Stop()
	}
	uc.logger.Info("HeadscaleUsecase stopped")
}

const updateInterval = 5 * time.Second

func (uc *HeadscaleUsecase) runScheduledTasks(ctx context.Context) {
	defer uc.wg.Done()

	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	uc.logger.Info("scheduled task worker started")

	for {
		select {
		case <-ctx.Done():
			uc.logger.Info("scheduled task worker shutting down")
			return
		case <-ticker.C:
			uc.checkExpiredNodes(ctx)
		}
	}
}

func (uc *HeadscaleUsecase) runDERPUpdates(ctx context.Context) {
	defer uc.wg.Done()

	if uc.derpManager == nil {
		return
	}

	uc.derpManager.StartUpdateLoop(ctx)
	uc.logger.Info("DERP update loop started")
}

func (uc *HeadscaleUsecase) runHAProbing(ctx context.Context) {
	defer uc.wg.Done()

	if uc.haProber == nil {
		return
	}

	uc.haProber.Start(ctx)
	uc.logger.Info("HA prober started")
}

func (uc *HeadscaleUsecase) checkExpiredNodes(ctx context.Context) {
	now := time.Now()
	nodes := uc.state.ListNodes()

	for _, node := range nodes {
		if !node.IsExpired() {
			continue
		}

		expiryOpt := node.Expiry()
		if !expiryOpt.Valid() {
			continue
		}
		expiry := expiryOpt.Get()

		uc.logger.Infof("Node %s (%d) expired at %v", node.Hostname(), node.ID(), expiry)

		if expiry.Before(now.Add(-24 * time.Hour)) {
			change, err := uc.state.DeleteNode(node)
			if err != nil {
				uc.logger.Warnf("failed to delete expired node %d: %v", node.ID(), err)
				continue
			}
			uc.NotifyChange(change)
			uc.logger.Infof("Deleted long-expired node %d", node.ID())
		} else {
			uc.NotifyChange(types.Change{
				TargetNode:   node.ID(),
				SendAllPeers: true,
				IncludeSelf:  true,
			})
		}
	}
}

func (uc *HeadscaleUsecase) NotifyChange(c types.Change) {
	if uc.batcher != nil {
		uc.batcher.SendChange(c)
	}
}

func (uc *HeadscaleUsecase) UpdateDERPMap(ctx context.Context) error {
	if uc.derpManager == nil {
		return nil
	}

	if err := uc.derpManager.UpdateNow(); err != nil {
		return err
	}

	newMap := uc.derpManager.GetDERPMap()
	oldMap := uc.state.DERPMap()

	if newMap != nil && oldMap != nil && !derpMapsEqual(oldMap, newMap) {
		uc.state.SetDERPMap(newMap)
		uc.NotifyChange(types.Change{SendAllPeers: true})
		uc.logger.Info("DERP map updated")
	}

	return nil
}

func derpMapsEqual(a, b *tailcfg.DERPMap) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.Regions) != len(b.Regions) {
		return false
	}
	for id := range a.Regions {
		if _, ok := b.Regions[id]; !ok {
			return false
		}
	}
	return a.OmitDefaultRegions == b.OmitDefaultRegions
}

func (uc *HeadscaleUsecase) GetDERPMap() *tailcfg.DERPMap {
	return uc.state.DERPMap()
}

func (uc *HeadscaleUsecase) ReloadPolicy(ctx context.Context) error {
	changes, err := uc.state.ReloadPolicy()
	if err != nil {
		return err
	}
	for _, c := range changes {
		uc.NotifyChange(c)
	}
	return nil
}