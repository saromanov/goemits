package goemits

import (
	"fmt"
	"gopkg.in/redis.v3"
	"sync"
	"time"
)

//Goemits provides main structure
type Goemits struct {
	//main client
	client *redis.Client
	//pubsub obecjt
	subclient    *redis.PubSub
	listeners    []string
	handlers     map[string]func(string)
	isrunning    bool
	anylistener  bool
	maxlisteners int
	syncdata     *sync.Mutex
}

//Init provides initialization of Goemis
//This should call in the first place
func Init(addr string) *Goemits {
	ge := new(Goemits)
	ge.client = initRedis(addr)
	ge.subclient = initRedis(addr).PubSub()
	ge.handlers = map[string]func(string){}
	ge.isrunning = true
	ge.syncdata = &sync.Mutex{}
	return ge
}

//On provides subscribe to event
func (ge *Goemits) On(event string, f func(string)) {
	liscount := len(ge.handlers)
	if liscount > 0 && liscount == ge.maxlisteners {
		fmt.Println("Can't add new listener, cause limit of listeners")
	}
	_, ok := ge.handlers[event]
	if !ok {
		ge.listeners = append(ge.listeners, event)
		ge.handlers[event] = f
		ge.subscribe(event)
	}
}

//OnAny provides catching any event
func (ge *Goemits) OnAny(f func(string)) {
	ge.handlers["_any"] = f
	ge.anylistener = true
}

//Emit event
func (ge *Goemits) Emit(event, message string) {
	err := ge.client.Publish(event, message).Err()
	if err != nil {
		panic(err)
	}
}

//EmitMany provides fire message to list of listeners
func (ge *Goemits) EmitMany(events []string, message string) {
	for _, listener := range events {
		ge.Emit(listener, message)
	}
}

//EmitAll provides fire message to all of listeners
func (ge *Goemits) EmitAll(message string) {
	for _, listener := range ge.listeners {
		ge.Emit(listener, message)
	}
}

//SetMaxListeners provides limitiation of amount of listeners
func (ge *Goemits) SetMaxListeners(num int) {
	ge.maxlisteners = num
}

//RemoveListener from store and unsubscribe from "listener" channel
func (ge *Goemits) RemoveListener(listener string) {
	_, ok := ge.handlers[listener]
	if ok {
		delete(ge.handlers, listener)
		ge.subclient.Unsubscribe(listener)
	}
}

//RemoveListeners from the base
func (ge *Goemits) RemoveListeners(listeners []string) {
	for _, listener := range listeners {
		ge.RemoveListener(listener)
	}
}

//Quit provides break up main loop
func (ge *Goemits) Quit() {
	ge.isrunning = false
}

func initRedis(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "",
		DB:       0,
	})
}

func (ge *Goemits) receiveMessages() (interface{}, error) {
	return ge.subclient.ReceiveTimeout(100 * time.Millisecond)
}

func (ge *Goemits) subscribe(event string) {
	err := ge.subclient.Subscribe(event)
	if err != nil {
		panic(err)
	}
}

func (ge *Goemits) startMessagesLoop() {
	for {
		msgi, err := ge.receiveMessages()
		if err != nil {
			//panic(err)
		}
		switch msg := msgi.(type) {
		case *redis.Message:
			hand, ok := ge.handlers[msg.Channel]
			if ok {
				hand(msg.Payload)
			}

			if ge.anylistener {
				handfunc, _ := ge.handlers["_any"]
				handfunc(msg.Payload)
			}
		}
	}
}

//Start provides beginning of catching messages
func (ge *Goemits) Start() {
	go ge.startMessagesLoop()
	defer ge.subclient.Close()
	for {
		if !ge.isrunning {
			break
		}
		time.Sleep(time.Millisecond * 10)
	}
}
