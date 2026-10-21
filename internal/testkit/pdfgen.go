// Package testkit contains helpers for building small PDFs and golden files in
// tests. It is not part of the public API.
package testkit

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"sort"
	"strings"
)

type Obj struct {
	Num  int
	Body string
}

func Raw(num int, body string) Obj { return Obj{Num: num, Body: body} }

func Stream(num int, dict string, data []byte) Obj {
	dict = strings.TrimSpace(dict)
	if !strings.HasPrefix(dict, "<<") {
		dict = "<< " + dict + " >>"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<< /Length %d", len(data))
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(dict, "<<"), ">>"))
	if inner != "" {
		b.WriteString(" ")
		b.WriteString(inner)
	}
	b.WriteString(" >>\nstream\n")
	b.Write(data)
	b.WriteString("\nendstream")
	return Obj{Num: num, Body: b.String()}
}

// Build creates a PDF 1.7 file with a classic xref table. Objects must have
// unique, positive numbers and are written in ascending order.
func Build(objs []Obj, trailerExtra string) []byte {
	sorted := append([]Obj(nil), objs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Num < sorted[j].Num })
	maxNum := 0
	for _, o := range sorted {
		if o.Num > maxNum {
			maxNum = o.Num
		}
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	offsets := make(map[int]int, len(sorted))
	for _, o := range sorted {
		offsets[o.Num] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", o.Num, o.Body)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n", maxNum+1)
	b.WriteString("0000000000 65535 f \n")
	for i := 1; i <= maxNum; i++ {
		if off, ok := offsets[i]; ok {
			fmt.Fprintf(&b, "%010d 00000 n \n", off)
		} else {
			b.WriteString("0000000000 65535 f \n")
		}
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d", maxNum+1)
	if trailerExtra != "" {
		b.WriteString(" ")
		b.WriteString(strings.TrimSpace(trailerExtra))
	}
	fmt.Fprintf(&b, " >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return b.Bytes()
}

// Basic returns a one-page document. The page's parent Pages node has the
// given media box.
func Basic(mediaBox string) []byte {
	objs := []Obj{
		Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox ["+mediaBox+"] >>"),
	}
	return Build(objs, "/Root 1 0 R")
}

// ObjectStream builds an object stream containing the given objects. The
// returned stream has the supplied object number.
func ObjectStream(num int, contained map[int]string) Obj {
	nums := make([]int, 0, len(contained))
	for n := range contained {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	var header, body strings.Builder
	for _, n := range nums {
		off := body.Len()
		fmt.Fprintf(&header, "%d %d ", n, off)
		body.WriteString(contained[n])
		body.WriteString(" ")
	}
	first := header.Len()
	data := []byte(header.String() + body.String())
	return Stream(num, fmt.Sprintf("/Type /ObjStm /N %d /First %d", len(nums), first), data)
}

// IncrementalUpdate appends an updated object and a new xref section linked by
// Prev. base must have been produced by Build.
func IncrementalUpdate(base []byte, obj Obj) []byte {
	prev := findStartXref(base)
	var b bytes.Buffer
	b.Write(base)
	if len(base) == 0 || base[len(base)-1] != '\n' {
		b.WriteByte('\n')
	}
	off := b.Len()
	fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", obj.Num, obj.Body)
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n%d 1\n%010d 00000 n \ntrailer\n<< /Size 10 /Root 1 0 R /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", obj.Num, off, prev, xref)
	return b.Bytes()
}

func findStartXref(data []byte) int {
	i := bytes.LastIndex(data, []byte("startxref"))
	if i < 0 {
		return 0
	}
	j := i + len("startxref")
	for j < len(data) && (data[j] == '\r' || data[j] == '\n' || data[j] == ' ' || data[j] == '\t') {
		j++
	}
	n := 0
	for j < len(data) && data[j] >= '0' && data[j] <= '9' {
		n = n*10 + int(data[j]-'0')
		j++
	}
	return n
}

// CorruptStartXref rewrites the final startxref offset to an invalid value.
func CorruptStartXref(data []byte) []byte {
	i := bytes.LastIndex(data, []byte("startxref"))
	if i < 0 {
		return data
	}
	out := append([]byte(nil), data...)
	j := i + len("startxref")
	for j < len(out) && (out[j] == '\r' || out[j] == '\n' || out[j] == ' ') {
		j++
	}
	for j < len(out) && out[j] >= '0' && out[j] <= '9' {
		out[j] = '9'
		j++
	}
	return out
}

// BuildXRefStream creates a PDF whose only xref representation is a Flate
// compressed /Type /XRef stream. Objects listed in compressed are stored in an
// existing object stream: num -> {objstm number, index}.
func BuildXRefStream(objs []Obj, compressed map[int][2]int, trailerExtra string) []byte {
	sorted := append([]Obj(nil), objs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Num < sorted[j].Num })
	maxNum := 0
	for _, o := range sorted {
		if o.Num > maxNum {
			maxNum = o.Num
		}
	}
	for n := range compressed {
		if n > maxNum {
			maxNum = n
		}
	}
	xrefNum := maxNum + 1
	size := xrefNum + 1

	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	offsets := make(map[int]int, len(sorted))
	for _, o := range sorted {
		offsets[o.Num] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", o.Num, o.Body)
	}
	xrefOffset := b.Len()
	w := [3]int{1, 4, 2}
	rowLen := w[0] + w[1] + w[2]
	raw := make([]byte, size*rowLen)
	put := func(num int, typ byte, f1, f2 int64) {
		base := num * rowLen
		raw[base] = typ
		raw[base+1] = byte(f1 >> 24)
		raw[base+2] = byte(f1 >> 16)
		raw[base+3] = byte(f1 >> 8)
		raw[base+4] = byte(f1)
		raw[base+5] = byte(f2 >> 8)
		raw[base+6] = byte(f2)
	}
	put(0, 0, 0, 65535)
	for _, o := range sorted {
		put(o.Num, 1, int64(offsets[o.Num]), 0)
	}
	for num, loc := range compressed {
		put(num, 2, int64(loc[0]), int64(loc[1]))
	}
	put(xrefNum, 1, int64(xrefOffset), 0)

	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	_, _ = zw.Write(raw)
	_ = zw.Close()
	fmt.Fprintf(&b, "%d 0 obj\n<< /Type /XRef /Size %d /W [%d %d %d] /Filter /FlateDecode /Length %d", xrefNum, size, w[0], w[1], w[2], zbuf.Len())
	if trailerExtra != "" {
		b.WriteString(" ")
		b.WriteString(strings.TrimSpace(trailerExtra))
	}
	b.WriteString(" >>\nstream\n")
	b.Write(zbuf.Bytes())
	b.WriteString("\nendstream\nendobj\n")
	fmt.Fprintf(&b, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return b.Bytes()
}
