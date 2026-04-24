package hybridcommon

import "sync"

// GetOrCreateMapValue returns an existing map value for key or stores a new one.
func GetOrCreateMapValue[K comparable, V any](mu *sync.RWMutex, values map[K]V, key K, create func() V) V {
	mu.RLock()
	value, exists := values[key]
	mu.RUnlock()
	if exists {
		return value
	}

	mu.Lock()
	defer mu.Unlock()

	value, exists = values[key]
	if exists {
		return value
	}

	value = create()
	values[key] = value
	return value
}
