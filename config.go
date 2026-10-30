// Copyright 2019 Sergey Romanov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENCE file.

package goemits

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

const defaultRedisAddress = "127.0.0.1:6379"

// Default values for reliable delivery configuration.
const (
	DefaultConsumerGroup = "goemits"
	DefaultConsumerName  = "goemits"
	DefaultClaimIdle     = time.Minute
	DefaultClaimInterval = 30 * time.Second
	DefaultPollInterval  = 100 * time.Millisecond
	defaultPayloadField  = "payload"
)

// Config defines attributes for the emitter.
type Config struct {
	// Address of the Redis server in the "host:port" form.
	// Defaults to 127.0.0.1:6379.
	Address string
	// Password for the Redis server, if required.
	Password string
	// DB is the Redis database to select after connecting.
	DB int
	// TLSConfig enables TLS connections to the Redis server.
	TLSConfig *tls.Config
	// MaxListeners limits the number of distinct events that can be
	// registered with On and OnPattern. Zero or a negative value means
	// unlimited.
	MaxListeners int
	// Codec encodes and decodes event payloads. Defaults to
	// JSONCodec.
	Codec Codec
	// Logger receives internal diagnostics, e.g. recovered handler
	// panics. Defaults to a logger that discards everything.
	Logger *slog.Logger

	// ConsumerGroup is the Redis Streams consumer group used for
	// reliable delivery. Defaults to "goemits".
	ConsumerGroup string
	// ConsumerName is the consumer name within the consumer group.
	// Defaults to "goemits".
	ConsumerName string
	// ClaimIdle is how long a pending reliable message must stay
	// unacknowledged before another emitter instance may reclaim it.
	// Defaults to time.Minute.
	ClaimIdle time.Duration
	// ClaimInterval is how often an emitter looks for stale pending
	// reliable messages. Defaults to 30s.
	ClaimInterval time.Duration
	// PollInterval is how often an emitter checks reliable streams for
	// new messages. Defaults to 100ms.
	PollInterval time.Duration
	// MaxStreamLength caps the length of reliable streams with an
	// approximate trim. Zero or a negative value means unlimited.
	MaxStreamLength int64
}

// Codec encodes outgoing messages and decodes incoming payloads.
//
// The default JSONCodec marshals values to JSON strings, which makes
// events consumable from any language with a JSON parser.
type Codec interface {
	// Encode converts the message into a string payload.
	Encode(message any) (string, error)
	// Decode parses the payload into the value pointed to by v.
	Decode(payload string, v any) error
}

// JSONCodec is the default Codec. It encodes messages as JSON strings.
type JSONCodec struct{}

// Encode marshals the message to a JSON string.
func (JSONCodec) Encode(message any) (string, error) {
	data, err := json.Marshal(message)
	if err != nil {
		return "", fmt.Errorf("goemits: unable to marshal message: %w", err)
	}
	return string(data), nil
}

// Decode unmarshals the JSON payload into the value pointed to by v.
func (JSONCodec) Decode(payload string, v any) error {
	if err := json.Unmarshal([]byte(payload), v); err != nil {
		return fmt.Errorf("goemits: unable to unmarshal payload: %w", err)
	}
	return nil
}

// RawCodec passes string messages through without any conversion. Use
// it when events are already serialized by the producer or consumed as
// opaque strings.
type RawCodec struct{}

// Encode returns the string message as is.
func (RawCodec) Encode(message any) (string, error) {
	payload, ok := message.(string)
	if !ok {
		return "", fmt.Errorf("goemits: RawCodec expects a string message, got %T", message)
	}
	return payload, nil
}

// Decode copies the payload into the string value pointed to by v.
func (RawCodec) Decode(payload string, v any) error {
	target, ok := v.(*string)
	if !ok {
		return fmt.Errorf("goemits: RawCodec expects a *string target, got %T", v)
	}
	*target = payload
	return nil
}
