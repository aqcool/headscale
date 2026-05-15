package derp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"tailscale.com/tailcfg"
)

func TestMergeDERPMaps(t *testing.T) {
	base := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			900: {
				RegionID:   900,
				RegionCode: "base",
				Nodes: []*tailcfg.DERPNode{
					{Name: "base-derp", RegionID: 900},
				},
			},
		},
	}

	override := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			900: {
				RegionID:   900,
				RegionCode: "override",
				Nodes: []*tailcfg.DERPNode{
					{Name: "override-derp", RegionID: 900},
				},
			},
			901: {
				RegionID:   901,
				RegionCode: "new",
				Nodes: []*tailcfg.DERPNode{
					{Name: "new-derp", RegionID: 901},
				},
			},
		},
	}

	result := MergeDERPMaps(base, override)

	assert.Len(t, result.Regions, 2)
	assert.Equal(t, "override", result.Regions[900].RegionCode)
	assert.Equal(t, "new", result.Regions[901].RegionCode)
	assert.Len(t, result.Regions[900].Nodes, 1)
	assert.Equal(t, "override-derp", result.Regions[900].Nodes[0].Name)
}

func TestMergeDERPMaps_NilBase(t *testing.T) {
	override := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			900: {
				RegionID:   900,
				RegionCode: "test",
			},
		},
	}

	result := MergeDERPMaps(nil, override)
	assert.Equal(t, override, result)
}

func TestMergeDERPMaps_NilOverlay(t *testing.T) {
	base := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			900: {
				RegionID:   900,
				RegionCode: "test",
			},
		},
	}

	result := MergeDERPMaps(base, nil)
	assert.Equal(t, base, result)
}

func TestMapsEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        *tailcfg.DERPMap
		b        *tailcfg.DERPMap
		expected bool
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name: "one nil",
			a:    nil,
			b: &tailcfg.DERPMap{
				Regions: map[int]*tailcfg.DERPRegion{},
			},
			expected: false,
		},
		{
			name: "same regions",
			a: &tailcfg.DERPMap{
				Regions: map[int]*tailcfg.DERPRegion{
					900: {RegionID: 900, RegionCode: "test"},
				},
			},
			b: &tailcfg.DERPMap{
				Regions: map[int]*tailcfg.DERPRegion{
					900: {RegionID: 900, RegionCode: "test"},
				},
			},
			expected: true,
		},
		{
			name: "different regions",
			a: &tailcfg.DERPMap{
				Regions: map[int]*tailcfg.DERPRegion{
					900: {RegionID: 900, RegionCode: "test1"},
				},
			},
			b: &tailcfg.DERPMap{
				Regions: map[int]*tailcfg.DERPRegion{
					900: {RegionID: 900, RegionCode: "test2"},
				},
			},
			expected: false,
		},
		{
			name: "different region count",
			a: &tailcfg.DERPMap{
				Regions: map[int]*tailcfg.DERPRegion{
					900: {RegionID: 900, RegionCode: "test"},
				},
			},
			b: &tailcfg.DERPMap{
				Regions: map[int]*tailcfg.DERPRegion{
					900: {RegionID: 900, RegionCode: "test"},
					901: {RegionID: 901, RegionCode: "test2"},
				},
			},
			expected: false,
		},
		{
			name: "omit default differs",
			a: &tailcfg.DERPMap{
				OmitDefaultRegions: true,
				Regions:            map[int]*tailcfg.DERPRegion{},
			},
			b: &tailcfg.DERPMap{
				OmitDefaultRegions: false,
				Regions:            map[int]*tailcfg.DERPRegion{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapsEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRegionEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        *tailcfg.DERPRegion
		b        *tailcfg.DERPRegion
		expected bool
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "one nil",
			a:        nil,
			b:        &tailcfg.DERPRegion{},
			expected: false,
		},
		{
			name: "identical",
			a: &tailcfg.DERPRegion{
				RegionID:   900,
				RegionCode: "test",
				RegionName: "Test Region",
			},
			b: &tailcfg.DERPRegion{
				RegionID:   900,
				RegionCode: "test",
				RegionName: "Test Region",
			},
			expected: true,
		},
		{
			name: "different id",
			a: &tailcfg.DERPRegion{
				RegionID: 900,
			},
			b: &tailcfg.DERPRegion{
				RegionID: 901,
			},
			expected: false,
		},
		{
			name: "different nodes count",
			a: &tailcfg.DERPRegion{
				Nodes: []*tailcfg.DERPNode{{Name: "node1"}},
			},
			b: &tailcfg.DERPRegion{
				Nodes: []*tailcfg.DERPNode{{Name: "node1"}, {Name: "node2"}},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := regionEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNodeEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        *tailcfg.DERPNode
		b        *tailcfg.DERPNode
		expected bool
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "one nil",
			a:        nil,
			b:        &tailcfg.DERPNode{},
			expected: false,
		},
		{
			name: "identical",
			a: &tailcfg.DERPNode{
				Name:      "derp1",
				RegionID:  900,
				HostName:  "derp.example.com",
				DERPPort:  443,
				STUNPort:  3478,
				IPv4:      "1.2.3.4",
				IPv6:      "fd00::1",
			},
			b: &tailcfg.DERPNode{
				Name:      "derp1",
				RegionID:  900,
				HostName:  "derp.example.com",
				DERPPort:  443,
				STUNPort:  3478,
				IPv4:      "1.2.3.4",
				IPv6:      "fd00::1",
			},
			expected: true,
		},
		{
			name: "different name",
			a:    &tailcfg.DERPNode{Name: "derp1"},
			b:    &tailcfg.DERPNode{Name: "derp2"},
			expected: false,
		},
		{
			name: "different port",
			a:    &tailcfg.DERPNode{DERPPort: 443},
			b:    &tailcfg.DERPNode{DERPPort: 8080},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nodeEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetDefaultDERPMap(t *testing.T) {
	derpMap := getDefaultDERPMap()

	assert.NotNil(t, derpMap)
	assert.Len(t, derpMap.Regions, 1)
	assert.Contains(t, derpMap.Regions, 900)
	assert.Equal(t, "headscale", derpMap.Regions[900].RegionCode)
	assert.Len(t, derpMap.Regions[900].Nodes, 1)
	assert.Equal(t, 443, derpMap.Regions[900].Nodes[0].DERPPort)
	assert.Equal(t, -1, derpMap.Regions[900].Nodes[0].STUNPort)
}

func TestShuffleDERPMap(t *testing.T) {
	derpMap := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			900: {
				RegionID: 900,
				Nodes: []*tailcfg.DERPNode{
					{Name: "node3"},
					{Name: "node1"},
					{Name: "node2"},
				},
			},
		},
	}

	shuffleDERPMap(derpMap)

	assert.Equal(t, "node1", derpMap.Regions[900].Nodes[0].Name)
	assert.Equal(t, "node2", derpMap.Regions[900].Nodes[1].Name)
	assert.Equal(t, "node3", derpMap.Regions[900].Nodes[2].Name)
}

func TestShuffleDERPMap_Nil(t *testing.T) {
	shuffleDERPMap(nil)
	assert.True(t, true, "should not panic on nil")
}

func TestShuffleDERPMap_Empty(t *testing.T) {
	derpMap := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{},
	}

	shuffleDERPMap(derpMap)
	assert.Empty(t, derpMap.Regions)
}