// Package diag collects structured diagnostics shared by every layer.
package diag

import (
	"context"
	"strings"
	"sync"
)

type Level int

const (
	Info Level = iota
	Warn
	Degraded
	Error
)

func (l Level) String() string {
	switch l {
	case Info:
		return "info"
	case Warn:
		return "warn"
	case Degraded:
		return "degraded"
	case Error:
		return "error"
	default:
		return "unknown"
	}
}

// Diagnostic is a single structural observation. Msg must never contain
// document text, metadata values, or decoded stream contents.
type Diagnostic struct {
	Level  Level
	Code   string
	Msg    string
	Page   int
	Obj    int
	Offset int64
}

var validPrefixes = map[string]bool{
	"SYN": true, "FLT": true, "CRY": true, "DOC": true, "CNT": true,
	"FNT": true, "CLR": true, "IMG": true, "BUD": true,
}

// ValidCode reports whether code uses one of the documented prefixes.
func ValidCode(code string) bool {
	i := strings.IndexByte(code, '-')
	if i <= 0 {
		return false
	}
	return validPrefixes[code[:i]]
}

const DefaultCapacity = 10_000

type Collector struct {
	mu       sync.Mutex
	items    []Diagnostic
	capacity int
	dropped  int
}

func New(capacity int) *Collector {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Collector{capacity: capacity, items: make([]Diagnostic, 0, 32)}
}

type collectorKey struct{}

func WithCollector(ctx context.Context, c *Collector) context.Context {
	return context.WithValue(ctx, collectorKey{}, c)
}

func From(ctx context.Context) *Collector {
	c, _ := ctx.Value(collectorKey{}).(*Collector)
	return c
}

// Add records d. When the collector is full the lowest-severity existing item
// is evicted so that Error diagnostics are retained as long as possible.
func (c *Collector) Add(d Diagnostic) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) < c.capacity {
		c.items = append(c.items, d)
		return
	}
	c.dropped++
	if d.Level == Error {
		if i := c.lowest(); i >= 0 {
			c.items[i] = d
			return
		}
	}
	if d.Level >= c.items[len(c.items)-1].Level {
		c.items[len(c.items)-1] = d
	}
}

func (c *Collector) lowest() int {
	best := -1
	for i := range c.items {
		if best < 0 || c.items[i].Level < c.items[best].Level {
			best = i
		}
	}
	return best
}

func (c *Collector) Addf(level Level, code, msg string, page, obj int, offset int64) {
	c.Add(Diagnostic{Level: level, Code: code, Msg: msg, Page: page, Obj: obj, Offset: offset})
}

func (c *Collector) All() []Diagnostic {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Diagnostic, len(c.items))
	copy(out, c.items)
	return out
}

func (c *Collector) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

func (c *Collector) Dropped() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dropped
}

// AddContext records a diagnostic into the collector attached to ctx, if any.
func AddContext(ctx context.Context, d Diagnostic) {
	From(ctx).Add(d)
}

// ForPage returns diagnostics for one page without copying the full list.
func (c *Collector) ForPage(page int) []Diagnostic {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Diagnostic, 0, 8)
	for _, d := range c.items {
		if d.Page == page {
			out = append(out, d)
		}
	}
	return out
}
