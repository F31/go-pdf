package font

import (
	"context"
	"testing"

	"github.com/F31/go-pdf/internal/diag"
	"github.com/F31/go-pdf/internal/syntax"
)

func TestBase14Widths(t *testing.T) {
	dc := diag.New(100)
	f, err := Load(context.Background(), nil, syntax.Dict{
		"Subtype":  syntax.Name("Type1"),
		"BaseFont": syntax.Name("Helvetica"),
	}, dc)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Width('A'); got != 667 {
		t.Fatalf("A width %v", got)
	}
	if got := f.Width(' '); got != 278 {
		t.Fatalf("space width %v", got)
	}
	if s, ok := f.Decode('H'); !ok || s != "H" {
		t.Fatalf("decode %q %v", s, ok)
	}
}

func TestSubsetNormalization(t *testing.T) {
	f, err := Load(context.Background(), nil, syntax.Dict{
		"Subtype":  syntax.Name("Type1"),
		"BaseFont": syntax.Name("ABCDEF+Helvetica-Bold"),
	}, diag.New(100))
	if err != nil {
		t.Fatal(err)
	}
	if f.Info.BaseFont != "Helvetica-Bold" || !f.Info.Subset || !f.Info.Bold {
		t.Fatalf("info %#v", f.Info)
	}
}

func TestWinAnsiAndDifferences(t *testing.T) {
	f, err := Load(context.Background(), nil, syntax.Dict{
		"Subtype":  syntax.Name("Type1"),
		"BaseFont": syntax.Name("Helvetica"),
		"Encoding": syntax.Name("WinAnsiEncoding"),
	}, diag.New(100))
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := f.Decode(0xE9); !ok || s != "é" {
		t.Fatalf("decode 0xE9 %q %v", s, ok)
	}
	enc := syntax.Dict{
		"BaseEncoding": syntax.Name("WinAnsiEncoding"),
		"Differences":  syntax.Array{syntax.Int(65), syntax.Name("Euro")},
	}
	f2, err := Load(context.Background(), nil, syntax.Dict{
		"Subtype":  syntax.Name("Type1"),
		"BaseFont": syntax.Name("Helvetica"),
		"Encoding": enc,
	}, diag.New(100))
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := f2.Decode(65); !ok || s != "€" {
		t.Fatalf("difference decode %q %v", s, ok)
	}
}

func TestType0Deferred(t *testing.T) {
	dc := diag.New(100)
	f, err := Load(context.Background(), nil, syntax.Dict{
		"Subtype":  syntax.Name("Type0"),
		"BaseFont": syntax.Name("SimSun"),
	}, dc)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Decode('A'); ok {
		t.Fatal("Type0 should not claim trusted Unicode")
	}
	found := false
	for _, d := range dc.All() {
		if d.Code == "FNT-TYPE0-DEFERRED" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing deferred diagnostic")
	}
}

func TestFallbackWidth(t *testing.T) {
	f, err := Load(context.Background(), nil, syntax.Dict{
		"Subtype":  syntax.Name("Type1"),
		"BaseFont": syntax.Name("UnknownFont"),
	}, diag.New(100))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Width('A'); got != 500 {
		t.Fatalf("fallback %v", got)
	}
}
