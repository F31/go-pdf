package doc

import (
	"context"
	"errors"
	"testing"

	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/diag"
	"github.com/F31/go-pdf/internal/errs"
	"github.com/F31/go-pdf/internal/syntax"
	"github.com/F31/go-pdf/internal/testkit"
)

func openDoc(t *testing.T, data []byte, repair bool) (*Document, *syntax.File) {
	t.Helper()
	b := budget.DefaultBudget
	tr := budget.NewTracker(&b)
	dc := diag.New(1000)
	f, err := syntax.Open(context.Background(), data, syntax.Options{Repair: repair, Tracker: tr, Diag: dc})
	if err != nil {
		t.Fatalf("syntax open: %v", err)
	}
	d, err := Open(context.Background(), f, Options{Repair: repair, Tracker: tr, Diag: dc})
	if err != nil {
		_ = f.Close()
		t.Fatalf("doc open: %v", err)
	}
	return d, f
}

func TestInheritedGeometryRotateUserUnit(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 600 800] /Rotate 90 /UserUnit 2 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R >>"),
	}
	d, f := openDoc(t, testkit.Build(objs, "/Root 1 0 R"), false)
	defer d.Close()
	if d.NumPages() != 1 {
		t.Fatalf("pages %d", d.NumPages())
	}
	p, err := d.Page(0)
	if err != nil {
		t.Fatal(err)
	}
	w, h := p.Size()
	if w != 1600 || h != 1200 {
		t.Fatalf("size = %v x %v", w, h)
	}
	_ = f
}

func TestCropBoxOverrideAndPageSpace(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 600 800] >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /CropBox [10 20 290 380] /Rotate 0 >>"),
	}
	d, _ := openDoc(t, testkit.Build(objs, "/Root 1 0 R"), false)
	defer d.Close()
	p, _ := d.Page(0)
	w, h := p.Size()
	if w != 280 || h != 360 {
		t.Fatalf("size = %v x %v", w, h)
	}
	info := p.Info()
	// PDF (10,20) should map to page (0,360).
	x := info.Matrix[0]*10 + info.Matrix[2]*20 + info.Matrix[4]
	y := info.Matrix[1]*10 + info.Matrix[3]*20 + info.Matrix[5]
	if x != 0 || y != 360 {
		t.Fatalf("mapped bottom-left = %v,%v", x, y)
	}
}

func TestMultiplePagesAndCountHint(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 10 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"),
		testkit.Raw(4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>"),
	}
	d, _ := openDoc(t, testkit.Build(objs, "/Root 1 0 R"), false)
	defer d.Close()
	if d.NumPages() != 2 {
		t.Fatalf("pages = %d", d.NumPages())
	}
	if _, err := d.Page(2); !errors.Is(err, errs.ErrCorrupt) {
		t.Fatalf("range error = %v", err)
	}
}

func TestMetadata(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /Metadata 5 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"),
		testkit.Raw(4, "<< /Title <FEFF00480069> /Producer (TestGen) >>"),
		testkit.Stream(5, "", []byte("<x:xmpmeta/>")),
	}
	d, _ := openDoc(t, testkit.Build(objs, "/Root 1 0 R /Info 4 0 R"), false)
	defer d.Close()
	info := d.Info()
	if info.Title != "Hi" {
		t.Fatalf("title %q", info.Title)
	}
	if info.Producer != "TestGen" {
		t.Fatalf("producer %q", info.Producer)
	}
	if string(info.XMP) != "<x:xmpmeta/>" {
		t.Fatalf("xmp %q", info.XMP)
	}
}

func TestEncryptedDetection(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"),
		testkit.Raw(4, "<< /Filter /Standard /V 2 /R 3 >>"),
	}
	data := testkit.Build(objs, "/Root 1 0 R /Encrypt 4 0 R")
	b := budget.DefaultBudget
	tr := budget.NewTracker(&b)
	f, err := syntax.Open(context.Background(), data, syntax.Options{Tracker: tr, Diag: diag.New(100)})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := Open(context.Background(), f, Options{Tracker: tr}); !errors.Is(err, errs.ErrPassword) {
		t.Fatalf("got %v", err)
	}

	objs[3] = testkit.Raw(4, "<< /Filter /Adobe.PubSec >>")
	f2, err := syntax.Open(context.Background(), testkit.Build(objs, "/Root 1 0 R /Encrypt 4 0 R"), syntax.Options{Tracker: tr, Diag: diag.New(100)})
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
	if _, err := Open(context.Background(), f2, Options{Tracker: tr}); !errors.Is(err, errs.ErrUnsupported) {
		t.Fatalf("got %v", err)
	}
}

func TestClosedDocument(t *testing.T) {
	d, _ := openDoc(t, testkit.Basic("0 0 100 100"), false)
	_ = d.Close()
	if _, err := d.Page(0); !errors.Is(err, errs.ErrClosed) {
		t.Fatalf("got %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}
