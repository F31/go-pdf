// Package filter implements the stream filters required by the G0 phase.
package filter

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/errs"
)

// Params is a normalized filter parameter dictionary. Keys are lower-case PDF
// names without the leading slash.
type Params map[string]any

func (p Params) Int(key string, def int) int {
	v, ok := p[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return def
}

func (p Params) Bool(key string, def bool) bool {
	if v, ok := p[key].(bool); ok {
		return v
	}
	return def
}

// DecodeChain applies filters in order. names and parms must be the same
// length; parms may contain nil entries.
func DecodeChain(ctx context.Context, r io.Reader, names []string, parms []Params, t *budget.Tracker, encodedLen int64) (io.Reader, error) {
	cur := r
	for i, name := range names {
		var p Params
		if i < len(parms) {
			p = parms[i]
		}
		dec, err := decodeOne(ctx, cur, name, p, t)
		if err != nil {
			return nil, err
		}
		enc := int64(-1)
		if i == 0 {
			enc = encodedLen
		}
		cur = newLimitedReader(ctx, dec, t, enc)
	}
	return cur, nil
}

func decodeOne(ctx context.Context, r io.Reader, name string, p Params, t *budget.Tracker) (io.Reader, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap(errs.ErrCanceled, "FLT-CANCELED", "decode filter", -1, 0, -1)
	}
	switch strings.TrimPrefix(name, "/") {
	case "FlateDecode", "Fl":
		out, err := newFlateReader(r)
		if err != nil {
			return nil, err
		}
		return applyPredictor(out, p)
	case "LZWDecode", "LZW":
		out := newLZWReader(r, p.Int("earlychange", 1) != 0)
		return applyPredictor(out, p)
	case "ASCII85Decode", "A85":
		return ascii85Reader(r)
	case "ASCIIHexDecode", "AHx":
		return newHexReader(r), nil
	case "RunLengthDecode", "RL":
		return newRunLengthReader(r), nil
	case "DCTDecode", "DCT", "CCITTFaxDecode", "CCF", "JBIG2Decode", "JPXDecode":
		return nil, errs.Wrap(errs.ErrUnsupported, "FLT-NO-DECODER", "decode filter "+strings.TrimPrefix(name, "/"), -1, 0, -1)
	default:
		return nil, errs.Wrap(fmt.Errorf("%w: %s", errs.ErrUnsupported, name), "FLT-UNKNOWN-FILTER", "decode filter", -1, 0, -1)
	}
}

func applyPredictor(r io.Reader, p Params) (io.Reader, error) {
	pred := p.Int("predictor", 1)
	if pred <= 1 {
		return r, nil
	}
	colors := p.Int("colors", 1)
	bpc := p.Int("bitspercomponent", 8)
	columns := p.Int("columns", 1)
	if colors <= 0 || bpc <= 0 || columns <= 0 {
		return nil, errs.Wrap(errs.ErrCorrupt, "FLT-PREDICTOR-PARAMS", "validate predictor", -1, 0, -1)
	}
	if colors > 64 || columns > 1<<20 {
		return nil, errs.Wrap(errs.ErrBudget, "BUD-PREDICTOR-PARAMS", "validate predictor", -1, 0, -1)
	}
	return newPredictorReader(r, pred, colors, bpc, columns)
}

type limitedReader struct {
	ctx context.Context
	r   io.Reader
	t   *budget.Tracker
	out int64
	enc int64
	err error
}

func newLimitedReader(ctx context.Context, r io.Reader, t *budget.Tracker, encoded int64) *limitedReader {
	return &limitedReader{ctx: ctx, r: r, t: t, enc: encoded}
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.err != nil {
		return 0, l.err
	}
	if err := l.ctx.Err(); err != nil {
		l.err = errs.Wrap(errs.ErrCanceled, "FLT-CANCELED", "read stream", -1, 0, -1)
		return 0, l.err
	}
	n, err := l.r.Read(p)
	if n > 0 {
		l.out += int64(n)
		if l.t != nil {
			if e := l.t.CheckStreamLen(l.out); e != nil {
				l.err = e
				return 0, e
			}
			if e := l.t.AddDecoded(int64(n)); e != nil {
				l.err = e
				return 0, e
			}
		}
	}
	if err == io.EOF && l.t != nil && l.enc > 0 {
		if e := l.t.CheckInflateRatio(l.enc, l.out); e != nil {
			l.err = e
			return n, e
		}
	}
	return n, err
}
