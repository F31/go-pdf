package pdf

import "github.com/F31/go-pdf/internal/budget"

const (
	MiB = 1 << 20
	GiB = 1 << 30
)

// Budget mirrors the resource limits from design section 15.1.
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

// DefaultBudget is the initial limit set. Values are calibrated against the
// corpus before release.
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
	MaxConcurrency:   budget.DefaultBudget.MaxConcurrency,
	MaxRasterPixels:  64 * 1_000_000,
}

func (b Budget) internal() *budget.Budget {
	return &budget.Budget{
		MaxFileSize: b.MaxFileSize, MaxObjects: b.MaxObjects,
		MaxStreamBytes: b.MaxStreamBytes, MaxTotalDecoded: b.MaxTotalDecoded,
		MaxInflateRatio: b.MaxInflateRatio, MaxXRefChain: b.MaxXRefChain,
		MaxPageTreeDepth: b.MaxPageTreeDepth, MaxFormDepth: b.MaxFormDepth,
		MaxNestingDepth: b.MaxNestingDepth, MaxOperators: b.MaxOperators,
		MaxPathSegments: b.MaxPathSegments, MaxImagePixels: b.MaxImagePixels,
		MaxFunctionSteps: b.MaxFunctionSteps, MaxConcurrency: b.MaxConcurrency,
		MaxRasterPixels: b.MaxRasterPixels,
	}
}

type options struct {
	repair   bool
	budget   *budget.Budget
	password string
	hidden   bool
	forms    bool
}

type Option func(*options) error

func WithRepair(v bool) Option {
	return func(o *options) error { o.repair = v; return nil }
}

func WithBudget(b *Budget) Option {
	return func(o *options) error {
		if b != nil {
			o.budget = b.internal()
		}
		return nil
	}
}

func WithPassword(p string) Option {
	return func(o *options) error { o.password = p; return nil }
}

func WithHiddenLayers(v bool) Option {
	return func(o *options) error { o.hidden = v; return nil }
}

func WithPreserveForms(v bool) Option {
	return func(o *options) error { o.forms = v; return nil }
}

func parseOptions(opts []Option) (options, error) {
	o := options{budget: DefaultBudget.internal()}
	for _, fn := range opts {
		if fn == nil {
			continue
		}
		if err := fn(&o); err != nil {
			return o, err
		}
	}
	return o, nil
}
