package procutil

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBufferBoundsOutput(t *testing.T) {
	var b Buffer
	b.Limit = 4
	if n, err := b.Write([]byte("abcdefgh")); n != 8 || err != nil {
		t.Fatalf("write = %d, %v", n, err)
	}
	if got := b.String(); got != "abcd\n... (truncated)" {
		t.Fatalf("buffer = %q", got)
	}
	if Truncate("abcdefgh", 4) != "abcd... (truncated)" {
		t.Fatal("truncate")
	}
}

func TestRunCancellationAndLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, "sh", "-c", "echo secret"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "out")
	if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckFileSize(path, 4); !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("size err = %v", err)
	}
	if _, err := ReadFile(path, 4); !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("read err = %v", err)
	}
	if got := FirstLine("\n  hello \nworld"); got != "hello" {
		t.Fatalf("first line = %q", got)
	}
}
