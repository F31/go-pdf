package font

import (
	"testing"

	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/syntax"
)

func TestCacheBudgetAndClose(t *testing.T) {
	b := budget.DefaultBudget
	b.MaxLiveBytes = 5000
	tr := budget.NewTracker(&b)
	c := NewCache(tr)
	big := &Font{program: &fontProgram{size: 10_000}}
	if c.Put(syntax.Ref{Num: 1}, big) {
		t.Fatal("oversized font was cached")
	}
	small := &Font{program: &fontProgram{size: 100}}
	if !c.Put(syntax.Ref{Num: 2}, small) {
		t.Fatal("small font was not cached")
	}
	if tr.Live() <= 0 {
		t.Fatalf("live=%d", tr.Live())
	}
	if f, ok := c.Get(syntax.Ref{Num: 2}); !ok || f != small {
		t.Fatal("cache get failed")
	}
	c.Close()
	if tr.Live() != 0 {
		t.Fatalf("live after close=%d", tr.Live())
	}
}
