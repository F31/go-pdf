package font

import (
	"sync"

	"github.com/F31/go-pdf/internal/syntax"
)

// Cache stores loaded fonts for the lifetime of a document. Fonts are
// immutable after Load except for the lazily populated Type3 glyph cache,
// which Font protects internally.
type Cache struct {
	mu    sync.Mutex
	items map[syntax.Ref]*Font
}

func NewCache() *Cache {
	return &Cache{items: map[syntax.Ref]*Font{}}
}

func (c *Cache) Get(ref syntax.Ref) (*Font, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	f, ok := c.items[ref]
	return f, ok
}

func (c *Cache) Put(ref syntax.Ref, f *Font) {
	if c == nil || f == nil || ref.Num == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[ref] = f
}
