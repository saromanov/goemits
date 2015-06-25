package goemits

import (
	//"fmt"
	"gopkg.in/redis.v3"
	"sync"
	"time"
)

//Goemits provides main structure
type Goemits struct {
	client    *redis.Client
	subclient *redis.PubSub
	listeners []string
	handlers  map[string]func(string)
	isrunning bool
	syncdata  *sync.Mutex
}

//Init provides initialization of Goemis
//This should call in the first place
func Init() *Goemits {
	ge := new(Goemits)
	ge.client = initRedis()
	ge.subclient = initRedis().PubSub()
	ge.handlers = map[string]func(string){}
	ge.isrunning = true
	ge.syncdata = &sync.Mutex{}
	return ge
}

func (ge *Goemits) AddListener(listener string) {
	ge.listeners = append(ge.listeners, listener)

}

//RemoveListener from store and unsubscribe from "listener" channel
func (ge *Goemits) RemoveListener(listener string) {
	_, ok := ge.handlers[listener]
	if ok {
		delete(ge.handlers, listener)
		ge.subclient.Unsubscribe(listener)
	}
}

//Emit event
func (ge *Goemits) Emit(event, message string) {
	err := ge.client.Publish(event, message).Err()
	if err != nil {
		panic(err)
	}
}

func (ge *Goemits) On(event string, f func(string)) {
	ge.handlers[event] = f
	ge.subscribe(event)
}

func (ge *Goemits) Quit() {
	ge.isrunning = false
}

func initRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
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
		}
	}
}

func (ge *Goemits) StartLoop() {
	go ge.startMessagesLoop()
	defer ge.subclient.Close()
	for {
		if !ge.isrunning {
			break
		}
		time.Sleep(time.Millisecond * 10)
	}
}
