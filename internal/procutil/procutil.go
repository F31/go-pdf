// Package procutil contains bounded helpers for optional external processes.
package procutil

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

const DefaultOutputLimit = 64 << 10

var ErrOutputLimit = errors.New("process output limit exceeded")

// Buffer keeps at most Limit bytes and reports whether it truncated output.
type Buffer struct {
	Limit     int
	buf       bytes.Buffer
	truncated bool
}

func (b *Buffer) Write(p []byte) (int, error) {
	if b.Limit <= 0 {
		return len(p), nil
	}
	remaining := b.Limit - b.buf.Len()
	if remaining > 0 {
		n := len(p)
		if n > remaining {
			n = remaining
		}
		_, _ = b.buf.Write(p[:n])
	}
	if len(p) > remaining {
		b.truncated = true
	}
	return len(p), nil
}

func (b *Buffer) String() string {
	s := b.buf.String()
	if b.truncated {
		s += "\n... (truncated)"
	}
	return s
}

// Run executes a command with bounded combined stdout/stderr. The returned
// context error takes precedence over the process exit error.
func Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out Buffer
	out.Limit = DefaultOutputLimit
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if ctx.Err() != nil {
		return out.String(), ctx.Err()
	}
	return out.String(), err
}

// Truncate bounds a string for an error message.
func Truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "... (truncated)"
}

// CheckFileSize rejects a produced file above max bytes.
func CheckFileSize(path string, max int64) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if max > 0 && st.Size() > max {
		return ErrOutputLimit
	}
	return nil
}

// ReadFile reads a produced file with a size cap.
func ReadFile(path string, max int64) ([]byte, error) {
	if err := CheckFileSize(path, max); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// FirstLine returns the first non-empty line, trimmed.
func FirstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}
