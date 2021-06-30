package memory

import (
	"github.com/bradfitz/gomemcache/memcache"
)

// Errors that may be raised during config parsing.
type MemoryError string

func (e MemoryError) Error() string {
	return string(e)
}

var _ Client = (*memcache.Client)(nil)

// Interface for memory, used for mocking.
type Client interface {
	GetMulti(keys []string) (map[string]*memcache.Item, error)
	Increment(key string, delta uint64) (newValue uint64, err error)
	Add(item *memcache.Item) error
}
