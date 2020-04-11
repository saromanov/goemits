package goemits

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	msg  = "foobar"
	host = "127.0.0.1:6379"
)

func TestRunning(t *testing.T) {
	res := New(Config{
		RedisAddress: host,
	})
	assert.NoError(t, res.Ping())
	assert.Error(t, res.Quit())
	res2 := New(Config{})
	assert.NoError(t, res2.Ping())
	res3 := New(Config{
		RedisAddress: host + "222",
	})
	assert.Error(t, res3.Ping())
}

func TestEmitEvent(t *testing.T) {
	res := New(Config{
		RedisAddress: host,
	})
	var value interface{}
	msg := "foobar"
	res.On("test", func(message interface{}) {
		value = message
		res.Quit()
	})
	assert.NoError(t, res.Emit("test", msg))
	res.Start()
	assert.Equal(t, msg, value)
}

func TestOnAny(t *testing.T) {
	emit := New(Config{
		RedisAddress: host,
	})
	values := []interface{}{}
	emit.On("test.first", func(message interface{}) {
		emit.Quit()
	})

	emit.On("test.second", func(message interface{}) {
		emit.Quit()
	})

	emit.OnAny(func(message interface{}) {
		values = append(values, message)
	})
	time.Sleep(1 * time.Second)
	assert.NoError(t, emit.Emit("test.first", "foobar"))
	assert.NoError(t, emit.Emit("test.second", "foobar"))
	emit.Start()
	assert.Equal(t, 2, len(values))
}

func TestEmitMany(t *testing.T) {
	emit := New(Config{
		RedisAddress: host,
	})
	values := []interface{}{}
	emit.On("test.first", func(message interface{}) {
		values = append(values, message)
	})

	emit.On("test.second", func(message interface{}) {
		values = append(values, message)
		emit.Quit()
	})
	emit.EmitMany([]string{"test.first", "test.second"}, msg)
	emit.Start()
	time.Sleep(1 * time.Second)
	assert.Equal(t, 2, len(values))
}

func TestEmitAll(t *testing.T) {
	emit := New(Config{
		RedisAddress: host,
	})
	values := []interface{}{}
	emit.On("test.first", func(message interface{}) {
		values = append(values, message)
	})

	emit.On("test.second", func(message interface{}) {
		values = append(values, message)
		emit.Quit()
	})
	emit.EmitAll(msg)
	emit.Start()
	assert.Equal(t, 2, len(values))
}

func TestMaxListeners(t *testing.T) {
	emit := New(Config{
		RedisAddress: host,
	})
	emit.SetMaxListeners(2)
	emit.On("foobar", func(mesage interface{}) {
		size := len(emit.listeners)
		assert.Equal(t, 2, size)
		emit.Quit()
	})
	emit.On("foobar2", func(mesage interface{}) {

	})
	emit.On("foobar3", func(mesage interface{}) {

	})

	emit.Emit("foobar", "A")
	emit.Start()

}

func TestRemoveListener(t *testing.T) {
	emit := New(Config{
		RedisAddress: host,
	})
	emit.On("foo", func(message interface{}) {
		emit.RemoveListener("foo")
		emit.Quit()
		assert.Equal(t, 0, len(emit.listeners))
	})

	emit.Emit("foo", "bar")
	emit.Start()
}

func TestRemoveListeners(t *testing.T) {
	emit := New(Config{
		RedisAddress: host,
	})
	emit.On("foo", func(message interface{}) {
		emit.RemoveListeners([]string{"foo", "value"})
		emit.Quit()
		assert.Equal(t, 0, len(emit.listeners))
	})
	emit.On("value", func(message interface{}) {})

	emit.Emit("foo", "bar")
	emit.Start()
}

func TestQuit(t *testing.T) {
	emit := New(Config{
		RedisAddress: host,
	})
	emit.On("foo", func(message interface{}) {
		emit.Quit()
	})
	emit.Emit("foo", "nn")
	emit.Start()
	assert.Error(t, emit.Quit())
}
