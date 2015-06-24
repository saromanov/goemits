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
	emit.StartLoop()
}
```
