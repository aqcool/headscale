package state

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHAHealthProber_Disabled(t *testing.T) {
	prober := NewHAHealthProber(nil, 0, 0, testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go prober.Start(ctx)

	time.Sleep(50 * time.Millisecond)

	assert.True(t, prober.IsRunning(), "prober marks as running even when disabled, but doesn't probe")

	prober.Stop()
}

func TestHAHealthProber_Stop(t *testing.T) {
	prober := NewHAHealthProber(nil, 100*time.Millisecond, 5*time.Second, testLogger)

	ctx, cancel := context.WithCancel(context.Background())

	go prober.Start(ctx)

	time.Sleep(50 * time.Millisecond)

	prober.Stop()

	assert.False(t, prober.IsRunning())

	cancel()
}

func TestHAHealthProber_StartStopWithState(t *testing.T) {
	stateCfg := &StateConfig{
		IPAlloc:      nil,
		BatchSize:    TestBatchSize,
		BatchTimeout: TestBatchTimeout,
		PeersFunc:    allowAllPeersFunc,
	}
	st := NewState(stateCfg, testLogger)

	prober := NewHAHealthProber(st, 50*time.Millisecond, 1*time.Second, testLogger)

	ctx, cancel := context.WithCancel(context.Background())

	go prober.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	assert.True(t, prober.IsRunning())

	prober.Stop()

	time.Sleep(50 * time.Millisecond)

	assert.False(t, prober.IsRunning())

	cancel()
}