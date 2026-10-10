package auth

import "sync"

// AuthCache is a concurrent key-value store used by Lua scripts
// to persist tokens across requests.
type AuthCache interface {
	Get(key string) (string, bool)
	Set(key, value string)
}

type memoryCache struct {
	mu   sync.RWMutex
	data map[string]string
}

func newMemoryCache(initial map[string]string) *memoryCache {
	c := &memoryCache{
		data: make(map[string]string, len(initial)),
	}
	for k, v := range initial {
		c.data[k] = v
	}
	return c
}

func (c *memoryCache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.data[key]
	return v, ok
}

func (c *memoryCache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}
