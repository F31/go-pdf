package filter

import (
	"compress/zlib"
	"io"

	"github.com/F31/go-pdf/internal/errs"
)

func newFlateReader(r io.Reader) (io.Reader, error) {
	zr, err := zlib.NewReader(r)
	if err != nil {
		return nil, errs.Wrap(errs.ErrCorrupt, "FLT-FLATE-HEADER", "open flate stream", -1, 0, -1)
	}
	return zr, nil
}
