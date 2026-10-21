// Package budget provides the document-wide resource limits defined by the
// go-pdf design. Limits are immutable; a Tracker owns the mutable counters for
// one document and is safe for concurrent use.
package budget

import (
	"context"
	"runtime"
	"sync/atomic"

	"github.com/F31/go-pdf/internal/errs"
)

const (
	MiB = 1 << 20
	GiB = 1 << 30
)

// Budget is the set of limits applied to a single Document.
type Budget struct {
	MaxFileSize      int64
	MaxObjects       int64
	MaxStreamBytes   int64
	MaxTotalDecoded  int64
	MaxInflateRatio  float64
	MaxXRefChain     int
	MaxPageTreeDepth int
	MaxFormDepth     int
	MaxNestingDepth  int
	MaxOperators     int64
	MaxPathSegments  int64
	MaxImagePixels   int64
	MaxFunctionSteps int64
	MaxConcurrency   int
	MaxRasterPixels  int64
}

// DefaultBudget is the starting point from design section 15.1. Values are
// expected to be calibrated against the corpus and frozen in a later phase.
var DefaultBudget = Budget{
	MaxFileSize:      512 * MiB,
	MaxObjects:       5_000_000,
	MaxStreamBytes:   256 * MiB,
	MaxTotalDecoded:  2 * GiB,
	MaxInflateRatio:  200,
	MaxXRefChain:     256,
	MaxPageTreeDepth: 64,
	MaxFormDepth:     16,
	MaxNestingDepth:  64,
	MaxOperators:     20_000_000,
	MaxPathSegments:  5_000_000,
	MaxImagePixels:   256 * 1_000_000,
	MaxFunctionSteps: 100_000,
	MaxConcurrency:   runtime.GOMAXPROCS(0),
	MaxRasterPixels:  64 * 1_000_000,
}

// Clone returns a copy so callers cannot mutate a shared budget.
func (b Budget) Clone() Budget { return b }

type trackerKey struct{}
type budgetKey struct{}

// WithBudget attaches a budget to ctx.
func WithBudget(ctx context.Context, b *Budget) context.Context {
	return context.WithValue(ctx, budgetKey{}, b)
}

// From returns the budget attached to ctx or DefaultBudget.
func From(ctx context.Context) *Budget {
	if b, ok := ctx.Value(budgetKey{}).(*Budget); ok && b != nil {
		return b
	}
	return &DefaultBudget
}

// WithTracker attaches a tracker to ctx for internal layers.
func WithTracker(ctx context.Context, t *Tracker) context.Context {
	return context.WithValue(ctx, trackerKey{}, t)
}

// TrackerFrom returns the tracker attached to ctx, if any.
func TrackerFrom(ctx context.Context) *Tracker {
	t, _ := ctx.Value(trackerKey{}).(*Tracker)
	return t
}

// Tracker holds mutable counters for one document. All methods are safe for
// concurrent use.
type Tracker struct {
	budget  *Budget
	objects atomic.Int64
	decoded atomic.Int64
}

func NewTracker(b *Budget) *Tracker {
	if b == nil {
		b = &DefaultBudget
	}
	return &Tracker{budget: b}
}

func (t *Tracker) Budget() *Budget { return t.budget }

func (t *Tracker) Objects() int64 { return t.objects.Load() }
func (t *Tracker) Decoded() int64 { return t.decoded.Load() }

// CheckFileSize enforces MaxFileSize.
func (t *Tracker) CheckFileSize(n int64) error {
	if n < 0 {
		return errs.Wrap(errs.ErrCorrupt, "SYN-FILE-SIZE", "check file size", -1, 0, -1)
	}
	if n > t.budget.MaxFileSize {
		return errs.Wrap(errs.ErrBudget, "BUD-FILE-SIZE", "check file size", -1, 0, -1)
	}
	return nil
}

// AddObjects reserves n object slots.
func (t *Tracker) AddObjects(n int64) error {
	if n <= 0 {
		return nil
	}
	if t.objects.Add(n) > t.budget.MaxObjects {
		return errs.Wrap(errs.ErrBudget, "BUD-OBJECTS", "reserve objects", -1, 0, -1)
	}
	return nil
}

// AddDecoded accounts for n decoded bytes and enforces MaxTotalDecoded.
func (t *Tracker) AddDecoded(n int64) error {
	if n <= 0 {
		return nil
	}
	if t.decoded.Add(n) > t.budget.MaxTotalDecoded {
		return errs.Wrap(errs.ErrBudget, "BUD-TOTAL-DECODED", "decode stream", -1, 0, -1)
	}
	return nil
}

// CheckStreamLen validates a single stream's declared output length.
func (t *Tracker) CheckStreamLen(n int64) error {
	if n > t.budget.MaxStreamBytes {
		return errs.Wrap(errs.ErrBudget, "BUD-STREAM-BYTES", "decode stream", -1, 0, -1)
	}
	return nil
}

// CheckInflateRatio validates encoded/decoded byte counts.
func (t *Tracker) CheckInflateRatio(encoded, decoded int64) error {
	if encoded <= 0 || t.budget.MaxInflateRatio <= 0 {
		return nil
	}
	if float64(decoded) > float64(encoded)*t.budget.MaxInflateRatio {
		return errs.Wrap(errs.ErrBudget, "BUD-INFLATE-RATIO", "decode stream", -1, 0, -1)
	}
	return nil
}

// CheckNesting enforces MaxNestingDepth.
func (t *Tracker) CheckNesting(depth int) error {
	if depth > t.budget.MaxNestingDepth {
		return errs.Wrap(errs.ErrBudget, "BUD-NESTING-DEPTH", "parse object", -1, 0, -1)
	}
	return nil
}
