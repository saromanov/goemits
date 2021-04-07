package goemits

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/go-redis/redis/v8"
)

//Goemits defines main structure of the app
type Goemits struct {
	//main client
	client *redis.Client
	//pubsub object
	pubsub *redis.PubSub
	//all of listeners
	listeners []string
	//triggers for events
	handlers map[string]func(interface{})
	//check if goemits is running
	quit         chan struct{}
	started      bool
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

	return &Goemits{
		client:       initRedis(c.RedisAddress),
		pubsub:       initRedis(c.RedisAddress).Subscribe(context.Background()),
		handlers:     map[string]func(interface{}){},
		quit:         make(chan struct{}),
		maxListeners: c.MaxListeners,
		m:            &sync.Mutex{},
	}
}

// Ping provides checking of redis connection and Goemits
func (ge *Goemits) Ping() error {
	if err := ge.client.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("unable to check Redis connection: %v", err)
	}

	return nil
}

//On provides subscribe to event
func (ge *Goemits) On(event string, f func(interface{})) error {
	ge.m.Lock()
	defer ge.m.Unlock()
	liscount := len(ge.handlers)

	if ge.maxListeners > 0 && liscount > 0 && liscount == ge.maxListeners {
		return fmt.Errorf("Can't add new listener, max limit of listeners %d has been reached", ge.maxListeners)
	}
	
	// event already in the handlers
	_, ok := ge.handlers[event]
	if ok {
		return nil
	}

	ge.listeners = append(ge.listeners, event)
	ge.handlers[event] = f
	ge.subscribe(event)
	return nil
}

//OnAny provides catching any event
func (ge *Goemits) OnAny(f func(interface{})) {
	ge.m.Lock()
	defer ge.m.Unlock()
	ge.handlers["_any"] = f
	ge.anyListener = true
}

//Emit event
func (ge *Goemits) Emit(event string, message interface{}) error {
	err := ge.client.Publish(context.Background(), event, message).Err()
	if err != nil {
		return fmt.Errorf("unable to publish message: %v", err)
	}
	return nil
}

//EmitMany provides fire message to list of listeners
func (ge *Goemits) EmitMany(events []string, message interface{}) {
	for _, listener := range events {
		ge.Emit(listener, message) // nolint
	}
}

//EmitAll provides fire message to all of listeners
func (ge *Goemits) EmitAll(message interface{}) {
	for _, listener := range ge.listeners {
		ge.Emit(listener, message) // nolint
	}
}

//SetMaxListeners provides limitiation of amount of listeners
func (ge *Goemits) SetMaxListeners(num int) {
	ge.maxListeners = num
}

//RemoveListener from store and unsubscribe from "listener" channel
func (ge *Goemits) RemoveListener(listener string) error {
	ge.m.Lock()
	defer ge.m.Unlock()
	// if listener is not registered, just return
	_, ok := ge.handlers[listener]
	if !ok {
		return nil
	}
	delete(ge.handlers, listener)
	idx := ge.findListener(listener)
	ge.listeners = append(ge.listeners[:idx], ge.listeners[idx+1:]...)
	if err := ge.pubsub.Unsubscribe(context.Background(), listener); err != nil {
		return err
	}
	return nil
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
func (ge *Goemits) Quit() error {
	if err := ge.client.Close(); err != nil {
		return fmt.Errorf("unable to close Redis connection: %v", err)
	}
	if !ge.started {
		return fmt.Errorf("app is not started")
	}
	t := func() error {
		ge.quit <- struct{}{}
		return nil
	}

	return t()
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
	return ge.pubsub.ReceiveMessage(context.TODO())
}

//Subscribe to another event
func (ge *Goemits) subscribe(event string) {
	err := ge.pubsub.Subscribe(context.Background(), event)
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
		switch msg := msgi.(type) { //nolint
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
	ge.started = true
	go ge.startMessagesLoop()
	for range ge.quit {
		close(ge.quit)
	}
}
