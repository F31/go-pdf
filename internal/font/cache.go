package font

import (
	"sync"

	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/syntax"
)

// Cache stores loaded fonts for the lifetime of a document. Fonts are
// immutable after Load except for the lazily populated Type3 glyph cache,
// which Font protects internally.
type Cache struct {
	mu      sync.Mutex
	items   map[syntax.Ref]*cacheEntry
	tracker *budget.Tracker
	closed  bool
}

type cacheEntry struct {
	font    *Font
	release func()
}

// NewCache creates a document-scoped font cache. A nil tracker disables
// retained-memory accounting.
func NewCache(tracker ...*budget.Tracker) *Cache {
	c := &Cache{items: map[syntax.Ref]*cacheEntry{}}
	if len(tracker) > 0 {
		c.tracker = tracker[0]
	}
	return c
}

func (c *Cache) Get(ref syntax.Ref) (*Font, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[ref]
	if !ok {
		return nil, false
	}
	return e.font, true
}

// Put caches a font and reserves its estimated retained bytes. It returns false
// when the live-memory budget is exhausted; the font remains usable but is not
// retained by the cache.
func (c *Cache) Put(ref syntax.Ref, f *Font) bool {
	if c == nil || f == nil || ref.Num == 0 {
		return false
	}
	var release func()
	if c.tracker != nil {
		r, err := c.tracker.Acquire(f.RetainedBytes())
		if err != nil {
			return false
		}
		release = r
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		if release != nil {
			release()
		}
		return false
	}
	if old, ok := c.items[ref]; ok {
		if old.release != nil {
			old.release()
		}
	}
	c.items[ref] = &cacheEntry{font: f, release: release}
	return true
}

// Close releases all retained-font leases. Idempotent.
func (c *Cache) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	for _, e := range c.items {
		if e.release != nil {
			e.release()
		}
	}
	c.items = map[syntax.Ref]*cacheEntry{}
}
