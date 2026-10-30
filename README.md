# goemits

[![CI](https://github.com/saromanov/goemits/actions/workflows/test.yml/badge.svg)](https://github.com/saromanov/goemits/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/saromanov/goemits/v2.svg)](https://pkg.go.dev/github.com/saromanov/goemits/v2)
[![Go Report Card](https://goreportcard.com/badge/github.com/saromanov/goemits/v2)](https://goreportcard.com/report/github.com/saromanov/goemits/v2)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)

Event emitter based on pub/sub in Redis.

## About

goemits lets you publish and consume events through Redis pub/sub with a
small, Go-idiomatic API:

- `On` registers a handler for an event; multiple handlers per event are
  allowed.
- `OnAny` receives every event with its name, so you can build audit or
  debug listeners.
- `Emit` publishes a message; payloads are encoded with a pluggable codec
  (JSON by default), so events are consumable from any language.
- Contexts are first-class: publish with cancellation, stop the delivery
  loop with a context or `Close`.

## Install

```
go get github.com/saromanov/goemits/v2
```

## Quick start

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/saromanov/goemits/v2"
)

func main() {
	ge, err := goemits.New(goemits.Config{Address: "127.0.0.1:6379"})
	if err != nil {
		log.Fatal(err)
	}
	defer ge.Close()

	err = ge.On("connect", func(payload string) {
		fmt.Println("connect:", payload)
	})
	if err != nil {
		log.Fatal(err)
	}

	err = ge.On("disconnect", func(payload string) {
		fmt.Println("disconnect")
		ge.Close()
	})
	if err != nil {
		log.Fatal(err)
	}

	err = ge.OnAny(func(event, payload string) {
		fmt.Printf("any: %s -> %s\n", event, payload)
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := ge.Emit(ctx, "connect", "hello"); err != nil {
			log.Print(err)
		}
	}()

	if err := ge.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}
```

## Configuration

| Field           | Description                                                         | Default             |
|-----------------|---------------------------------------------------------------------|---------------------|
| `Address`       | Redis address in the `host:port` form                               | `127.0.0.1:6379`    |
| `Password`      | Redis password, if required                                         | empty               |
| `DB`            | Redis database to select                                            | `0`                 |
| `TLSConfig`     | TLS configuration for encrypted connections                         | nil (no TLS)        |
| `MaxListeners`  | Maximum number of distinct events registered with `On`; 0 = no limit| `0`                 |
| `Codec`         | Payload encoder/decoder                                             | `JSONCodec{}`       |
| `Logger`        | `*slog.Logger` for internal diagnostics                             | discards everything |

## Codecs

`JSONCodec` (default) encodes messages as JSON, so events can be consumed
from Python, Node.js or any other language. `RawCodec` passes strings
through unchanged for producers that do their own serialization. Any
custom encoding can be plugged in by implementing the two-method `Codec`
interface.

## Error handling

All methods return errors instead of panicking. `EmitMany`, `EmitAll` and
`RemoveListeners` join the errors of individual operations with
`errors.Join`. Panics raised by user handlers are recovered and logged,
so a broken handler cannot take down the delivery loop.

## Testing

Unit tests run against an in-process Redis ([miniredis]) and need no
external services:

```
go test -race ./...
```

Integration tests require a running Redis server:

```
GOEMITS_TEST_REDIS=127.0.0.1:6379 go test ./...
```

## License

MIT — see [LICENCE](LICENCE).

[miniredis]: https://github.com/alicebob/miniredis
