package filter

import (
	"bufio"
	"encoding/ascii85"
	"io"

	"github.com/F31/go-pdf/internal/errs"
)

func ascii85Reader(r io.Reader) (io.Reader, error) {
	return ascii85.NewDecoder(stripASCII85End(r)), nil
}

// stripASCII85End removes the optional ~> EOD marker because the stdlib decoder
// treats every non-ASCII85 byte as an error rather than an end marker.
func stripASCII85End(r io.Reader) io.Reader {
	return &ascii85Filter{r: bufio.NewReader(r)}
}

type ascii85Filter struct {
	r    *bufio.Reader
	done bool
}

func (f *ascii85Filter) Read(p []byte) (int, error) {
	if f.done {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) {
		b, err := f.r.ReadByte()
		if err != nil {
			f.done = true
			if n > 0 {
				return n, nil
			}
			return 0, err
		}
		if b == '~' {
			if next, err := f.r.ReadByte(); err == nil && next == '>' {
				f.done = true
				if n > 0 {
					return n, nil
				}
				return 0, io.EOF
			}
			f.done = true
			return n, errs.Wrap(errs.ErrCorrupt, "FLT-ASCII85-EOD", "decode ASCII85", -1, 0, -1)
		}
		p[n] = b
		n++
	}
	return n, nil
}

func newHexReader(r io.Reader) io.Reader {
	return &hexReader{r: bufio.NewReader(r)}
}

type hexReader struct {
	r    *bufio.Reader
	high int
	have bool
	done bool
}

func (h *hexReader) Read(p []byte) (int, error) {
	if h.done {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) {
		c, err := h.r.ReadByte()
		if err != nil {
			h.done = true
			if n > 0 {
				return n, nil
			}
			return 0, err
		}
		if c == '>' {
			h.done = true
			if h.have {
				p[n] = byte(h.high << 4)
				n++
			}
			if n == 0 {
				return 0, io.EOF
			}
			return n, nil
		}
		v, ok := hexVal(c)
		if !ok {
			if isSpace(c) {
				continue
			}
			return n, errs.Wrap(errs.ErrCorrupt, "FLT-ASCIIHEX", "decode ASCIIHex", -1, 0, -1)
		}
		if !h.have {
			h.high = v
			h.have = true
			continue
		}
		p[n] = byte(h.high<<4 | v)
		n++
		h.have = false
	}
	return n, nil
}

func hexVal(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	}
	return 0, false
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == 0
}
