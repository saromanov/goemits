// Copyright 2019 Sergey Romanov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENCE file.

package goemits

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type eventDelivery struct {
	event   string
	payload string
}

func newTestEmitter(t *testing.T) *Goemits {
	t.Helper()
	mr := miniredis.RunT(t)
	ge, err := New(Config{Address: mr.Addr(), Codec: RawCodec{}})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ge.Close()
	})
	return ge
}

func waitFor[T any](t *testing.T, ch chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
		var zero T
		return zero
	}
}

func TestNewDefaults(t *testing.T) {
	ge, err := New(Config{})
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:6379", ge.client.Options().Addr)
	require.NoError(t, ge.Close())
}

func TestPing(t *testing.T) {
	ge := newTestEmitter(t)
	require.NoError(t, ge.Ping(context.Background()))

	bad, err := New(Config{Address: "127.0.0.1:1"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = bad.Close() })
	assert.Error(t, bad.Ping(context.Background()))
}

func TestOnDeliversPayload(t *testing.T) {
	ge := newTestEmitter(t)
	got := make(chan string, 1)
	require.NoError(t, ge.On("test", func(payload string) {
		got <- payload
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "test", "foobar"))
	assert.Equal(t, "foobar", waitFor(t, got))

	cancel()
	assert.ErrorIs(t, <-done, context.Canceled)
}

func TestOnDeliversJSONStruct(t *testing.T) {
	mr := miniredis.RunT(t)
	ge, err := New(Config{Address: mr.Addr()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ge.Close() })

	type user struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	got := make(chan string, 1)
	require.NoError(t, ge.On("user:created", func(payload string) {
		got <- payload
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "user:created", user{ID: 42, Name: "alice"}))

	payload := waitFor(t, got)
	var decoded user
	require.NoError(t, JSONCodec{}.Decode(payload, &decoded))
	assert.Equal(t, user{ID: 42, Name: "alice"}, decoded)
}

func TestMultipleHandlersPerEvent(t *testing.T) {
	ge := newTestEmitter(t)
	first := make(chan string, 1)
	second := make(chan string, 1)
	require.NoError(t, ge.On("evt", func(payload string) { first <- payload }))
	require.NoError(t, ge.On("evt", func(payload string) { second <- payload }))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "evt", "payload"))
	assert.Equal(t, "payload", waitFor(t, first))
	assert.Equal(t, "payload", waitFor(t, second))
}

func TestDuplicateHandler(t *testing.T) {
	ge := newTestEmitter(t)
	calls := make(chan string, 4)
	handler := func(payload string) { calls <- payload }
	require.NoError(t, ge.On("evt", handler))
	require.NoError(t, ge.On("evt", handler))
	assert.Len(t, ge.handlers["evt"], 2)
}

func TestOnPatternDelivery(t *testing.T) {
	ge := newTestEmitter(t)
	got := make(chan eventDelivery, 2)
	require.NoError(t, ge.OnPattern("user:*", func(event, payload string) {
		got <- eventDelivery{event, payload}
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "user:created", "alice"))
	require.NoError(t, ge.Emit(ctx, "user:deleted", "bob"))

	d := waitFor(t, got)
	assert.Equal(t, "user:created", d.event)
	assert.Equal(t, "alice", d.payload)
	d = waitFor(t, got)
	assert.Equal(t, "user:deleted", d.event)
	assert.Equal(t, "bob", d.payload)
}

func TestOnPatternDoesNotMatchOtherEvents(t *testing.T) {
	ge := newTestEmitter(t)
	got := make(chan eventDelivery, 1)
	require.NoError(t, ge.OnPattern("user:*", func(event, payload string) {
		got <- eventDelivery{event, payload}
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "order:created", "x"))
	select {
	case <-got:
		t.Fatal("pattern handler must not be called for non-matching event")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestOnPatternAndExactBothFire(t *testing.T) {
	ge := newTestEmitter(t)
	got := make(chan string, 4)
	require.NoError(t, ge.OnPattern("user:*", func(event, payload string) { got <- "pattern:" + payload }))
	require.NoError(t, ge.On("user:created", func(payload string) { got <- "exact:" + payload }))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "user:created", "data"))
	first := waitFor(t, got)
	second := waitFor(t, got)
	assert.ElementsMatch(t, []string{"exact:data", "pattern:data"}, []string{first, second})
}

func TestRemovePattern(t *testing.T) {
	ge := newTestEmitter(t)
	got := make(chan eventDelivery, 1)
	require.NoError(t, ge.OnPattern("user:*", func(event, payload string) {
		got <- eventDelivery{event, payload}
	}))
	require.NoError(t, ge.RemovePattern("user:*"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "user:created", "ignored"))
	select {
	case <-got:
		t.Fatal("pattern handler must not be called after removal")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestOnPatternMultipleHandlers(t *testing.T) {
	ge := newTestEmitter(t)
	first := make(chan string, 1)
	second := make(chan string, 1)
	require.NoError(t, ge.OnPattern("evt:*", func(_, payload string) { first <- payload }))
	require.NoError(t, ge.OnPattern("evt:*", func(_, payload string) { second <- payload }))
	assert.Len(t, ge.patternHandlers["evt:*"], 2)
}

func TestOnPatternValidation(t *testing.T) {
	ge := newTestEmitter(t)
	assert.Error(t, ge.OnPattern("", func(string, string) {}))
	assert.Error(t, ge.OnPattern("user:*", nil))
}

func TestMaxListenersCountsPatterns(t *testing.T) {
	ge := newTestEmitter(t)
	ge.SetMaxListeners(2)
	require.NoError(t, ge.On("a", func(string) {}))
	require.NoError(t, ge.OnPattern("b:*", func(string, string) {}))
	assert.Error(t, ge.OnPattern("c:*", func(string, string) {}))
}

func TestOnAnyReceivesEventName(t *testing.T) {
	ge := newTestEmitter(t)
	got := make(chan eventDelivery, 2)
	require.NoError(t, ge.OnAny(func(event, payload string) {
		got <- eventDelivery{event, payload}
	}))
	require.NoError(t, ge.On("only.exact", func(payload string) {}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "only.exact", "data"))

	d := waitFor(t, got)
	assert.Equal(t, "only.exact", d.event)
	assert.Equal(t, "data", d.payload)
}

func TestMaxListeners(t *testing.T) {
	ge := newTestEmitter(t)
	ge.SetMaxListeners(1)
	require.NoError(t, ge.On("first", func(string) {}))
	assert.Error(t, ge.On("second", func(string) {}))
}

func TestRemoveListener(t *testing.T) {
	ge := newTestEmitter(t)
	got := make(chan string, 1)
	require.NoError(t, ge.On("evt", func(payload string) { got <- payload }))
	require.NoError(t, ge.RemoveListener("evt"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "evt", "ignored"))
	select {
	case <-got:
		t.Fatal("handler must not be called after removal")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestEmitMany(t *testing.T) {
	ge := newTestEmitter(t)
	got := make(chan string, 4)
	require.NoError(t, ge.On("a", func(payload string) { got <- "a:" + payload }))
	require.NoError(t, ge.On("b", func(payload string) { got <- "b:" + payload }))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.EmitMany(ctx, []string{"a", "b"}, "data"))
	assert.Equal(t, "a:data", waitFor(t, got))
	assert.Equal(t, "b:data", waitFor(t, got))
}

func TestEmitAll(t *testing.T) {
	ge := newTestEmitter(t)
	got := make(chan string, 4)
	require.NoError(t, ge.On("a", func(payload string) { got <- "a:" + payload }))
	require.NoError(t, ge.On("b", func(payload string) { got <- "b:" + payload }))
	require.NoError(t, ge.RemoveListener("b"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.EmitAll(ctx, "data"))
	assert.Equal(t, "a:data", waitFor(t, got))
	select {
	case <-got:
		t.Fatal("removed listener must not receive EmitAll")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestEmitEncodeError(t *testing.T) {
	ge := newTestEmitter(t)
	require.Error(t, ge.Emit(context.Background(), "evt", make(chan int)))
}

func TestCustomCodec(t *testing.T) {
	mr := miniredis.RunT(t)
	ge, err := New(Config{Address: mr.Addr(), Codec: prefixCodec{}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ge.Close() })

	got := make(chan string, 1)
	require.NoError(t, ge.On("evt", func(payload string) { got <- payload }))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "evt", "value"))
	assert.Equal(t, "encoded:value", waitFor(t, got))
}

type prefixCodec struct{}

func (prefixCodec) Encode(message any) (string, error) {
	return "encoded:" + strings.TrimSpace(anyToString(message)), nil
}

func (prefixCodec) Decode(payload string, v any) error {
	return JSONCodec{}.Decode(strings.TrimPrefix(payload, "encoded:"), v)
}

func anyToString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return "unknown"
}

func TestStartTwice(t *testing.T) {
	ge := newTestEmitter(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()
	require.Eventually(t, func() bool { return ge.started.Load() }, time.Second, 5*time.Millisecond)
	assert.Error(t, ge.Start(context.Background()))
}

func TestCloseStopsStart(t *testing.T) {
	ge := newTestEmitter(t)
	done := make(chan error, 1)
	go func() { done <- ge.Start(context.Background()) }()
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, ge.Close())
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after Close")
	}
	require.NoError(t, ge.Close())
}

func TestCloseIdempotent(t *testing.T) {
	ge := newTestEmitter(t)
	require.NoError(t, ge.Close())
	require.NoError(t, ge.Close())
}

func TestOnAfterClose(t *testing.T) {
	ge := newTestEmitter(t)
	require.NoError(t, ge.Close())
	assert.Error(t, ge.On("evt", func(string) {}))
}

func TestOnValidation(t *testing.T) {
	ge := newTestEmitter(t)
	assert.Error(t, ge.On("", func(string) {}))
	assert.Error(t, ge.On("evt", nil))
	assert.Error(t, ge.OnAny(nil))
}

func TestHandlerPanicRecovered(t *testing.T) {
	ge := newTestEmitter(t)
	require.NoError(t, ge.On("boom", func(string) { panic("handler failed") }))
	got := make(chan string, 1)
	require.NoError(t, ge.On("alive", func(payload string) { got <- payload }))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- ge.Start(ctx) }()

	require.NoError(t, ge.Emit(ctx, "boom", "x"))
	require.NoError(t, ge.Emit(ctx, "alive", "still running"))
	assert.Equal(t, "still running", waitFor(t, got))

	cancel()
	assert.ErrorIs(t, <-done, context.Canceled)
}

func TestConcurrentUsage(t *testing.T) {
	ge := newTestEmitter(t)
	got := make(chan string, 8)
	require.NoError(t, ge.On("concurrent", func(payload string) { got <- payload }))
	require.NoError(t, ge.OnAny(func(event, payload string) { got <- payload }))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ge.Start(ctx) }()

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 2; j++ {
				assert.NoError(t, ge.Emit(ctx, "concurrent", "payload"))
			}
		}()
	}
	wg.Wait()

	for i := 0; i < 8; i++ {
		assert.Equal(t, "payload", waitFor(t, got))
	}
}

func TestListenerLimitZeroMeansUnlimited(t *testing.T) {
	ge := newTestEmitter(t)
	for _, event := range []string{"a", "b", "c", "d", "e"} {
		require.NoError(t, ge.On(event, func(string) {}))
	}
}

func TestStartReturnsErrorWhenPubsubClosed(t *testing.T) {
	ge := newTestEmitter(t)
	require.NoError(t, ge.Close())
	done := make(chan error, 1)
	go func() { done <- ge.Start(context.Background()) }()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after pubsub close")
	}
}

func TestEmitManyPartialFailure(t *testing.T) {
	ge := newTestEmitter(t)
	err := ge.EmitMany(context.Background(), []string{"a"}, 42)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to encode")
}

func TestRawCodec(t *testing.T) {
	ge := newTestEmitter(t)
	var out string
	require.NoError(t, RawCodec{}.Decode("payload", &out))
	assert.Equal(t, "payload", out)
	assert.Error(t, RawCodec{}.Decode("payload", new(int)))
	assert.Error(t, ge.Emit(context.Background(), "evt", 42))
}
