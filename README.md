# goemits [![Build Status](https://travis-ci.org/saromanov/goemits.svg?branch=master)](https://travis-ci.org/saromanov/goemits)
[![Coverage Status](https://coveralls.io/repos/saromanov/goemits/badge.svg?branch=master)](https://coveralls.io/r/saromanov/goemits?branch=master)

Event emitters based on pubsub in Redis.


```go
package main

import
(
	"github.com/saromanov/goemits"
	"fmt"
)

func main() {
	emit := goemits.Init()
	emit.On("connect", func(message string){
		fmt.Println("Found: ", message)
		emit.Emit("disconnect", "data")
	})

	emit.On("disconnect", func(message string){
		fmt.Println("Disconnect")
		emit.Quit()
	})

	emit.OnAny(func(message string){
		//This get any events
	})
	emit.StartLoop()
}
```

# API

## emit.RemoveListsner(listener string)

## emit.RemoveListeners(listener []string)

## emit.On(event, message string)
add new listener

## emit.OnAny(message string)
getting messages from all listeners

## emit.Emit(listener string, message string)
Fire message to listener

## emit.EmitMany(listeners []stirng, message string)
Fire message to list of listeners

## emit.EmitAll(message)
Fire message to all of listeners

## emit.SetMaxListeners(num int)
set maximum number of listeners
