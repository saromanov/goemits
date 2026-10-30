// Copyright 2019 Sergey Romanov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENCE file.

package goemits

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration runs against a real Redis server when the
// GOEMITS_TEST_REDIS environment variable is set to its address, e.g.
// GOEMITS_TEST_REDIS=127.0.0.1:6379.
func TestIntegration(t *testing.T) {
	addr := os.Getenv("GOEMITS_TEST_REDIS")
	if addr == "" {
		t.Skip("GOEMITS_TEST_REDIS is not set")
	}

	ge, err := New(Config{Address: addr})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ge.Close() })
	require.NoError(t, ge.Ping(context.Background()))

	got := make(chan string, 1)
	anyGot := make(chan string, 1)
	patternGot := make(chan string, 1)
	require.NoError(t, ge.On("integration", func(payload string) {
		got <- payload
	}))
	require.NoError(t, ge.OnAny(func(event, payload string) {
		anyGot <- event
	}))
	require.NoError(t, ge.OnPattern("integration:*", func(event, payload string) {
		patternGot <- event
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "integration", "hello"))
	require.NoError(t, ge.Emit(ctx, "integration:created", "evt"))

	select {
	case payload := <-got:
		var s string
		require.NoError(t, JSONCodec{}.Decode(payload, &s))
		assert.Equal(t, "hello", s)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for exact handler")
	}

	select {
	case event := <-anyGot:
		assert.Equal(t, "integration", event)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for any handler")
	}

	select {
	case event := <-patternGot:
		assert.Equal(t, "integration:created", event)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for pattern handler")
	}
}
