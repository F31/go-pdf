package filter

import (
	"io"

	"github.com/F31/go-pdf/internal/errs"
)

type predictorReader struct {
	r       io.Reader
	pred    int
	colors  int
	bpc     int
	columns int
	rowLen  int
	bpp     int
	prev    []byte
	cur     []byte
	out     []byte
	pos     int
	eof     bool
	err     error
}

func newPredictorReader(r io.Reader, pred, colors, bpc, columns int) (io.Reader, error) {
	rowBits := colors * bpc * columns
	if rowBits <= 0 || rowBits/8 > 256<<20 {
		return nil, errs.Wrap(errs.ErrBudget, "BUD-PREDICTOR-ROW", "validate predictor", -1, 0, -1)
	}
	rowLen := (rowBits + 7) / 8
	bpp := (colors*bpc + 7) / 8
	if bpp < 1 {
		bpp = 1
	}
	switch pred {
	case 2:
		if bpc != 8 && bpc != 16 {
			return nil, errs.Wrap(errs.ErrUnsupported, "FLT-PREDICTOR-BPC", "validate predictor", -1, 0, -1)
		}
	case 10, 11, 12, 13, 14, 15:
	default:
		return nil, errs.Wrap(errs.ErrUnsupported, "FLT-PREDICTOR-TYPE", "validate predictor", -1, 0, -1)
	}
	return &predictorReader{
		r: r, pred: pred, colors: colors, bpc: bpc, columns: columns,
		rowLen: rowLen, bpp: bpp,
		prev: make([]byte, rowLen),
	}, nil
}

func (p *predictorReader) Read(dst []byte) (int, error) {
	if p.err != nil {
		return 0, p.err
	}
	n := 0
	for n < len(dst) {
		if p.pos >= len(p.out) {
			if p.eof {
				if n == 0 {
					return 0, io.EOF
				}
				return n, nil
			}
			if err := p.nextRow(); err != nil {
				if err == io.EOF {
					p.eof = true
					if n == 0 {
						return 0, io.EOF
					}
					return n, nil
				}
				p.err = err
				return n, err
			}
		}
		c := copy(dst[n:], p.out[p.pos:])
		n += c
		p.pos += c
	}
	return n, nil
}

func (p *predictorReader) nextRow() error {
	row := make([]byte, p.rowLen)
	var filter byte
	if p.pred >= 10 {
		var hdr [1]byte
		if _, err := io.ReadFull(p.r, hdr[:]); err != nil {
			if err == io.ErrUnexpectedEOF {
				return errs.Wrap(errs.ErrCorrupt, "FLT-PREDICTOR-TRUNCATED", "read predictor row", -1, 0, -1)
			}
			return err
		}
		filter = hdr[0]
	}
	if _, err := io.ReadFull(p.r, row); err != nil {
		if err == io.ErrUnexpectedEOF {
			return errs.Wrap(errs.ErrCorrupt, "FLT-PREDICTOR-TRUNCATED", "read predictor row", -1, 0, -1)
		}
		if err == io.EOF && p.pred < 10 && len(row) == 0 {
			return io.EOF
		}
		return err
	}
	switch {
	case p.pred == 2:
		if p.bpc == 8 {
			for i := p.colors; i < len(row); i++ {
				row[i] += row[i-p.colors]
			}
		} else {
			samples := p.columns * p.colors
			for i := p.colors; i < samples; i++ {
				cur := int(row[2*i])<<8 | int(row[2*i+1])
				prev := int(row[2*(i-p.colors)])<<8 | int(row[2*(i-p.colors)+1])
				v := (cur + prev) & 0xffff
				row[2*i] = byte(v >> 8)
				row[2*i+1] = byte(v)
			}
		}
	case p.pred >= 10:
		if err := p.pngFilter(filter, row); err != nil {
			return err
		}
	}
	if p.pred >= 10 {
		copy(p.prev, row)
	}
	p.cur, p.out, p.pos = row, row, 0
	return nil
}

func (p *predictorReader) pngFilter(filter byte, row []byte) error {
	bpp := p.bpp
	switch filter {
	case 0:
	case 1:
		for i := bpp; i < len(row); i++ {
			row[i] += row[i-bpp]
		}
	case 2:
		for i := range row {
			row[i] += p.prev[i]
		}
	case 3:
		for i := range row {
			left := 0
			if i >= bpp {
				left = int(row[i-bpp])
			}
			row[i] += byte((left + int(p.prev[i])) / 2)
		}
	case 4:
		for i := range row {
			left, ul := 0, 0
			if i >= bpp {
				left = int(row[i-bpp])
				ul = int(p.prev[i-bpp])
			}
			row[i] += byte(paeth(left, int(p.prev[i]), ul))
		}
	default:
		return errs.Wrap(errs.ErrCorrupt, "FLT-PREDICTOR-FILTER", "decode predictor row", -1, 0, -1)
	}
	return nil
}

func paeth(a, b, c int) int {
	p := a + b - c
	pa, pb, pc := abs(p-a), abs(p-b), abs(p-c)
	if pa <= pb && pa <= pc {
		return a
	}
	if pb <= pc {
		return b
	}
	return c
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
