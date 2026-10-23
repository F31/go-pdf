package budget

import (
	"errors"
	"testing"

	"github.com/F31/go-pdf/internal/errs"
)

func TestFileSize(t *testing.T) {
	b := DefaultBudget
	b.MaxFileSize = 10
	tr := NewTracker(&b)
	if err := tr.CheckFileSize(10); err != nil {
		t.Fatal(err)
	}
	if err := tr.CheckFileSize(11); !errors.Is(err, errs.ErrBudget) {
		t.Fatalf("got %v", err)
	}
	if err := tr.CheckFileSize(-1); !errors.Is(err, errs.ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
}

func TestObjectsAndDecoded(t *testing.T) {
	b := DefaultBudget
	b.MaxObjects = 2
	b.MaxTotalDecoded = 3
	tr := NewTracker(&b)
	if err := tr.AddObjects(2); err != nil {
		t.Fatal(err)
	}
	if err := tr.AddObjects(1); !errors.Is(err, errs.ErrBudget) {
		t.Fatalf("got %v", err)
	}
	if err := tr.AddDecoded(3); err != nil {
		t.Fatal(err)
	}
	if err := tr.AddDecoded(1); !errors.Is(err, errs.ErrBudget) {
		t.Fatalf("got %v", err)
	}
}

func TestStreamAndRatio(t *testing.T) {
	b := DefaultBudget
	b.MaxStreamBytes = 10
	b.MaxInflateRatio = 2
	tr := NewTracker(&b)
	if err := tr.CheckStreamLen(11); !errors.Is(err, errs.ErrBudget) {
		t.Fatalf("got %v", err)
	}
	if err := tr.CheckInflateRatio(10, 21); !errors.Is(err, errs.ErrBudget) {
		t.Fatalf("got %v", err)
	}
	if err := tr.CheckInflateRatio(0, 100); err != nil {
		t.Fatalf("zero encoded: %v", err)
	}
}

func TestNesting(t *testing.T) {
	b := DefaultBudget
	b.MaxNestingDepth = 2
	tr := NewTracker(&b)
	if err := tr.CheckNesting(2); err != nil {
		t.Fatal(err)
	}
	if err := tr.CheckNesting(3); !errors.Is(err, errs.ErrBudget) {
		t.Fatalf("got %v", err)
	}
}

func TestConcurrentCounters(t *testing.T) {
	tr := NewTracker(&DefaultBudget)
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			for j := 0; j < 1000; j++ {
				_ = tr.AddObjects(1)
				_ = tr.AddDecoded(1)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	if tr.Objects() != 8000 || tr.Decoded() != 8000 {
		t.Fatalf("objects=%d decoded=%d", tr.Objects(), tr.Decoded())
	}
}

func TestLiveMemory(t *testing.T) {
	b := DefaultBudget
	b.MaxLiveBytes = 10
	tr := NewTracker(&b)
	r1, err := tr.Acquire(4)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := tr.Acquire(6)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Live() != 10 || tr.Peak() != 10 {
		t.Fatalf("live=%d peak=%d", tr.Live(), tr.Peak())
	}
	if _, err := tr.Acquire(1); !errors.Is(err, errs.ErrBudget) {
		t.Fatalf("got %v", err)
	}
	r2()
	r2()
	if tr.Live() != 4 {
		t.Fatalf("live=%d", tr.Live())
	}
	r1()
	if tr.Live() != 0 {
		t.Fatalf("live=%d", tr.Live())
	}
}
