package testkit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Golden compares got with the file at path. Set UPDATE_GOLDEN=1 to rewrite
// goldens. Paths are relative to the calling package's working directory.
func Golden(t testing.TB, path string, got []byte) {
	t.Helper()
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("golden mkdir: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("golden write: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden read %s: %v (run with UPDATE_GOLDEN=1)", path, err)
	}
	if bytes.Equal(want, got) {
		return
	}
	t.Fatalf("golden mismatch %s:\n%s", path, lineDiff(want, got))
}

// GoldenJSON compares JSON with floats rounded to six decimal places. It keeps
// integer fields (including signatures) exact while avoiding architecture-
// specific low-bit differences in computed geometry.
func GoldenJSON(t testing.TB, path string, got []byte) {
	t.Helper()
	normalized, err := normalizeJSON(got)
	if err != nil {
		t.Fatalf("golden json normalize %s: %v", path, err)
	}
	Golden(t, path, normalized)
}

func normalizeJSON(data []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	v = normalizeJSONValue(v)
	return json.Marshal(v)
}

func normalizeJSONValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			x[k] = normalizeJSONValue(value)
		}
		return x
	case []any:
		for i, value := range x {
			x[i] = normalizeJSONValue(value)
		}
		return x
	case json.Number:
		s := x.String()
		if !strings.ContainsAny(s, ".eE") {
			if u, err := strconv.ParseUint(s, 10, 64); err == nil {
				return u
			}
			if i, err := strconv.ParseInt(s, 10, 64); err == nil {
				return i
			}
		}
		f, err := x.Float64()
		if err != nil {
			return s
		}
		return math.Round(f*1e6) / 1e6
	default:
		return v
	}
}

func lineDiff(want, got []byte) string {
	wl := strings.Split(string(want), "\n")
	gl := strings.Split(string(got), "\n")
	var b strings.Builder
	n := len(wl)
	if len(gl) > n {
		n = len(gl)
	}
	shown := 0
	for i := 0; i < n && shown < 20; i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			fmt.Fprintf(&b, "line %d:\n  - %q\n  + %q\n", i+1, w, g)
			shown++
		}
	}
	if shown == 20 {
		b.WriteString("... diff truncated\n")
	}
	return b.String()
}
