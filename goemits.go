// Copyright 2019 Sergey Romanov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENCE file.

// Package goemits provides an event emitter backed by Redis pub/sub.
//
// Events are published to Redis channels with Emit and delivered to
// handlers registered with On and OnAny. Payloads are encoded with a
// pluggable codec (JSON by default), so events can be consumed from
// other languages as well.
//
// A minimal setup looks like this:
//
//	ge, err := goemits.New(goemits.Config{Address: "127.0.0.1:6379"})
//	if err != nil {
//		return err
//	}
//	defer ge.Close()
//
//	err = ge.On("user:created", func(payload string) {
//		fmt.Println("user created:", payload)
//	})
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	go func() {
//		if err := ge.Emit(ctx, "user:created", map[string]any{"id": 42}); err != nil {
//			log.Print(err)
//		}
//	}()
//
//	if err := ge.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
//		return err
//	}
package goemits

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
)

// handlerFunc receives a raw payload string for a subscribed event.
type handlerFunc func(payload string)

// anyHandlerFunc receives the event name and the raw payload for every
// event delivered through the wildcard subscription.
type anyHandlerFunc func(event string, payload string)

// Goemits is an event emitter backed by Redis pub/sub. Create it with
// New, register handlers with On and OnAny, publish events with Emit
// and run the delivery loop with Start.
type Goemits struct {
	client *redis.Client
	pubsub *redis.PubSub
	codec  Codec
	logger *slog.Logger

	handlers     map[string][]handlerFunc
	anyHandlers  []anyHandlerFunc
	listeners    []string
	maxListeners int

	m        sync.RWMutex
	started  atomic.Bool
	closed   atomic.Bool
	closeCtx context.CancelFunc
}

// New creates a Goemits instance with the given configuration. Redis
// connection is established lazily; use Ping to verify it.
func New(cfg Config) (*Goemits, error) {
	if cfg.Address == "" {
		cfg.Address = defaultRedisAddress
	}
	if cfg.Codec == nil {
		cfg.Codec = JSONCodec{}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	client := redis.NewClient(&redis.Options{
		Addr:      cfg.Address,
		Password:  cfg.Password,
		DB:        cfg.DB,
		TLSConfig: cfg.TLSConfig,
	})
	pubsub := client.Subscribe(context.Background())

	return &Goemits{
		client:       client,
		pubsub:       pubsub,
		codec:        cfg.Codec,
		logger:       logger,
		handlers:     make(map[string][]handlerFunc),
		maxListeners: cfg.MaxListeners,
	}, nil
}

// Ping verifies the Redis connection.
func (ge *Goemits) Ping(ctx context.Context) error {
	if err := ge.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("goemits: unable to check Redis connection: %w", err)
	}
	return nil
}

// On registers handler for the event and subscribes to its Redis
// channel. Multiple handlers per event are allowed. An error is
// returned if the maximum number of listeners is reached or if the
// subscription fails.
func (ge *Goemits) On(event string, handler func(payload string)) error {
	if event == "" {
		return errors.New("goemits: event name is empty")
	}
	if handler == nil {
		return errors.New("goemits: handler is nil")
	}

	ge.m.Lock()
	defer ge.m.Unlock()
	if _, ok := ge.handlers[event]; !ok {
		if ge.maxListeners > 0 && len(ge.handlers) >= ge.maxListeners {
			return fmt.Errorf("goemits: listener limit reached (%d)", ge.maxListeners)
		}
		ge.listeners = append(ge.listeners, event)
	}
	ge.handlers[event] = append(ge.handlers[event], handler)

	if err := ge.pubsub.Subscribe(context.Background(), event); err != nil {
		return fmt.Errorf("goemits: unable to subscribe to %q: %w", event, err)
	}
	return nil
}

// OnAny registers handler that receives every event published through
// this instance, including events subscribed by On. The handler
// receives the event name along with the payload.
func (ge *Goemits) OnAny(handler func(event string, payload string)) error {
	if handler == nil {
		return errors.New("goemits: handler is nil")
	}

	ge.m.Lock()
	defer ge.m.Unlock()
	ge.anyHandlers = append(ge.anyHandlers, handler)

	if err := ge.pubsub.PSubscribe(context.Background(), "*"); err != nil {
		return fmt.Errorf("goemits: unable to subscribe to any event: %w", err)
	}
	return nil
}

// Emit publishes the message to the event channel. The message is
// encoded with the configured codec (JSON by default).
func (ge *Goemits) Emit(ctx context.Context, event string, message any) error {
	payload, err := ge.codec.Encode(message)
	if err != nil {
		return fmt.Errorf("goemits: unable to encode message for %q: %w", event, err)
	}
	if err := ge.client.Publish(ctx, event, payload).Err(); err != nil {
		return fmt.Errorf("goemits: unable to publish %q: %w", event, err)
	}
	return nil
}

// EmitMany publishes the message to each of the events.
func (ge *Goemits) EmitMany(ctx context.Context, events []string, message any) error {
	var errs []error
	for _, event := range events {
		if err := ge.Emit(ctx, event, message); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// EmitAll publishes the message to every event registered with On.
func (ge *Goemits) EmitAll(ctx context.Context, message any) error {
	ge.m.RLock()
	events := make([]string, len(ge.listeners))
	copy(events, ge.listeners)
	ge.m.RUnlock()
	return ge.EmitMany(ctx, events, message)
}

// SetMaxListeners limits the number of distinct events that can be
// registered with On. Zero or a negative value means unlimited.
func (ge *Goemits) SetMaxListeners(num int) {
	ge.m.Lock()
	defer ge.m.Unlock()
	ge.maxListeners = num
}

// RemoveListener removes all handlers registered for the event and
// unsubscribes from its Redis channel. Removing an event that was
// never registered does nothing.
func (ge *Goemits) RemoveListener(event string) error {
	ge.m.Lock()
	if _, ok := ge.handlers[event]; !ok {
		ge.m.Unlock()
		return nil
	}
	delete(ge.handlers, event)
	for i, name := range ge.listeners {
		if name == event {
			ge.listeners = append(ge.listeners[:i], ge.listeners[i+1:]...)
			break
		}
	}
	ge.m.Unlock()

	if err := ge.pubsub.Unsubscribe(context.Background(), event); err != nil {
		return fmt.Errorf("goemits: unable to unsubscribe from %q: %w", event, err)
	}
	return nil
}

// RemoveListeners removes each of the events.
func (ge *Goemits) RemoveListeners(events []string) error {
	var errs []error
	for _, event := range events {
		if err := ge.RemoveListener(event); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Start runs the delivery loop and blocks until Close is called or
// ctx is cancelled. It returns nil after Close and context.Canceled on
// cancellation. Start can be called only once.
func (ge *Goemits) Start(ctx context.Context) error {
	if !ge.started.CompareAndSwap(false, true) {
		return errors.New("goemits: emitter is already started")
	}

	ctx, cancel := context.WithCancel(ctx)
	ge.m.Lock()
	ge.closeCtx = cancel
	ge.m.Unlock()
	defer func() {
		ge.m.Lock()
		ge.closeCtx = nil
		ge.m.Unlock()
	}()

	messages := ge.pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			if ge.closed.Load() {
				return nil
			}
			return ctx.Err()
		case msg, ok := <-messages:
			if !ok {
				return nil
			}
			ge.dispatch(msg)
		}
	}
}

// Close unsubscribes and releases the Redis connections. Close is
// idempotent; it also stops a running Start loop.
func (ge *Goemits) Close() error {
	if !ge.closed.CompareAndSwap(false, true) {
		return nil
	}
	ge.m.Lock()
	cancel := ge.closeCtx
	ge.closeCtx = nil
	ge.m.Unlock()
	if cancel != nil {
		cancel()
	}

	err := ge.pubsub.Close()
	if cerr := ge.client.Close(); err == nil {
		err = cerr
	}
	return err
}

// dispatch delivers the message to the registered handlers. Handler
// slices are copied under the lock, so handlers may call On, Emit and
// friends safely. A panic inside a handler is recovered and logged.
func (ge *Goemits) dispatch(msg *redis.Message) {
	ge.m.RLock()
	handlers := ge.handlers[msg.Channel]
	anyHandlers := ge.anyHandlers
	ge.m.RUnlock()

	payload := msg.Payload
	for _, h := range handlers {
		ge.safeCall(msg.Channel, func() { h(payload) })
	}
	for _, h := range anyHandlers {
		ge.safeCall(msg.Channel, func() { h(msg.Channel, payload) })
	}
}

func (ge *Goemits) safeCall(event string, call func()) {
	defer func() {
		if r := recover(); r != nil {
			ge.logger.Error("goemits: recovered panic from handler",
				slog.String("event", event), slog.Any("panic", r))
		}
	}()
	call()
}
