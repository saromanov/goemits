// Copyright 2019 Sergey Romanov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENCE file.

package goemits

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// slogError builds a structured error attribute for the logger.
func slogError(err error) slog.Attr {
	return slog.Any("error", err)
}

// OnReliable registers handler for the event and guarantees
// at-least-once delivery backed by a Redis Stream. Messages published
// with EmitReliable are stored in the stream, so an emitter that starts
// later still receives them; unacknowledged messages are reclaimed by
// other instances after Config.ClaimIdle. Multiple handlers per event
// are allowed.
//
// Handlers must be side-effect safe against redelivery: a message is
// acknowledged after all registered handlers have run, and is
// redelivered after a crash in between.
func (ge *Goemits) OnReliable(event string, handler func(payload string)) error {
	if event == "" {
		return errors.New("goemits: event name is empty")
	}
	if handler == nil {
		return errors.New("goemits: handler is nil")
	}

	ge.m.Lock()
	defer ge.m.Unlock()
	if _, ok := ge.reliableHandlers[event]; !ok {
		if ge.maxListeners > 0 && len(ge.handlers)+len(ge.patternHandlers)+len(ge.reliableHandlers) >= ge.maxListeners {
			return fmt.Errorf("goemits: listener limit reached (%d)", ge.maxListeners)
		}
	}
	ge.reliableHandlers[event] = append(ge.reliableHandlers[event], handler)

	if err := ge.ensureGroup(context.Background(), event); err != nil {
		return fmt.Errorf("goemits: unable to create consumer group for %q: %w", event, err)
	}
	return nil
}

// EmitReliable publishes the message to the event stream with
// at-least-once delivery semantics. The message is encoded with the
// configured codec (JSON by default).
func (ge *Goemits) EmitReliable(ctx context.Context, event string, message any) error {
	payload, err := ge.codec.Encode(message)
	if err != nil {
		return fmt.Errorf("goemits: unable to encode message for %q: %w", event, err)
	}
	args := &redis.XAddArgs{
		Stream: event,
		Values: map[string]any{defaultPayloadField: payload},
	}
	if ge.maxStreamLength > 0 {
		args.MaxLen = ge.maxStreamLength
		args.Approx = true
	}
	if err := ge.client.XAdd(ctx, args).Err(); err != nil {
		return fmt.Errorf("goemits: unable to publish %q: %w", event, err)
	}
	return nil
}

// RemoveReliable removes all handlers registered for the event with
// OnReliable. The stream and the consumer group are kept.
func (ge *Goemits) RemoveReliable(event string) error {
	ge.m.Lock()
	defer ge.m.Unlock()
	delete(ge.reliableHandlers, event)
	return nil
}

// reliableEvents returns the streams with registered handlers.
func (ge *Goemits) reliableEvents() []string {
	ge.m.RLock()
	events := make([]string, 0, len(ge.reliableHandlers))
	for event := range ge.reliableHandlers {
		events = append(events, event)
	}
	ge.m.RUnlock()
	return events
}

// ensureGroup creates the consumer group for the stream, reading from
// the beginning of the stream, unless it already exists.
func (ge *Goemits) ensureGroup(ctx context.Context, event string) error {
	err := ge.client.XGroupCreateMkStream(ctx, event, ge.group, "0").Err()
	if err != nil && strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}
	return err
}

// reliableLoop reads new messages from all reliable streams and
// periodically reclaims stale pending messages. It runs until ctx is
// cancelled.
func (ge *Goemits) reliableLoop(ctx context.Context) {
	claimTicker := time.NewTicker(ge.claimInterval)
	defer claimTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-claimTicker.C:
			ge.claimStale(ctx)
		default:
		}

		if !ge.readNew(ctx) {
			return
		}
	}
}

// readNew reads a batch of new messages from every reliable stream. It
// blocks for up to the poll interval when there are no messages and
// reports whether the loop may continue.
func (ge *Goemits) readNew(ctx context.Context) bool {
	events := ge.reliableEvents()
	if len(events) == 0 {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(ge.pollInterval):
			return true
		}
	}

	streams := make([]string, 0, len(events)*2)
	for _, event := range events {
		streams = append(streams, event, ">")
	}
	res, err := ge.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    ge.group,
		Consumer: ge.consumer,
		Streams:  streams,
		Count:    16,
		Block:    ge.pollInterval,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) || ctx.Err() != nil {
			return ctx.Err() == nil
		}
		ge.logger.Warn("goemits: unable to read reliable streams", slogError(err))
		return ctx.Err() == nil
	}

	for _, stream := range res {
		for _, msg := range stream.Messages {
			ge.deliverReliable(ctx, stream.Stream, msg)
		}
	}
	return true
}

// claimStale reclaims pending messages that stayed unacknowledged for
// longer than the claim idle time and delivers them again.
func (ge *Goemits) claimStale(ctx context.Context) {
	for _, event := range ge.reliableEvents() {
		msgs, _, err := ge.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   event,
			Group:    ge.group,
			Consumer: ge.consumer,
			MinIdle:  ge.claimIdle,
			Start:    "0-0",
			Count:    16,
		}).Result()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			ge.logger.Warn("goemits: unable to reclaim messages", slogError(err))
			continue
		}
		for _, msg := range msgs {
			ge.deliverReliable(ctx, event, msg)
		}
	}
}

// deliverReliable runs all handlers registered for the stream with the
// message payload and acknowledges the message afterwards.
func (ge *Goemits) deliverReliable(ctx context.Context, event string, msg redis.XMessage) {
	ge.m.RLock()
	handlers := ge.reliableHandlers[event]
	ge.m.RUnlock()

	payload, _ := msg.Values[defaultPayloadField].(string)
	for _, h := range handlers {
		ge.safeCall(event, func() { h(payload) })
	}
	if err := ge.client.XAck(ctx, event, ge.group, msg.ID).Err(); err != nil {
		ge.logger.Warn("goemits: unable to acknowledge message", slogError(err))
	}
}
