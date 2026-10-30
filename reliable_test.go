// Copyright 2019 Sergey Romanov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENCE file.

package goemits

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newReliableEmitter(t *testing.T, cfg Config) *Goemits {
	t.Helper()
	mr := miniredis.RunT(t)
	cfg.Address = mr.Addr()
	cfg.Codec = RawCodec{}
	cfg.PollInterval = 20 * time.Millisecond
	cfg.ClaimIdle = 30 * time.Millisecond
	cfg.ClaimInterval = 50 * time.Millisecond
	ge, err := New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ge.Close()
	})
	return ge
}

func startInBackground(t *testing.T, ge *Goemits) (context.CancelFunc, chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ge.Start(ctx) }()
	return cancel, done
}

func TestEmitReliableDelivered(t *testing.T) {
	ge := newReliableEmitter(t, Config{})
	got := make(chan string, 1)
	require.NoError(t, ge.OnReliable("jobs", func(payload string) {
		got <- payload
	}))
	cancel, _ := startInBackground(t, ge)
	defer cancel()

	require.NoError(t, ge.EmitReliable(context.Background(), "jobs", "do-work"))
	assert.Equal(t, "do-work", waitFor(t, got))

	assert.Eventually(t, func() bool {
		n, err := ge.client.XLen(context.Background(), "jobs").Result()
		return err == nil && n == 1
	}, 2*time.Second, 20*time.Millisecond)
	pending, err := ge.client.XPending(context.Background(), "jobs", ge.group).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), pending.Count)
}

func TestReliableOfflineSubscriber(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := Config{
		Address:       mr.Addr(),
		Codec:         RawCodec{},
		PollInterval:  20 * time.Millisecond,
		ClaimIdle:     30 * time.Millisecond,
		ClaimInterval: 50 * time.Millisecond,
	}
	producer, err := New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = producer.Close() })

	// Messages are emitted before any subscriber exists.
	require.NoError(t, producer.EmitReliable(context.Background(), "jobs", "first"))
	require.NoError(t, producer.EmitReliable(context.Background(), "jobs", "second"))

	consumer, err := New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = consumer.Close() })
	got := make(chan string, 4)
	require.NoError(t, consumer.OnReliable("jobs", func(payload string) {
		got <- payload
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = consumer.Start(ctx) }()

	assert.Equal(t, "first", waitFor(t, got))
	assert.Equal(t, "second", waitFor(t, got))
}

func TestReliableStalePendingReclaimed(t *testing.T) {
	ge := newReliableEmitter(t, Config{})

	// A message that stays pending, as if a previous consumer crashed
	// before acknowledging it.
	require.NoError(t, ge.EmitReliable(context.Background(), "jobs", "orphan"))
	require.NoError(t, ge.ensureGroup(context.Background(), "jobs"))
	_, err := ge.client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    ge.group,
		Consumer: "dead-consumer",
		Streams:  []string{"jobs", ">"},
	}).Result()
	require.NoError(t, err)

	got := make(chan string, 1)
	require.NoError(t, ge.OnReliable("jobs", func(payload string) {
		got <- payload
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	assert.Equal(t, "orphan", waitFor(t, got))
}

func TestReliableJSONPayload(t *testing.T) {
	mr := miniredis.RunT(t)
	ge, err := New(Config{Address: mr.Addr(), PollInterval: 20 * time.Millisecond})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ge.Close() })

	type job struct {
		ID int `json:"id"`
	}
	got := make(chan string, 1)
	require.NoError(t, ge.OnReliable("jobs", func(payload string) {
		got <- payload
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.EmitReliable(ctx, "jobs", job{ID: 7}))
	var decoded job
	require.NoError(t, JSONCodec{}.Decode(waitFor(t, got), &decoded))
	assert.Equal(t, 7, decoded.ID)
}

func TestReliableMultipleHandlers(t *testing.T) {
	ge := newReliableEmitter(t, Config{})
	first := make(chan string, 1)
	second := make(chan string, 1)
	require.NoError(t, ge.OnReliable("jobs", func(payload string) { first <- payload }))
	require.NoError(t, ge.OnReliable("jobs", func(payload string) { second <- payload }))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.EmitReliable(ctx, "jobs", "work"))
	assert.Equal(t, "work", waitFor(t, first))
	assert.Equal(t, "work", waitFor(t, second))
}

func TestRemoveReliable(t *testing.T) {
	ge := newReliableEmitter(t, Config{})
	got := make(chan string, 1)
	require.NoError(t, ge.OnReliable("jobs", func(payload string) { got <- payload }))
	require.NoError(t, ge.RemoveReliable("jobs"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.EmitReliable(ctx, "jobs", "work"))
	select {
	case <-got:
		t.Fatal("handler must not be called after removal")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestOnReliableValidation(t *testing.T) {
	ge := newReliableEmitter(t, Config{})
	assert.Error(t, ge.OnReliable("", func(string) {}))
	assert.Error(t, ge.OnReliable("jobs", nil))
}

func TestReliableMaxStreamLength(t *testing.T) {
	ge := newReliableEmitter(t, Config{MaxStreamLength: 2})
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		require.NoError(t, ge.EmitReliable(ctx, "jobs", "work"))
	}
	assert.Eventually(t, func() bool {
		n, err := ge.client.XLen(ctx, "jobs").Result()
		return err == nil && n <= 3
	}, 2*time.Second, 20*time.Millisecond)
}

func TestReliableMaxListeners(t *testing.T) {
	ge := newReliableEmitter(t, Config{})
	ge.SetMaxListeners(1)
	require.NoError(t, ge.OnReliable("a", func(string) {}))
	assert.Error(t, ge.OnReliable("b", func(string) {}))
}

func TestEmitReliableEncodeError(t *testing.T) {
	ge := newReliableEmitter(t, Config{})
	assert.Error(t, ge.EmitReliable(context.Background(), "jobs", 42))
}

func TestReliableDoesNotAffectPubsub(t *testing.T) {
	ge := newReliableEmitter(t, Config{})
	pubsubGot := make(chan string, 1)
	reliableGot := make(chan string, 1)
	require.NoError(t, ge.On("live", func(payload string) { pubsubGot <- payload }))
	require.NoError(t, ge.OnReliable("jobs", func(payload string) { reliableGot <- payload }))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "live", "now"))
	assert.Equal(t, "now", waitFor(t, pubsubGot))
	select {
	case <-reliableGot:
		t.Fatal("pub/sub message must not reach reliable handler")
	case <-time.After(300 * time.Millisecond):
	}
}
