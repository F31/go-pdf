package diag

import (
	"sync"
	"testing"
)

func TestCollectorConcurrent(t *testing.T) {
	c := New(10000)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				c.Addf(Info, "SYN-TEST", "test", -1, n, -1)
			}
		}(i)
	}
	wg.Wait()
	if c.Len() != 8000 {
		t.Fatalf("len = %d", c.Len())
	}
}

func TestCollectorCapacityKeepsErrors(t *testing.T) {
	c := New(2)
	c.Addf(Info, "SYN-A", "a", -1, 0, -1)
	c.Addf(Info, "SYN-B", "b", -1, 0, -1)
	c.Addf(Error, "SYN-C", "c", -1, 0, -1)
	if c.Len() != 2 {
		t.Fatalf("len = %d", c.Len())
	}
	if c.Dropped() != 1 {
		t.Fatalf("dropped = %d", c.Dropped())
	}
	found := false
	for _, d := range c.All() {
		if d.Level == Error {
			found = true
		}
	}
	if !found {
		t.Fatal("error diagnostic was dropped")
	}
}

func TestValidCode(t *testing.T) {
	for _, code := range []string{"SYN-A", "FLT-B", "BUD-C", "DOC-D", "CRY-E"} {
		if !ValidCode(code) {
			t.Errorf("%s should be valid", code)
		}
	}
	for _, code := range []string{"", "FOO-A", "SYN", "-A"} {
		if ValidCode(code) {
			t.Errorf("%s should be invalid", code)
		}
	}
}
