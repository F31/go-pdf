package pdf

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/F31/go-pdf/internal/testkit"
)

func isolatedTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
	return dir
}

func tempPDFFiles(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []os.DirEntry
	for _, e := range entries {
		if len(e.Name()) >= len("go-pdf-") && e.Name()[:len("go-pdf-")] == "go-pdf-" {
			out = append(out, e)
		}
	}
	return out
}

func TestMapReaderAtTruncated(t *testing.T) {
	dir := isolatedTempDir(t)
	size := int64(spoolThreshold + 1)
	data := make([]byte, int(size)-1)
	type result struct {
		release func()
		err     error
	}
	done := make(chan result, 1)
	go func() {
		_, release, err := mapReaderAt(context.Background(), bytes.NewReader(data), size)
		done <- result{release: release, err: err}
	}()
	select {
	case res := <-done:
		if res.release != nil {
			res.release()
		}
		if !errors.Is(res.err, io.ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want unexpected EOF", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("mapReaderAt hung on truncated ReaderAt")
	}
	if files := tempPDFFiles(t, dir); len(files) != 0 {
		t.Fatalf("temp file leaked: %v", files)
	}
}

type cancelAfterRead struct {
	data   []byte
	cancel context.CancelFunc
}

func (r *cancelAfterRead) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n > 0 {
		r.cancel()
	}
	return n, nil
}

func TestMapReaderAtCancelsDuringCopy(t *testing.T) {
	dir := isolatedTempDir(t)
	size := int64(spoolThreshold + 2<<20)
	ctx, cancel := context.WithCancel(context.Background())
	r := &cancelAfterRead{data: make([]byte, size), cancel: cancel}
	_, release, err := mapReaderAt(ctx, r, size)
	if release != nil {
		release()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want canceled", err)
	}
	if files := tempPDFFiles(t, dir); len(files) != 0 {
		t.Fatalf("temp file leaked: %v", files)
	}
}

func TestMapReaderAtTempCleanup(t *testing.T) {
	dir := isolatedTempDir(t)
	size := int64(spoolThreshold + 1)
	_, release, err := mapReaderAt(context.Background(), bytes.NewReader(make([]byte, size)), size)
	if err != nil {
		t.Fatal(err)
	}
	if files := tempPDFFiles(t, dir); len(files) == 0 {
		t.Fatal("expected spool temp file while mapped")
	}
	release()
	if files := tempPDFFiles(t, dir); len(files) != 0 {
		t.Fatalf("temp file leaked after release: %v", files)
	}
}

func TestOpenLargeReaderAtCleansSpool(t *testing.T) {
	dir := isolatedTempDir(t)
	data := append(testkit.Basic("0 0 10 10"), make([]byte, spoolThreshold+1)...)
	doc, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if files := tempPDFFiles(t, dir); len(files) == 0 {
		t.Fatal("expected spool temp file while open")
	}
	if err := doc.Close(); err != nil {
		t.Fatal(err)
	}
	if files := tempPDFFiles(t, dir); len(files) != 0 {
		t.Fatalf("spool temp file leaked after close: %v", files)
	}
}
