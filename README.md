# goemits
[![Documentation](https://godoc.org/github.com/saromanov/goemits?status.svg)](http://godoc.org/github.com/saromanov/goemits)
[![Go Report Card](https://goreportcard.com/badge/github.com/saromanov/goemits)](https://goreportcard.com/report/github.com/saromanov/goemits)
[![Build Status](https://travis-ci.org/saromanov/goemits.svg?branch=master)](https://travis-ci.org/saromanov/goemits) [![Coverage Status](https://coveralls.io/repos/saromanov/goemits/badge.svg?branch=master)](https://coveralls.io/r/saromanov/goemits?branch=master)

Event emitters based on pubsub in Redis.


```go
package main

import (
	"fmt"

	"github.com/saromanov/goemits"
)

func main() {
	emit := goemits.New(goemits.Config{
		RedisAddress: "127.0.0.1:6379",
	})
	emit.On("connect", func(message interface{}) {
		fmt.Println("Found: ", message)
		emit.Emit("disconnect", "data")
	})

	emit.On("disconnect", func(message interface{}) {
		fmt.Println("Disconnect")
		emit.Quit()
	})
	

	emit.OnAny(func(message interface{}) {
		//This get any events
	})
	emit.Start()
}

```

Emit of the event
```go
emit.On("disconnect", "now")
```
