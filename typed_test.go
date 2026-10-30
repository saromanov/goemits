// Copyright 2019 Sergey Romanov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENCE file.

package goemits

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testUser struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func TestOnTypedDelivery(t *testing.T) {
	mr := miniredis.RunT(t)
	ge, err := New(Config{Address: mr.Addr()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ge.Close() })

	got := make(chan testUser, 1)
	require.NoError(t, OnTyped(ge, "user:created", func(u testUser) {
		got <- u
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "user:created", testUser{ID: 42, Name: "alice"}))
	assert.Equal(t, testUser{ID: 42, Name: "alice"}, waitFor(t, got))
}

func TestOnTypedInference(t *testing.T) {
	mr := miniredis.RunT(t)
	ge, err := New(Config{Address: mr.Addr()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ge.Close() })

	got := make(chan int, 1)
	require.NoError(t, OnTyped(ge, "count", func(n int) { got <- n }))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "count", 7))
	assert.Equal(t, 7, waitFor(t, got))
}

func TestOnTypedSkipsUndecodablePayload(t *testing.T) {
	mr := miniredis.RunT(t)
	ge, err := New(Config{Address: mr.Addr()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ge.Close() })

	got := make(chan int, 1)
	require.NoError(t, OnTyped(ge, "count", func(n int) { got <- n }))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "count", "not-a-number"))
	require.NoError(t, ge.Emit(ctx, "count", 5))
	assert.Equal(t, 5, waitFor(t, got))
}

func TestOnReliableTypedDelivery(t *testing.T) {
	ge := newReliableEmitter(t, Config{Codec: JSONCodec{}})
	got := make(chan testUser, 1)
	require.NoError(t, OnReliableTyped(ge, "users", func(u testUser) {
		got <- u
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.EmitReliable(ctx, "users", testUser{ID: 1, Name: "bob"}))
	assert.Equal(t, testUser{ID: 1, Name: "bob"}, waitFor(t, got))
}

func TestOnTypedValidation(t *testing.T) {
	ge := newTestEmitter(t)
	assert.Error(t, OnTyped(ge, "", func(testUser) {}))
	assert.Error(t, OnTyped[testUser](ge, "users", nil))
	assert.Error(t, OnReliableTyped[testUser](ge, "users", nil))
}

func TestOnTypedMultipleHandlers(t *testing.T) {
	mr := miniredis.RunT(t)
	ge, err := New(Config{Address: mr.Addr()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ge.Close() })

	type nameOnly struct {
		Name string `json:"name"`
	}
	first := make(chan testUser, 1)
	second := make(chan nameOnly, 1)
	require.NoError(t, OnTyped(ge, "user:created", func(u testUser) { first <- u }))
	require.NoError(t, OnTyped(ge, "user:created", func(n nameOnly) { second <- n }))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "user:created", testUser{ID: 2, Name: "carol"}))
	assert.Equal(t, testUser{ID: 2, Name: "carol"}, waitFor(t, first))
	assert.Equal(t, nameOnly{Name: "carol"}, waitFor(t, second))
}

func TestOnTypedPanicRecovered(t *testing.T) {
	mr := miniredis.RunT(t)
	ge, err := New(Config{Address: mr.Addr()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ge.Close() })

	require.NoError(t, OnTyped(ge, "boom", func(u testUser) { panic("typed handler failed") }))
	got := make(chan testUser, 1)
	require.NoError(t, OnTyped(ge, "alive", func(u testUser) { got <- u }))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "boom", testUser{ID: 3, Name: "dave"}))
	require.NoError(t, ge.Emit(ctx, "alive", testUser{ID: 4, Name: "eve"}))
	assert.Equal(t, testUser{ID: 4, Name: "eve"}, waitFor(t, got))

	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
