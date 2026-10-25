package pdf

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestMapReaderAtCanceled(t *testing.T) {
	data := make([]byte, spoolThreshold+1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := mapReaderAt(ctx, bytes.NewReader(data), int64(len(data)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestReadAllContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := readAllContext(ctx, bytes.NewReader(make([]byte, 16)), 16)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
