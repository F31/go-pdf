package content

import (
	"context"
	"testing"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/diag"
)

func benchPage(content string) PageData {
	b := budget.DefaultBudget
	return PageData{
		Content:  []byte(content),
		Matrix:   [6]float64{1, 0, 0, -1, 0, 100},
		CropBox:  displaylist.Rect{X0: 0, Y0: 0, X1: 200, Y1: 100},
		MediaBox: displaylist.Rect{X0: 0, Y0: 0, X1: 200, Y1: 100},
		Tracker:  budget.NewTracker(&b), Diag: diag.New(100),
	}
}

func BenchmarkInterpretText(b *testing.B) {
	pd := benchPage("BT /F1 12 Tf 10 50 Td (Hello world) Tj 0 -14 Td (Second line) Tj ET")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Interpret(context.Background(), pd, func(displaylist.Item) error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInterpretPaths(b *testing.B) {
	pd := benchPage("0 0 100 100 re 1 0 0 rg f 10 10 m 20 20 l S")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Interpret(context.Background(), pd, func(displaylist.Item) error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}
