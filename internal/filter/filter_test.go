package filter

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/errs"
)

func decode(t *testing.T, name string, data []byte, p Params) []byte {
	t.Helper()
	r, err := decodeOne(context.Background(), bytes.NewReader(data), name, p, budget.NewTracker(&budget.DefaultBudget))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("%s read: %v", name, err)
	}
	return out
}

func TestFlate(t *testing.T) {
	var b bytes.Buffer
	zw := zlib.NewWriter(&b)
	_, _ = zw.Write([]byte("hello pdf"))
	_ = zw.Close()
	if got := string(decode(t, "FlateDecode", b.Bytes(), nil)); got != "hello pdf" {
		t.Fatalf("got %q", got)
	}
}

func TestASCIIHex(t *testing.T) {
	if got := string(decode(t, "ASCIIHexDecode", []byte("61 62\n63>"), nil)); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := string(decode(t, "ASCIIHexDecode", []byte("616>"), nil)); got != "a`" {
		t.Fatalf("odd nibble got %q", got)
	}
}

func TestASCII85(t *testing.T) {
	// "hello" in ASCII85 with EOD marker.
	if got := string(decode(t, "ASCII85Decode", []byte("BOu!rDZ~>"), nil)); got != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestRunLength(t *testing.T) {
	// 3 literal bytes "abc", then 4 repetitions of 'z' (257-253=4), EOD.
	data := []byte{2, 'a', 'b', 'c', 253, 'z', 128}
	if got := string(decode(t, "RunLengthDecode", data, nil)); got != "abczzzz" {
		t.Fatalf("got %q", got)
	}
}

func TestPNGPredictor(t *testing.T) {
	// Two rows of 3 bytes: row 1 uses Sub, row 2 uses None.
	data := []byte{1, 10, 10, 10, 0, 5, 6, 7}
	r, err := newPredictorReader(bytes.NewReader(data), 12, 1, 8, 3)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{10, 20, 30, 5, 6, 7}) {
		t.Fatalf("predictor got %v", got)
	}
}

func TestTIFFPredictor(t *testing.T) {
	// RGB row: first pixel 1,2,3; subsequent samples are deltas.
	data := []byte{1, 2, 3, 1, 1, 1}
	r, err := newPredictorReader(bytes.NewReader(data), 2, 3, 8, 2)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	if !bytes.Equal(got, []byte{1, 2, 3, 2, 3, 4}) {
		t.Fatalf("got %v", got)
	}
}

func TestBudgetLimits(t *testing.T) {
	b := budget.DefaultBudget
	b.MaxStreamBytes = 4
	tr := budget.NewTracker(&b)
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, _ = zw.Write(bytes.Repeat([]byte("a"), 100))
	_ = zw.Close()
	r, err := DecodeChain(context.Background(), bytes.NewReader(buf.Bytes()), []string{"FlateDecode"}, nil, tr, int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(r)
	if !errors.Is(err, errs.ErrBudget) {
		t.Fatalf("expected ErrBudget, got %v", err)
	}
}

func TestUnsupportedFilters(t *testing.T) {
	for _, name := range []string{"DCTDecode", "CCITTFaxDecode", "JBIG2Decode", "JPXDecode", "FooDecode"} {
		_, err := decodeOne(context.Background(), bytes.NewReader(nil), name, nil, nil)
		if !errors.Is(err, errs.ErrUnsupported) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func FuzzDecodeFilters(f *testing.F) {
	f.Add([]byte("BOu!rDZ~>"))
	f.Add([]byte{2, 'a', 'b', 'c', 128})
	f.Add([]byte("6162>"))
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, name := range []string{"ASCII85Decode", "ASCIIHexDecode", "RunLengthDecode"} {
			r, err := decodeOne(context.Background(), bytes.NewReader(data), name, nil, nil)
			if err == nil {
				_, _ = io.Copy(io.Discard, io.LimitReader(r, 1<<20))
			}
		}
	})
}

func TestLZW(t *testing.T) {
	src := []byte("TOBEORNOTTOBEORTOBEORNOT")
	encoded := lzwEncode(src, true)
	r, err := decodeOne(context.Background(), bytes.NewReader(encoded), "LZWDecode", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, src) {
		t.Fatalf("got %q, want %q", got, src)
	}
}

// lzwEncode mirrors the PDF LZW variant for round-trip testing.
func lzwEncode(src []byte, early bool) []byte {
	dict := map[string]int{}
	for i := 0; i < 256; i++ {
		dict[string([]byte{byte(i)})] = i
	}
	next, width := 258, uint(9)
	var out []byte
	var bits uint64
	var nbits uint
	emit := func(code int) {
		bits = bits<<width | uint64(code)
		nbits += width
		for nbits >= 8 {
			out = append(out, byte(bits>>(nbits-8)))
			nbits -= 8
		}
		if nbits == 0 {
			bits = 0
		} else {
			bits &= 1<<nbits - 1
		}
	}
	emit(256)
	var w []byte
	for _, b := range src {
		wc := append(append([]byte(nil), w...), b)
		if _, ok := dict[string(wc)]; ok {
			w = wc
			continue
		}
		emit(dict[string(w)])
		if next < 4096 {
			dict[string(wc)] = next
			next++
			threshold := 1 << width
			if early {
				threshold--
			}
			if next == threshold && width < 12 {
				width++
			}
		}
		w = []byte{b}
	}
	if len(w) > 0 {
		emit(dict[string(w)])
	}
	emit(257)
	if nbits > 0 {
		out = append(out, byte(bits<<(8-nbits)))
	}
	return out
}

func FuzzPredictor(f *testing.F) {
	f.Add([]byte{1, 10, 10, 10, 0, 5, 6, 7})
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := newPredictorReader(bytes.NewReader(data), 12, 1, 8, 3)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(r, 1<<20))
		}
		r, err = newPredictorReader(bytes.NewReader(data), 2, 3, 8, 2)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(r, 1<<20))
		}
	})
}

func BenchmarkFlateDecode(b *testing.B) {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, _ = zw.Write(bytes.Repeat([]byte("pdf benchmark "), 1024))
	_ = zw.Close()
	data := buf.Bytes()
	tr := budget.NewTracker(&budget.DefaultBudget)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		r, err := decodeOne(context.Background(), bytes.NewReader(data), "FlateDecode", nil, tr)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, r); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkASCIIHexDecode(b *testing.B) {
	data := []byte("6162636465666768696a6b6c6d6e6f70>")
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		r, _ := decodeOne(context.Background(), bytes.NewReader(data), "ASCIIHexDecode", nil, nil)
		_, _ = io.Copy(io.Discard, r)
	}
}
