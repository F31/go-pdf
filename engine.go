package pdf

import (
	"context"
	"io"
	"sync"

	"github.com/F31/go-pdf/internal/errs"
)

// Engine holds application-level defaults. It is safe for concurrent use and
// does not own opened Documents.
type Engine struct {
	mu     sync.Mutex
	opts   []Option
	closed bool
}

// NewEngine validates and stores default options for subsequent opens.
func NewEngine(opts ...Option) (*Engine, error) {
	if _, err := parseOptions(opts); err != nil {
		return nil, err
	}
	return &Engine{opts: append([]Option(nil), opts...)}, nil
}

// OpenFile opens a file using the engine defaults plus per-call options.
func (e *Engine) OpenFile(ctx context.Context, path string, opts ...Option) (*Document, error) {
	all, err := e.options(opts)
	if err != nil {
		return nil, err
	}
	return OpenFile(ctx, path, all...)
}

// Open opens a ReaderAt using the engine defaults plus per-call options.
func (e *Engine) Open(ctx context.Context, r io.ReaderAt, size int64, opts ...Option) (*Document, error) {
	all, err := e.options(opts)
	if err != nil {
		return nil, err
	}
	return Open(ctx, r, size, all...)
}

// Close prevents new documents from being opened through this engine. Already
// opened Documents retain their own lifecycle.
func (e *Engine) Close() error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	e.closed = true
	e.mu.Unlock()
	return nil
}

func (e *Engine) options(extra []Option) ([]Option, error) {
	if e == nil {
		return nil, errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "open via nil engine", -1, 0, -1)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, errs.Wrap(errs.ErrClosed, "DOC-CLOSED", "open via closed engine", -1, 0, -1)
	}
	all := make([]Option, 0, len(e.opts)+len(extra))
	all = append(all, e.opts...)
	all = append(all, extra...)
	return all, nil
}
