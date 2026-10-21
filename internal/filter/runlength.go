package filter

import (
	"bufio"
	"io"

	"github.com/F31/go-pdf/internal/errs"
)

func newRunLengthReader(r io.Reader) io.Reader {
	return &runLengthReader{r: bufio.NewReader(r)}
}

type runLengthReader struct {
	r       *bufio.Reader
	literal int
	repeat  int
	val     byte
	done    bool
}

func (r *runLengthReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) {
		if r.literal > 0 {
			c, err := r.r.ReadByte()
			if err != nil {
				if n > 0 {
					return n, nil
				}
				return 0, err
			}
			p[n] = c
			n++
			r.literal--
			continue
		}
		if r.repeat > 0 {
			p[n] = r.val
			n++
			r.repeat--
			continue
		}
		c, err := r.r.ReadByte()
		if err != nil {
			if n > 0 {
				return n, nil
			}
			return 0, err
		}
		switch {
		case c == 128:
			r.done = true
			if n == 0 {
				return 0, io.EOF
			}
			return n, nil
		case c < 128:
			r.literal = int(c) + 1
		default:
			v, err := r.r.ReadByte()
			if err != nil {
				return n, errs.Wrap(errs.ErrCorrupt, "FLT-RUNLENGTH", "decode RunLength", -1, 0, -1)
			}
			r.repeat = 257 - int(c)
			r.val = v
		}
	}
	return n, nil
}
