package g32d

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/F31/go-pdf/extension"
)

var eol = bitString{0x001, 12}

type testBits struct{ out []bool }

func (b *testBits) write(bs bitString) {
	for i := int(bs.nBits) - 1; i >= 0; i-- {
		b.out = append(b.out, (bs.bits>>uint(i))&1 != 0)
	}
}

func (b *testBits) writeTag(v bool) {
	if v {
		b.out = append(b.out, true)
	} else {
		b.out = append(b.out, false)
	}
}

func (b *testBits) pad() {
	for len(b.out)%8 != 0 {
		b.out = append(b.out, false)
	}
}

func (b *testBits) bytes() []byte {
	out := make([]byte, (len(b.out)+7)/8)
	for i, bit := range b.out {
		if bit {
			out[i/8] |= 1 << uint(7-i%8)
		}
	}
	return out
}

func encodeRun(b *testBits, n int, white bool) {
	t2, t3 := whiteEncodeTable2[:], whiteEncodeTable3[:]
	if !white {
		t2, t3 = blackEncodeTable2[:], blackEncodeTable3[:]
	}
	for n >= 64 {
		b.write(t3[n/64])
		n %= 64
	}
	b.write(t2[n])
}

func rowRuns(row []bool) []int {
	runs := []int{}
	white := true
	n := 0
	for _, px := range row {
		pixelWhite := px
		if pixelWhite != white {
			runs = append(runs, n)
			n = 0
			white = !white
		}
		n++
	}
	runs = append(runs, n)
	return runs
}

func encodeRows(width int, rows [][]bool, modes []bool) []byte {
	return encodeRowsAligned(width, rows, modes, false)
}

func encodeRowsAligned(width int, rows [][]bool, modes []bool, align bool) []byte {
	var b testBits
	for i, row := range rows {
		twoD := false
		if len(modes) > 0 {
			twoD = modes[i]
		}
		b.write(eol)
		b.writeTag(!twoD)
		if align {
			b.pad()
		}
		runs := rowRuns(row)
		if twoD {
			if len(runs)%2 == 1 {
				runs = append(runs, 0)
			}
			for j := 0; j < len(runs); j += 2 {
				b.write(modeEncodeTable[modeH])
				encodeRun(&b, runs[j], true)
				encodeRun(&b, runs[j+1], false)
			}
		} else {
			for j, n := range runs {
				encodeRun(&b, n, j%2 == 0)
			}
		}
	}
	// trailing RTC is optional for known PDF heights
	for i := 0; i < 6; i++ {
		b.write(eol)
	}
	return b.bytes()
}

func bitsOf(rows [][]bool) []byte {
	width := len(rows[0])
	out := make([]byte, (width+7)/8*len(rows))
	for y, row := range rows {
		for x, v := range row {
			if v {
				out[y*((width+7)/8)+x/8] |= 1 << uint(7-x%8)
			}
		}
	}
	return out
}

func TestDecodeGroup32D(t *testing.T) {
	rows := [][]bool{
		{true, true, true, true, true, true, true, true},
		{false, true, false, true, false, true, false, true},
		{true, false, false, true, true, false, false, true},
	}
	data := encodeRows(8, rows, []bool{true, true, true})
	r := NewReader(bytes.NewReader(data), MSB, Group3_2D, 8, len(rows), nil)
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bitsOf(rows)) {
		t.Fatalf("got %08b want %08b", got, bitsOf(rows))
	}
}

func TestDecodeMixed1D2D(t *testing.T) {
	rows := [][]bool{
		{true, true, false, false, true, true, true, true},
		{false, false, true, true, false, false, true, true},
		{true, false, true, false, true, false, true, false},
	}
	data := encodeRows(8, rows, []bool{false, true, false})
	r := NewReader(bytes.NewReader(data), MSB, Group3_2D, 8, len(rows), nil)
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bitsOf(rows)) {
		t.Fatalf("got %08b want %08b", got, bitsOf(rows))
	}
}

func TestProviderDecodeAndMask(t *testing.T) {
	rows := [][]bool{{true, false, true, false}, {false, true, false, true}}
	data := encodeRows(4, rows, []bool{true, true})
	f := New()
	cfg := extension.ImageConfig{
		Encoding: extension.ImageCCITTG32D, Width: 4, Height: 2,
		BitsPerComponent: 1, ColorSpace: "DeviceGray",
		CCITT: &extension.CCITTParams{K: 1},
	}
	dec, err := f.NewDecoder(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	plane, err := dec.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if plane.Width != 4 || plane.Height != 2 || plane.Components != 1 || plane.Pix[0] != 255 || plane.Pix[1] != 0 {
		t.Fatalf("plane = %#v", plane)
	}

	cfg.ImageMask = true
	cfg.ColorSpace = "ImageMask"
	dec, err = f.NewDecoder(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	mask, err := dec.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !mask.Alpha || mask.Components != 2 || mask.Pix[1] != 0 || mask.Pix[3] != 255 {
		t.Fatalf("mask = %#v", mask)
	}
}

func TestProviderRequiresKAndBudget(t *testing.T) {
	f := New()
	cfg := extension.ImageConfig{Width: 4, Height: 2, CCITT: &extension.CCITTParams{K: 0}}
	dec, err := f.NewDecoder(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dec.Decode(context.Background(), bytes.NewReader([]byte{1}), 1, cfg); err == nil {
		t.Fatal("expected K error")
	}
	cfg.CCITT = &extension.CCITTParams{K: 1, Columns: maxDimension + 1}
	dec, _ = f.NewDecoder(context.Background(), cfg)
	if _, err := dec.Decode(context.Background(), bytes.NewReader([]byte{1}), 1, cfg); err == nil {
		t.Fatal("expected dimension error")
	}
}

func TestProviderMetadata(t *testing.T) {
	f := New()
	info := f.Info()
	if info.Name == "" || info.License == "" || info.Determinism != extension.Deterministic {
		t.Fatalf("info = %#v", info)
	}
	if got := f.Encodings(); len(got) != 1 || got[0] != extension.ImageCCITTG32D {
		t.Fatalf("encodings = %v", got)
	}
}

func FuzzCCITTG32D(f *testing.F) {
	rows := [][]bool{{true, false, true, false}, {false, true, false, true}}
	f.Add(encodeRows(4, rows, []bool{true, true}))
	f.Add([]byte{0, 1, 2, 3})
	f.Fuzz(func(t *testing.T, data []byte) {
		cfg := extension.ImageConfig{
			Width: 8, Height: 8, CCITT: &extension.CCITTParams{K: 1},
		}
		factory := New()
		dec, err := factory.NewDecoder(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = dec.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), cfg)
	})
}

func TestProviderBlackIsZeroAndTruncation(t *testing.T) {
	rows := [][]bool{{true, false}, {false, true}}
	data := encodeRows(2, rows, []bool{true, true})
	f := New()
	cfg := extension.ImageConfig{
		Width: 2, Height: 2, CCITT: &extension.CCITTParams{K: 1, BlackIsZero: true},
	}
	dec, err := f.NewDecoder(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	plane, err := dec.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if plane.Pix[0] != 0 || plane.Pix[1] != 255 {
		t.Fatalf("black-is-zero pixels = %v", plane.Pix[:2])
	}

	cfg.CCITT.BlackIsZero = false
	cfg.Height = 3
	dec, _ = f.NewDecoder(context.Background(), cfg)
	if _, err := dec.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), cfg); err == nil {
		t.Fatal("expected truncated stream error")
	}
}

func TestProviderContextAndCleanup(t *testing.T) {
	rows := [][]bool{{true, false}, {false, true}}
	data := encodeRows(2, rows, []bool{true, true})
	cfg := extension.ImageConfig{Width: 2, Height: 2, CCITT: &extension.CCITTParams{K: 1}}
	f := New()
	dec, err := f.NewDecoder(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := dec.Decode(ctx, bytes.NewReader(data), int64(len(data)), cfg); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if err := dec.Close(); err != nil || dec.Close() != nil {
		t.Fatal("Close is not idempotent")
	}
}

func TestRegistryIntegration(t *testing.T) {
	r, err := extension.NewRegistry(extension.WithImageFactory(New()))
	if err != nil {
		t.Fatal(err)
	}
	f, ok := r.ImageFactory(extension.ImageCCITTG32D)
	if !ok || f == nil {
		t.Fatal("g32d provider not registered")
	}
	info := f.Info()
	if info.Name != "g32d" || info.License != "Apache-2.0" || info.Determinism != extension.Deterministic {
		t.Fatalf("info = %#v", info)
	}
	if got := r.Encodings(); len(got) != 1 || got[0] != extension.ImageCCITTG32D {
		t.Fatalf("encodings = %v", got)
	}
}

func TestDecodeEncodedByteAlign(t *testing.T) {
	rows := [][]bool{
		{true, false, false, true},
		{false, true, true, false},
		{true, true, false, false},
	}
	data := encodeRowsAligned(4, rows, []bool{true, true, true}, true)
	r := NewReader(bytes.NewReader(data), MSB, Group3_2D, 4, len(rows), &Options{Align: true})
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bitsOf(rows)) {
		t.Fatalf("aligned got %08b want %08b", got, bitsOf(rows))
	}
}

func TestProviderColumnsRowsOverride(t *testing.T) {
	rows := [][]bool{{true, false}, {false, true}}
	data := encodeRows(2, rows, []bool{true, true})
	f := New()
	cfg := extension.ImageConfig{Width: 1, Height: 1, CCITT: &extension.CCITTParams{K: 1, Columns: 2, Rows: 2}}
	dec, _ := f.NewDecoder(context.Background(), cfg)
	plane, err := dec.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if plane.Width != 2 || plane.Height != 2 {
		t.Fatalf("dimensions = %dx%d", plane.Width, plane.Height)
	}
}

func TestDecodeVerticalAndPassModes(t *testing.T) {
	// First row: black in columns 0..2, white elsewhere (1-D).
	// Second row: white 0..3 then black 3..8, encoded as Pass + Horizontal.
	var b testBits
	first := []bool{false, false, false, true, true, true, true, true}
	b.write(eol)
	b.writeTag(true)
	runs := rowRuns(first)
	for i, n := range runs {
		encodeRun(&b, n, i%2 == 0)
	}
	b.write(eol)
	b.writeTag(false)
	b.write(modeEncodeTable[modePass])
	b.write(modeEncodeTable[modeH])
	encodeRun(&b, 0, true)
	encodeRun(&b, 5, false)
	for i := 0; i < 6; i++ {
		b.write(eol)
	}
	rows := [][]bool{
		first,
		{true, true, true, false, false, false, false, false},
	}
	r := NewReader(bytes.NewReader(b.bytes()), MSB, Group3_2D, 8, 2, nil)
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bitsOf(rows)) {
		t.Fatalf("modes got %08b want %08b", got, bitsOf(rows))
	}
}

func TestDecodeVerticalZero(t *testing.T) {
	var b testBits
	for row := 0; row < 2; row++ {
		b.write(eol)
		b.writeTag(false)
		b.write(modeEncodeTable[modeV0])
	}
	for i := 0; i < 2; i++ {
		b.write(eol)
	}
	rows := [][]bool{{true, true, true, true}, {true, true, true, true}}
	r := NewReader(bytes.NewReader(b.bytes()), MSB, Group3_2D, 4, 2, nil)
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bitsOf(rows)) {
		t.Fatalf("vertical got %08b want %08b", got, bitsOf(rows))
	}
}

func BenchmarkG32DDecode(b *testing.B) {
	rows := make([][]bool, 64)
	for i := range rows {
		rows[i] = make([]bool, 256)
		for j := range rows[i] {
			rows[i][j] = (i+j)%7 != 0
		}
	}
	modes := make([]bool, len(rows))
	for i := range modes {
		modes[i] = true
	}
	data := encodeRows(256, rows, modes)
	cfg := extension.ImageConfig{Width: 256, Height: 64, CCITT: &extension.CCITTParams{K: 1}}
	f := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dec, err := f.NewDecoder(context.Background(), cfg)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = dec.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), cfg)
	}
}
