package derp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/types"
	"gopkg.in/yaml.v3"
	"tailscale.com/tailcfg"
)

type DERPManager struct {
	embedded bool
	cfg      *types.DERPConfig
	logger   *log.Helper
	map_     atomic.Pointer[tailcfg.DERPMap]
	stopChan chan struct{}
}

func NewDERPManager(cfg *types.DERPConfig, logger log.Logger) *DERPManager {
	helper := log.NewHelper(logger)

	m := &DERPManager{
		embedded: cfg.ServerEnabled,
		cfg:      cfg,
		logger:   helper,
		stopChan: make(chan struct{}),
	}

	initialMap, err := GetDERPMap(cfg)
	if err != nil {
		helper.Warnf("failed to load initial DERP map: %v, using default", err)
		initialMap = getDefaultDERPMap()
	}

	m.SetDERPMap(initialMap)

	return m
}

func (m *DERPManager) GetDERPMap() *tailcfg.DERPMap {
	return m.map_.Load()
}

func (m *DERPManager) SetDERPMap(derpMap *tailcfg.DERPMap) {
	m.map_.Store(derpMap)
}

func (m *DERPManager) StartUpdateLoop(ctx context.Context) {
	if !m.cfg.AutoUpdate {
		return
	}

	updateFrequency := m.cfg.UpdateFrequency
	if updateFrequency == 0 {
		updateFrequency = 3 * time.Hour
	}

	ticker := time.NewTicker(updateFrequency)
	defer ticker.Stop()

	m.logger.Infof("DERP map auto-update started, frequency: %v", updateFrequency)

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("DERP map update loop stopped")
			return
		case <-m.stopChan:
			m.logger.Info("DERP map update loop stopped")
			return
		case <-ticker.C:
			m.logger.Debug("updating DERP map")
			newMap, err := GetDERPMap(m.cfg)
			if err != nil {
				m.logger.Warnf("failed to update DERP map: %v", err)
				continue
			}

			oldMap := m.GetDERPMap()
			if !mapsEqual(oldMap, newMap) {
				m.SetDERPMap(newMap)
				m.logger.Info("DERP map updated")
			}
		}
	}
}

func (m *DERPManager) Stop() {
	close(m.stopChan)
}

func (m *DERPManager) UpdateNow() error {
	newMap, err := GetDERPMap(m.cfg)
	if err != nil {
		return err
	}

	m.SetDERPMap(newMap)
	return nil
}

func mapsEqual(a, b *tailcfg.DERPMap) bool {
	if a == nil || b == nil {
		return a == b
	}

	if len(a.Regions) != len(b.Regions) {
		return false
	}

	for id, regionA := range a.Regions {
		regionB, ok := b.Regions[id]
		if !ok {
			return false
		}

		if !regionEqual(regionA, regionB) {
			return false
		}
	}

	return a.OmitDefaultRegions == b.OmitDefaultRegions
}

func regionEqual(a, b *tailcfg.DERPRegion) bool {
	if a == nil || b == nil {
		return a == b
	}

	if a.RegionID != b.RegionID || a.RegionCode != b.RegionCode || a.RegionName != b.RegionName {
		return false
	}

	if len(a.Nodes) != len(b.Nodes) {
		return false
	}

	for i, nodeA := range a.Nodes {
		nodeB := b.Nodes[i]
		if !nodeEqual(nodeA, nodeB) {
			return false
		}
	}

	return true
}

func nodeEqual(a, b *tailcfg.DERPNode) bool {
	if a == nil || b == nil {
		return a == b
	}

	return a.Name == b.Name &&
		a.RegionID == b.RegionID &&
		a.HostName == b.HostName &&
		a.DERPPort == b.DERPPort &&
		a.STUNPort == b.STUNPort &&
		a.IPv4 == b.IPv4 &&
		a.IPv6 == b.IPv6
}

func GetDERPMap(cfg *types.DERPConfig) (*tailcfg.DERPMap, error) {
	var derpMaps []*tailcfg.DERPMap

	for _, addr := range cfg.URLs {
		derpMap, err := loadDERPMapFromURL(addr)
		if err != nil {
			return nil, fmt.Errorf("loading DERP map from URL %s: %w", addr.String(), err)
		}
		derpMaps = append(derpMaps, derpMap)
	}

	for _, path := range cfg.Paths {
		derpMap, err := loadDERPMapFromPath(path)
		if err != nil {
			return nil, fmt.Errorf("loading DERP map from path %s: %w", path, err)
		}
		derpMaps = append(derpMaps, derpMap)
	}

	if len(derpMaps) == 0 {
		return getDefaultDERPMap(), nil
	}

	derpMap := mergeDERPMaps(derpMaps)
	shuffleDERPMap(derpMap)

	return derpMap, nil
}

func loadDERPMapFromPath(path string) (*tailcfg.DERPMap, error) {
	derpFile, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer derpFile.Close()

	var derpMap tailcfg.DERPMap

	b, err := io.ReadAll(derpFile)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(b, &derpMap)

	return &derpMap, err
}

func loadDERPMapFromURL(addr url.URL) (*tailcfg.DERPMap, error) {
	ctx, cancel := context.WithTimeout(context.Background(), types.HTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr.String(), nil)
	if err != nil {
		return nil, err
	}

	client := http.Client{
		Timeout: types.HTTPTimeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var derpMap tailcfg.DERPMap

	err = json.Unmarshal(body, &derpMap)

	return &derpMap, err
}

func mergeDERPMaps(derpMaps []*tailcfg.DERPMap) *tailcfg.DERPMap {
	result := tailcfg.DERPMap{
		OmitDefaultRegions: false,
		Regions:            map[int]*tailcfg.DERPRegion{},
	}

	for _, derpMap := range derpMaps {
		maps.Copy(result.Regions, derpMap.Regions)
	}

	for id, region := range result.Regions {
		if region == nil {
			delete(result.Regions, id)
		}
	}

	return &result
}

func shuffleDERPMap(dm *tailcfg.DERPMap) {
	if dm == nil || len(dm.Regions) == 0 {
		return
	}

	ids := make([]int, 0, len(dm.Regions))
	for id := range dm.Regions {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	for _, id := range ids {
		region := dm.Regions[id]
		if len(region.Nodes) == 0 {
			continue
		}

		slices.SortFunc(region.Nodes, func(a, b *tailcfg.DERPNode) int {
			if a.Name < b.Name {
				return -1
			}
			if a.Name > b.Name {
				return 1
			}
			return 0
		})
	}
}

func getDefaultDERPMap() *tailcfg.DERPMap {
	return &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			900: {
				RegionID:   900,
				RegionCode: "headscale",
				RegionName: "Headscale DERP",
				Nodes: []*tailcfg.DERPNode{
					{
						Name:     "headscale-derp",
						RegionID: 900,
						HostName: "derp.headscale.net",
						DERPPort: 443,
						STUNPort: -1,
					},
				},
			},
		},
		OmitDefaultRegions: false,
	}
}

func MergeDERPMaps(base, overlay *tailcfg.DERPMap) *tailcfg.DERPMap {
	if base == nil {
		return overlay
	}
	if overlay == nil {
		return base
	}

	result := &tailcfg.DERPMap{
		Regions:            make(map[int]*tailcfg.DERPRegion),
		OmitDefaultRegions: overlay.OmitDefaultRegions,
	}

	for id, region := range base.Regions {
		result.Regions[id] = region
	}

	for id, region := range overlay.Regions {
		result.Regions[id] = region
	}

	return result
}