package goemits

// Config defines attributes for start
type Config struct {
	// RedisAddress defines address for connect to Redis
	RedisAddress string
	// Number of maximum listeners
	MaxListeners int
}
