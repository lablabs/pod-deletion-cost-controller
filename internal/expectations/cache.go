package expectations

import (
	"sync"
)

func NewCache[K comparable, V any]() *Cache[K, V] {
	return &Cache[K, V]{
		data: make(map[K]V),
	}
}

type Cache[K comparable, V any] struct {
	mu   sync.RWMutex
	data map[K]V
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.data[key]
	return val, ok
}

func (c *Cache[K, V]) Has(key K) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exist := c.data[key]
	return exist
}

func (c *Cache[K, V]) GetList(keys ...K) []V {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]V, 0)
	for _, key := range keys {
		if val, ok := c.data[key]; ok {
			out = append(out, val)
		}
	}
	return out
}

func (c *Cache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}

func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
}
