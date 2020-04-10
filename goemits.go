package goemits

import (
	"fmt"
	"log"
	"sync"
	"time"

	"gopkg.in/redis.v3"
)

//Goemits defines main structure of the app
type Goemits struct {
	//main client
	client *redis.Client
	//pubsub object
	subclient *redis.PubSub
	//all of listeners
	listeners []string
	//triggers for events
	handlers map[string]func(string)
	//check if goemits is running
	isRunning    bool
	anyListener  bool
	maxListeners int
	m            *sync.Mutex
}

//New provides initialization of Goemits
//This should call in the first place
func New(c Config) *Goemits {
	if c.RedisAddress == "" {
		c.RedisAddress = "127.0.0.1:6379"
	}
	ge := new(Goemits)
	ge.client = initRedis(c.RedisAddress)
	ge.subclient = initRedis(c.RedisAddress).PubSub()
	ge.handlers = map[string]func(string){}
	ge.isRunning = true
	ge.maxListeners = c.MaxListeners
	ge.m = &sync.Mutex{}
	return ge
}

//On provides subscribe to event
func (ge *Goemits) On(event string, f func(string)) {
	ge.m.Lock()
	defer ge.m.Unlock()
	liscount := len(ge.handlers)
	if ge.maxListeners > 0 && liscount > 0 && liscount == ge.maxListeners {
		log.Println("Can't add new listener, cause limit of listeners")
		return
	}
	_, ok := ge.handlers[event]
	if ok {
		return
	}
	ge.listeners = append(ge.listeners, event)
	ge.handlers[event] = f
	ge.subscribe(event)
}

//OnAny provides catching any event
func (ge *Goemits) OnAny(f func(string)) {
	ge.m.Lock()
	defer ge.m.Unlock()
	ge.handlers["_any"] = f
	ge.anyListener = true
}

//Emit event
func (ge *Goemits) Emit(event, message string) error {
	err := ge.client.Publish(event, message).Err()
	if err != nil {
		return fmt.Errorf("unable to publish message: %v", err)
	}
	return nil
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
	ge.maxListeners = num
}

//RemoveListener from store and unsubscribe from "listener" channel
func (ge *Goemits) RemoveListener(listener string) {
	ge.m.Lock()
	defer ge.m.Unlock()
	_, ok := ge.handlers[listener]
	if !ok {
		return
	}
	delete(ge.handlers, listener)
	idx := ge.findListener(listener)
	ge.listeners = append(ge.listeners[:idx], ge.listeners[idx+1:]...)
	ge.subclient.Unsubscribe(listener)
}

func (ge *Goemits) findListener(targlistener string) int {
	res := -1
	for i, listener := range ge.listeners {
		if listener == targlistener {
			return i
		}
	}
	return res
}

//RemoveListeners from the base
func (ge *Goemits) RemoveListeners(listeners []string) {
	for _, listener := range listeners {
		ge.RemoveListener(listener)
	}
}

//Quit provides break up main loop
func (ge *Goemits) Quit() {
	ge.client.Close()
	ge.isRunning = false
}

func initRedis(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "",
		DB:       0,
	})
}

//This method gets messages from redis
func (ge *Goemits) receiveMessages() (interface{}, error) {
	return ge.subclient.ReceiveTimeout(100 * time.Millisecond)
}

//Subscribe to another event
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
			return
		}
		switch msg := msgi.(type) {
		case *redis.Message:
			h, ok := ge.handlers[msg.Channel]
			if ok {
				h(msg.Payload)
			}

			if ge.anyListener {
				h := ge.handlers["_any"]
				h(msg.Payload)
			}
		}
	}
}

//Start provides beginning of catching messages
func (ge *Goemits) Start() {
	go ge.startMessagesLoop()
	for {
		if !ge.isRunning {
			break
		}
		time.Sleep(time.Millisecond * 10)
	}
}
