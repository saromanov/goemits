# goemits

Experimental
Work in progress

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

## emit.AddListener(listener string)
add new listener

## emit.On(event, message string)

## emit.OnAny(message string)

## emit.SetMaxListeners(num int)
