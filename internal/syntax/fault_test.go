package syntax

import (
	"context"
	"errors"
	"testing"

	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/diag"
	"github.com/F31/go-pdf/internal/errs"
	"github.com/F31/go-pdf/internal/testkit"
)

func TestFaultWrongXRefOffset(t *testing.T) {
	good := testkit.Basic("0 0 200 100")
	bad := testkit.SetXRefOffset(good, 3, 5)
	f, err := Open(context.Background(), bad, Options{Tracker: budget.NewTracker(&budget.DefaultBudget), Diag: diag.New(50)})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := f.ResolveRef(context.Background(), Ref{Num: 3}); !errors.Is(err, errs.ErrCorrupt) {
		t.Fatalf("strict resolve got %v", err)
	}
	if err := f.Rebuild(context.Background()); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if !f.Repaired() {
		t.Fatal("expected repair")
	}
	if _, err := f.ResolveRef(context.Background(), Ref{Num: 3}); err != nil {
		t.Fatalf("resolve after rebuild: %v", err)
	}
	_ = f.Close()
}

func TestFaultStreamLength(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("q Q")),
	}
	good := testkit.Build(objs, "/Root 1 0 R")
	bad := testkit.CorruptStreamLength(good, 4, 99)
	f, err := Open(context.Background(), bad, Options{Tracker: budget.NewTracker(&budget.DefaultBudget), Diag: diag.New(50)})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := f.ResolveRef(context.Background(), Ref{Num: 4}); !errors.Is(err, errs.ErrCorrupt) {
		t.Fatalf("strict resolve got %v", err)
	}
	if err := f.Rebuild(context.Background()); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if _, err := f.ResolveRoot(context.Background()); err != nil {
		t.Fatalf("root: %v", err)
	}
	_ = f.Close()
}

func TestFaultTruncatedObject(t *testing.T) {
	good := testkit.Basic("0 0 200 100")
	bad := testkit.TruncateObject(good, 2)
	// Repair scans to the next object or EOF; the catalog should survive.
	f := openBytes(t, bad, true)
	defer f.Close()
	if _, err := f.ResolveRoot(context.Background()); err != nil {
		t.Fatalf("root: %v", err)
	}
}
