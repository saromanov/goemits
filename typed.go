// Copyright 2019 Sergey Romanov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENCE file.

package goemits

import (
	"errors"
	"log/slog"
)

// OnTyped registers a typed handler for the event. The payload is
// decoded into T with the emitter's codec, so handlers work with real
// values instead of type assertions:
//
//	type user struct {
//		ID   int    `json:"id"`
//		Name string `json:"name"`
//	}
//
//	err := goemits.OnTyped(ge, "user:created", func(u user) {
//		fmt.Println(u.Name)
//	})
//
// The codec must be able to decode into arbitrary values: the default
// JSONCodec works, while RawCodec (string pass-through) does not.
// Events whose payload fails to decode are logged and skipped. Multiple
// typed handlers per event are allowed.
func OnTyped[T any](ge *Goemits, event string, handler func(payload T)) error {
	if handler == nil {
		return errNilHandler()
	}
	return ge.On(event, typedHandler(ge, handler))
}

// OnReliableTyped is OnTyped for reliable delivery: the handler
// receives a decoded value and the message follows the at-least-once
// semantics of OnReliable.
func OnReliableTyped[T any](ge *Goemits, event string, handler func(payload T)) error {
	if handler == nil {
		return errNilHandler()
	}
	return ge.OnReliable(event, typedHandler(ge, handler))
}

// typedHandler wraps a typed handler into a raw payload handler.
func typedHandler[T any](ge *Goemits, handler func(payload T)) handlerFunc {
	return func(raw string) {
		var value T
		if err := ge.codec.Decode(raw, &value); err != nil {
			ge.logger.Warn("goemits: unable to decode event payload",
				slogError(err), slog.String("payload", raw))
			return
		}
		handler(value)
	}
}

func errNilHandler() error {
	return errors.New("goemits: handler is nil")
}
