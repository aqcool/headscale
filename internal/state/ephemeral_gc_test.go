package state

import (
	"context"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/types"
	"github.com/stretchr/testify/assert"
)

var testLogger = log.DefaultLogger

func createEphemeralTestNode(id types.NodeID, hostname string, ephemeral bool) types.Node {
	node := createTestNode(id, 1, "user1", hostname)
	if ephemeral {
		node.AuthKey = &types.PreAuthKey{
			Ephemeral: true,
		}
	}
	return node
}

func TestEphemeralGarbageCollector_Schedule(t *testing.T) {
	gc := NewEphemeralGarbageCollector(nil, 50*time.Millisecond, testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go gc.Start(ctx)

	time.Sleep(10 * time.Millisecond)

	node := createEphemeralTestNode(1, "ephemeral-node", true)

	gc.Schedule(node.View())

	assert.True(t, gc.IsScheduled(1))
	assert.Equal(t, 1, gc.ScheduledCount())
}

func TestEphemeralGarbageCollector_CancelSchedule(t *testing.T) {
	gc := NewEphemeralGarbageCollector(nil, 50*time.Millisecond, testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go gc.Start(ctx)

	time.Sleep(10 * time.Millisecond)

	node := createEphemeralTestNode(1, "ephemeral-node", true)

	gc.Schedule(node.View())
	assert.True(t, gc.IsScheduled(1))

	gc.CancelSchedule(1)
	assert.False(t, gc.IsScheduled(1))
	assert.Equal(t, 0, gc.ScheduledCount())
}

func TestEphemeralGarbageCollector_ScheduleNonEphemeral(t *testing.T) {
	gc := NewEphemeralGarbageCollector(nil, 50*time.Millisecond, testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go gc.Start(ctx)

	time.Sleep(10 * time.Millisecond)

	node := createEphemeralTestNode(1, "non-ephemeral-node", false)

	gc.Schedule(node.View())

	assert.False(t, gc.IsScheduled(1))
	assert.Equal(t, 0, gc.ScheduledCount())
}

func TestEphemeralGarbageCollector_Stop(t *testing.T) {
	gc := NewEphemeralGarbageCollector(nil, 50*time.Millisecond, testLogger)

	ctx, cancel := context.WithCancel(context.Background())

	go gc.Start(ctx)

	time.Sleep(10 * time.Millisecond)

	node := createEphemeralTestNode(1, "ephemeral-node", true)

	gc.Schedule(node.View())
	assert.Equal(t, 1, gc.ScheduledCount())

	gc.Stop()

	assert.Equal(t, 0, gc.ScheduledCount())

	cancel()
}

func TestEphemeralGarbageCollector_ScheduleReplaces(t *testing.T) {
	gc := NewEphemeralGarbageCollector(nil, 10*time.Second, testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go gc.Start(ctx)

	time.Sleep(10 * time.Millisecond)

	node := createEphemeralTestNode(1, "ephemeral-node", true)

	gc.Schedule(node.View())
	assert.True(t, gc.IsScheduled(1))

	gc.Schedule(node.View())
	assert.True(t, gc.IsScheduled(1))
	assert.Equal(t, 1, gc.ScheduledCount())
}

func TestEphemeralGarbageCollector_ProcessNodes(t *testing.T) {
	gc := NewEphemeralGarbageCollector(nil, 10*time.Second, testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go gc.Start(ctx)

	time.Sleep(10 * time.Millisecond)

	online := true

	offlineNode := createEphemeralTestNode(1, "offline-ephemeral", true)

	onlineNode := createEphemeralTestNode(2, "online-ephemeral", true)
	onlineNode.IsOnline = &online

	nonEphemeralNode := createEphemeralTestNode(3, "non-ephemeral", false)

	nodes := []types.NodeView{
		offlineNode.View(),
		onlineNode.View(),
		nonEphemeralNode.View(),
	}

	gc.ProcessNodes(nodes)

	assert.True(t, gc.IsScheduled(1), "offline ephemeral should be scheduled")
	assert.False(t, gc.IsScheduled(2), "online ephemeral should not be scheduled")
	assert.False(t, gc.IsScheduled(3), "non-ephemeral should not be scheduled")
}