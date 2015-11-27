package goemits

import (
	"testing"
)

const (
	msg = "foobar"
)

func TestRunning(t *testing.T) {
	res := Init("localhost:6379")
	if res.isrunning != true {
		t.Errorf("Init is not started")
	}
	res.Quit()
}

func TestEmitEvent(t *testing.T) {
	res := Init("localhost:6380")
	value := ""
	msg := "foobar"
	res.On("test", func(message string) {
		value = message
		res.Quit()
	})
	err := res.Emit("test", msg)
	if err != nil {
		t.Errorf("%v", err)
	}
	res.Start()
	if value != msg {
		t.Errorf("%s not match %s", value, msg)
	}
}

func TestOnAny(t *testing.T) {
	emit := Init("localhost:6379")
	values := []string{}
	emit.On("test.first", func(message string) {
		emit.Quit()
	})

	emit.On("test.second", func(message string) {
		emit.Quit()
	})

	emit.OnAny(func(message string) {
		values = append(values, message)
	})

	var err error
	err = emit.Emit("test.first", "foobar")
	if err != nil {
		t.Errorf("%v", err)
	}
	err = emit.Emit("test.second", "foobar")
	if err != nil {
		t.Errorf("%v", err)
	}
	emit.Start()

	if len(values) != 2 {
		t.Errorf("%d not match %d", 2, len(values))
	}
}

func TestEmitMany(t *testing.T) {
	emit := Init("localhost:6379")
	values := []string{}
	emit.On("test.first", func(message string) {
		values = append(values, message)
	})

	emit.On("test.second", func(message string) {
		values = append(values, message)
		emit.Quit()
	})
	emit.EmitMany([]string{"test.first", "test.second"}, msg)
	emit.Start()
	size := len(values)
	if size != 2 {
		t.Errorf("%d not match %d", 2, size)
	}
}

func TestEmitAll(t *testing.T) {
	emit := Init("localhost:6379")
	values := []string{}
	emit.On("test.first", func(message string) {
		values = append(values, message)
	})

	emit.On("test.second", func(message string) {
		values = append(values, message)
		emit.Quit()
	})
	emit.EmitAll(msg)
	emit.Start()
	size := len(values)
	if size != 2 {
		t.Errorf("%d not match %d", 2, size)
	}
}

func TestMaxListeners(t *testing.T) {
	emit := Init("localhost:6379")
	emit.SetMaxListeners(2)
	emit.On("foobar", func(mesage string){
		size := len(emit.listeners)
		if size != 2 {
			t.Errorf("%d not match %d", size, 2)
		}
		emit.Quit()
	})
	emit.On("foobar2", func(mesage string){
		
	})
	emit.On("foobar3", func(mesage string){
		
	})

	emit.Emit("foobar", "A")
	emit.Start()

}

func TestRemoveListener(t *testing.T) {
	emit := Init("localhost:6379")
	emit.On("foo", func(message string) {
		emit.RemoveListener("foo")
		emit.Quit()
		if len(emit.listeners) != 0 {
			t.Errorf("%d not match %d", 0, len(emit.listeners))
		}
	})

	emit.Emit("foo", "bar")
	emit.Start()
}

func TestRemoveListeners(t *testing.T) {
	emit := Init("localhost:6379")
	emit.On("foo", func(message string) {
		emit.RemoveListeners([]string{"foo", "value"})
		emit.Quit()
		if len(emit.listeners) != 0 {
			t.Errorf("%d not match %d", 0, len(emit.listeners))
		}
	})
	emit.On("value", func(message string) {})

	emit.Emit("foo", "bar")
	emit.Start()
}

func TestQuit(t *testing.T) {
	emit := Init("localhost:6379")
	emit.On("foo", func(message string) {
		emit.Quit()
	})
	emit.Emit("foo", "nn")
	emit.Start()
	if emit.isrunning {
		t.Errorf("goemits must be stops")
	}
}
