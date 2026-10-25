package pdf

import (
	"context"
	"errors"
	"io"
	"os"
)

// spoolThreshold keeps small in-memory ReaderAt inputs cheap. Larger inputs are
// copied in bounded chunks to a temporary file and mapped, so the Go heap does
// not grow by the whole PDF size.
const spoolThreshold = 8 << 20

func mapReaderAt(ctx context.Context, r io.ReaderAt, size int64) ([]byte, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if size <= spoolThreshold {
		data, err := readAllContext(ctx, r, size)
		if err != nil {
			return nil, nil, err
		}
		return data, func() {}, nil
	}
	f, err := os.CreateTemp("", "go-pdf-*")
	if err != nil {
		return nil, nil, err
	}
	name := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(name)
	}
	if err := f.Truncate(size); err != nil {
		cleanup()
		return nil, nil, err
	}
	buf := make([]byte, 1<<20)
	for off := int64(0); off < size; {
		if err := ctx.Err(); err != nil {
			cleanup()
			return nil, nil, err
		}
		n := int64(len(buf))
		if remain := size - off; remain < n {
			n = remain
		}
		read, rerr := r.ReadAt(buf[:n], off)
		if read > 0 {
			if _, werr := f.WriteAt(buf[:read], off); werr != nil {
				cleanup()
				return nil, nil, werr
			}
			off += int64(read)
		}
		if read == 0 {
			cleanup()
			if rerr == nil {
				return nil, nil, io.ErrNoProgress
			}
			if errors.Is(rerr, io.EOF) {
				return nil, nil, io.ErrUnexpectedEOF
			}
			return nil, nil, rerr
		}
		if rerr != nil && !errors.Is(rerr, io.EOF) {
			cleanup()
			return nil, nil, rerr
		}
	}
	data, unmap, err := mapReadOnly(f, size)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	_ = f.Close()
	return data, func() {
		unmap()
		_ = os.Remove(name)
	}, nil
}
