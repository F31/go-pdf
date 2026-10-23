package doc

import (
	"context"
	"sort"
	"strconv"

	"github.com/F31/go-pdf/internal/syntax"
)

type labelRange struct {
	start  int
	style  string
	prefix string
	first  int
}

func (d *Document) PageLabel(ctx context.Context, index int) string {
	if index < 0 || index >= len(d.pages) {
		return ""
	}
	ranges := d.pageLabelRanges(ctx)
	selected := labelRange{start: 0, style: "D", first: 1}
	for _, r := range ranges {
		if r.start <= index {
			selected = r
		}
	}
	if selected.style == "" {
		return selected.prefix
	}
	n := selected.first + index - selected.start
	if n < 1 {
		n = 1
	}
	return selected.prefix + formatLabel(n, selected.style)
}

func (d *Document) pageLabelRanges(ctx context.Context) []labelRange {
	obj, err := d.file.Resolve(ctx, d.catalog.Get("PageLabels"))
	if err != nil {
		return nil
	}
	dict, ok := obj.(syntax.Dict)
	if !ok {
		return nil
	}
	nums, ok := dict.Get("Nums").(syntax.Array)
	if !ok {
		return nil
	}
	var out []labelRange
	for i := 0; i+1 < len(nums); i += 2 {
		start, ok := syntax.AsNumber(nums[i])
		if !ok {
			continue
		}
		item, err := d.file.Resolve(ctx, nums[i+1])
		if err != nil {
			continue
		}
		def, ok := item.(syntax.Dict)
		if !ok {
			continue
		}
		r := labelRange{start: int(start), first: 1}
		if v, ok := def.NameValue("S"); ok {
			r.style = v
		}
		if v, ok := def.Get("P").(syntax.String); ok {
			r.prefix = decodeBytes(v)
		}
		if v, ok := def.IntValue("St"); ok {
			r.first = int(v)
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].start < out[j].start })
	return out
}

func formatLabel(n int, style string) string {
	switch style {
	case "R":
		return roman(n, true)
	case "r":
		return roman(n, false)
	case "A":
		return letters(n, true)
	case "a":
		return letters(n, false)
	default:
		return strconv.Itoa(n)
	}
}

func roman(n int, upper bool) string {
	if n <= 0 {
		return ""
	}
	vals := []struct {
		v int
		s string
	}{{1000, "m"}, {900, "cm"}, {500, "d"}, {400, "cd"}, {100, "c"}, {90, "xc"}, {50, "l"}, {40, "xl"}, {10, "x"}, {9, "ix"}, {5, "v"}, {4, "iv"}, {1, "i"}}
	var out string
	for _, x := range vals {
		for n >= x.v {
			out += x.s
			n -= x.v
		}
	}
	if upper {
		return stringsToUpper(out)
	}
	return out
}

func stringsToUpper(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 'a' - 'A'
		}
	}
	return string(b)
}

func letters(n int, upper bool) string {
	if n <= 0 {
		return ""
	}
	var out []byte
	for n > 0 {
		n--
		out = append([]byte{byte('A' + n%26)}, out...)
		n /= 26
	}
	if !upper {
		for i := range out {
			out[i] += 'a' - 'A'
		}
	}
	return string(out)
}

func decodeBytes(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF {
		return utf16BE(b[2:])
	}
	return string(b)
}
