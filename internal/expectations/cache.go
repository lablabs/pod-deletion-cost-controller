package expectations

import (
	"sync"
)

// NewCache create new cache
func NewCache[K comparable, V any]() *Cache[K, V] {
	return &Cache[K, V]{
		data: make(map[K]V),
	}
}

// Cache for sync with Api server
type Cache[K comparable, V any] struct {
	mu   sync.RWMutex
	data map[K]V
}

// Get gets element
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.data[key]
	return val, ok
}

// Has verify if element is presented
func (c *Cache[K, V]) Has(key K) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exist := c.data[key]
	return exist
}

// GetList return list of values base on keys
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

// Set sets new key, value pair
func (c *Cache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}

// Delete deletes key form cache
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
}
