package goemits

import (
	"github.com/fzzy/radix/extra/pubsub"
	"github.com/fzzy/radix/redis"
	"time"
	"sync"
	//"fmt"
)

type Goemits struct {
	client    *redis.Client
	subclient *redis.Client
	pubs      *pubsub.SubClient
	listeners []string
	handlers  map[string]func(string)
	isrunning bool
	syncdata* sync.Mutex

}

func Init() *Goemits {
	ge := new(Goemits)
	ge.client = initRedis()
	ge.subclient = initRedis()
	ge.pubs = pubsub.NewSubClient(ge.subclient)
	ge.handlers = map[string]func(string){}
	ge.isrunning = true
	ge.syncdata = &sync.Mutex{}
	return ge
}

func (ge *Goemits) AddListener(listener string) {
	ge.listeners = append(ge.listeners, listener)

}

func (ge *Goemits) Emit(event, message string) {
	ge.client.Cmd("publish", event, message)
}

func (ge *Goemits) On(event string, f func(string)) {
	ge.handlers[event] = f
	ge.subscribe(event)
	go ge.startMessagesLoop()
}

func (ge *Goemits) Quit() {
	ge.isrunning = false
}

func initRedis() *redis.Client {
	client, err := redis.DialTimeout("tcp", "127.0.0.1:6379", time.Duration(10)*time.Second)
	if err != nil {
		panic("Can't initialize redis client")
	}
	return client
}

func (ge *Goemits) receiveMessages() *pubsub.SubReply {
	return ge.pubs.Receive()
}

func (ge* Goemits) subscribe(event string){
	sr := ge.pubs.Subscribe(event)
	if sr.Err != nil {
		panic(sr.Err)
	}
}

func (ge *Goemits) startMessagesLoop() {
	for {
			reply := ge.receiveMessages()
			if reply.Type != 0 {
				hand, ok := ge.handlers[reply.Channel]
				if ok {
					hand(reply.Message)
				}
			}
	}
}

func (ge *Goemits) StartLoop() {
	for {
		if !ge.isrunning {
			break
		}
		time.Sleep(time.Millisecond * 10)
	}
}
