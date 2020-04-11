package goemits

import (
	"testing"

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
}

func TestEmitEvent(t *testing.T) {
	res := New(Config{
		RedisAddress: host,
	})
	value := ""
	msg := "foobar"
	res.On("test", func(message string) {
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

	assert.NoError(t, emit.Emit("test.first", "foobar"))
	assert.NoError(t, emit.Emit("test.second", "foobar"))
	emit.Start()
	assert.Equal(t, 2, len(values))
}

func TestEmitMany(t *testing.T) {
	emit := New(Config{
		RedisAddress: host,
	})
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
	assert.Equal(t, 2, len(values))
}

func TestEmitAll(t *testing.T) {
	emit := New(Config{
		RedisAddress: host,
	})
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
	assert.Equal(t, 2, len(values))
}

func TestMaxListeners(t *testing.T) {
	emit := New(Config{
		RedisAddress: host,
	})
	emit.SetMaxListeners(2)
	emit.On("foobar", func(mesage string) {
		size := len(emit.listeners)
		assert.Equal(t, 2, size)
		emit.Quit()
	})
	emit.On("foobar2", func(mesage string) {

	})
	emit.On("foobar3", func(mesage string) {

	})

	emit.Emit("foobar", "A")
	emit.Start()

}

func TestRemoveListener(t *testing.T) {
	emit := New(Config{
		RedisAddress: host,
	})
	emit.On("foo", func(message string) {
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
	emit.On("foo", func(message string) {
		emit.RemoveListeners([]string{"foo", "value"})
		emit.Quit()
		assert.Equal(t, 0, len(emit.listeners))
	})
	emit.On("value", func(message string) {})

	emit.Emit("foo", "bar")
	emit.Start()
}

func TestQuit(t *testing.T) {
	emit := New(Config{
		RedisAddress: host,
	})
	emit.On("foo", func(message string) {
		emit.Quit()
	})
	emit.Emit("foo", "nn")
	emit.Start()
	assert.Error(t, emit.Quit())
}
