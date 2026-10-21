package syntax

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/F31/go-pdf/internal/testkit"
)

func TestStructureGolden(t *testing.T) {
	f := openBytes(t, testkit.Basic("0 0 200 100"), false)
	defer f.Close()
	var b strings.Builder
	fmt.Fprintf(&b, "version=%s\n", f.HeaderVersion())
	fmt.Fprintf(&b, "repaired=%v\n", f.Repaired())
	nums := make([]int, 0, len(f.xref))
	for n := range f.xref {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	fmt.Fprintf(&b, "xref=%d\n", len(nums))
	for _, n := range nums {
		e := f.xref[n]
		fmt.Fprintf(&b, "obj=%d type=%d off=%d gen=%d stm=%d index=%d\n", n, e.Type, e.Offset, e.Gen, e.ObjStm, e.Index)
	}
	keys := make([]string, 0, len(f.trailer))
	for k := range f.trailer {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "trailer.%s=%T\n", k, f.trailer[Name(k)])
	}
	root, err := f.ResolveRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	typ, _ := root.NameValue("Type")
	fmt.Fprintf(&b, "root.type=%s\n", typ)
	testkit.Golden(t, "../../testdata/goldens/syntax-basic.txt", []byte(b.String()))
}
