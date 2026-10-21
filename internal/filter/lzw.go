package filter

import (
	"bufio"
	"io"

	"github.com/F31/go-pdf/internal/errs"
)

// lzwReader implements the PDF variant of LZW (MSB-first, 8-bit literals,
// EarlyChange configurable). The standard library decoder implements the
// non-early variant, so PDF streams need a dedicated decoder.
type lzwReader struct {
	r       *bufio.Reader
	early   bool
	bits    uint32
	nbits   uint
	dict    [][]byte
	prev    []byte
	next    int
	width   uint
	pending []byte
	err     error
	done    bool
}

func newLZWReader(r io.Reader, early bool) io.Reader {
	l := &lzwReader{r: bufio.NewReader(r), early: early}
	l.reset()
	return l
}

func (l *lzwReader) reset() {
	l.dict = make([][]byte, 258, 4096)
	for i := 0; i < 256; i++ {
		l.dict[i] = []byte{byte(i)}
	}
	l.next = 258
	l.width = 9
	l.prev = nil
}

func (l *lzwReader) readCode() (int, error) {
	if l.nbits < l.width {
		for l.nbits < l.width {
			b, err := l.r.ReadByte()
			if err != nil {
				return 0, err
			}
			l.bits = l.bits<<8 | uint32(b)
			l.nbits += 8
		}
	}
	shift := l.nbits - l.width
	code := int((l.bits >> shift) & (1<<l.width - 1))
	l.nbits -= l.width
	if l.nbits == 0 {
		l.bits = 0
	} else {
		l.bits &= 1<<l.nbits - 1
	}
	return code, nil
}

func (l *lzwReader) decode() error {
	for len(l.pending) == 0 {
		code, err := l.readCode()
		if err != nil {
			if err == io.EOF {
				l.done = true
				return io.EOF
			}
			return errs.Wrap(errs.ErrCorrupt, "FLT-LZW-READ", "decode LZW", -1, 0, -1)
		}
		switch code {
		case 256:
			l.reset()
			continue
		case 257:
			l.done = true
			return io.EOF
		}
		var seq []byte
		switch {
		case code < l.next && l.dict[code] != nil:
			seq = l.dict[code]
		case code == l.next && l.prev != nil:
			seq = append(append([]byte(nil), l.prev...), l.prev[0])
		default:
			return errs.Wrap(errs.ErrCorrupt, "FLT-LZW-CODE", "decode LZW", -1, 0, -1)
		}
		if l.prev != nil && l.next < 4096 {
			entry := make([]byte, len(l.prev)+1)
			copy(entry, l.prev)
			entry[len(l.prev)] = seq[0]
			l.dict = append(l.dict, entry)
			l.next++
			threshold := 1 << l.width
			if l.early {
				threshold--
			}
			// The decoder lags the encoder by one dictionary entry, so the
			// width must switch one entry before the encoder's threshold.
			if l.next == threshold-1 && l.width < 12 {
				l.width++
			}
		}
		l.pending = seq
		l.prev = append([]byte(nil), seq...)
	}
	return nil
}

func (l *lzwReader) Read(p []byte) (int, error) {
	if l.err != nil {
		return 0, l.err
	}
	for len(l.pending) == 0 {
		if l.done {
			return 0, io.EOF
		}
		if err := l.decode(); err != nil {
			if err == io.EOF {
				return 0, io.EOF
			}
			l.err = err
			return 0, err
		}
	}
	n := copy(p, l.pending)
	l.pending = l.pending[n:]
	return n, nil
}
