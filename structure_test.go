package pdf

import (
	"context"
	"reflect"
	"testing"

	"github.com/F31/go-pdf/internal/testkit"
)

func numberValue(v float64) StructValue { return StructValue{Kind: "number", Number: v} }

func TestStructureTreeCycleTerminates(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 5 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"),
		testkit.Raw(5, "<< /Type /StructTreeRoot /K [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /StructElem /S /Document /K [6 0 R] >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	tree, err := d.StructureTree(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 1 || tree[0].Type != "Document" || len(tree[0].Children) != 0 {
		t.Fatalf("cycle tree = %#v", tree)
	}
}

func TestAsTable(t *testing.T) {
	table := StructElement{
		Type: "Table",
		Children: []StructElement{
			{Type: "TR", Children: []StructElement{
				{Type: "TH", IDName: "h1", Attributes: []StructAttribute{{
					Owner: "Table",
					Values: map[string]StructValue{
						"Scope": {Kind: "name", Text: "Column"},
						"BBox":  {Kind: "array", Items: []StructValue{numberValue(0), numberValue(0), numberValue(10), numberValue(20)}},
					},
				}}},
				{Type: "TD", Attributes: []StructAttribute{{
					Owner: "Table", Values: map[string]StructValue{"ColSpan": numberValue(2), "RowSpan": numberValue(2)},
				}}},
			}},
			{Type: "TR", Children: []StructElement{
				{Type: "TD", Attributes: []StructAttribute{{
					Owner: "Table", Values: map[string]StructValue{
						"Headers": {Kind: "array", Items: []StructValue{{Kind: "string", Text: "h1"}}},
					},
				}}},
			}},
		},
	}
	tbl, ok := table.AsTable()
	if !ok || len(tbl.Rows) != 2 || len(tbl.Rows[0].Cells) != 2 {
		t.Fatalf("table = %#v", tbl)
	}
	if tbl.Columns != 3 {
		t.Fatalf("columns = %d", tbl.Columns)
	}
	th := tbl.Rows[0].Cells[0]
	if !th.Header || !th.HasBBox || th.BBox.X1 != 10 || len(th.Headers) != 0 {
		t.Fatalf("header = %#v", th)
	}
	td := tbl.Rows[0].Cells[1]
	if td.Header || td.ColSpan != 2 || td.RowSpan != 2 || td.HasBBox || td.Row != 0 || td.Col != 1 {
		t.Fatalf("cell = %#v", td)
	}
	data := tbl.Rows[1].Cells[0]
	if data.Row != 1 || data.Col != 0 || len(data.HeaderRefs) != 1 || data.HeaderRefs[0] != (TableCellRef{Row: 0, Cell: 0}) {
		t.Fatalf("data cell = %#v", data)
	}
	if scope, ok := th.Element.Scope(); !ok || scope != "Column" {
		t.Fatalf("scope = %q", scope)
	}
	if _, ok := (&StructElement{Type: "P"}).AsTable(); ok {
		t.Fatal("non-table accepted")
	}
}

func TestPageTables(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 5 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 9 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/TD << /MCID 0 >> BDC BT /F1 12 Tf 10 50 Td (Cell A) Tj ET EMC")),
		testkit.Raw(5, "<< /Type /StructTreeRoot /K [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /StructElem /S /Table /K [7 0 R] >>"),
		testkit.Raw(7, "<< /Type /StructElem /S /TR /K [8 0 R] >>"),
		testkit.Raw(8, "<< /Type /StructElem /S /TD /Pg 3 0 R /K 0 >>"),
		testkit.Raw(9, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	tables, err := p.Tables(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || len(tables[0].Rows) != 1 || tables[0].Rows[0].Cells[0].Text != "Cell A" {
		t.Fatalf("tables = %#v", tables)
	}
	all, err := d.Tables(context.Background())
	if err != nil || len(all) != 1 || all[0].Page != 0 {
		t.Fatalf("document tables = %#v err=%v", all, err)
	}
	again, err := d.Tables(context.Background())
	if err != nil || !reflect.DeepEqual(all, again) {
		t.Fatalf("tables are not deterministic: %#v / %#v err=%v", all, again, err)
	}
}

func TestNestedTable(t *testing.T) {
	nested := StructElement{
		Type: "Table",
		Children: []StructElement{{
			Type: "TR", Children: []StructElement{{Type: "TD"}},
		}},
	}
	outer := StructElement{
		Type: "Table",
		Children: []StructElement{{
			Type: "TR", Children: []StructElement{{Type: "TD", Children: []StructElement{nested}}},
		}},
	}
	tbl, ok := outer.AsTable()
	if !ok || len(tbl.Rows) != 1 || len(tbl.Rows[0].Cells) != 1 {
		t.Fatalf("outer = %#v", tbl)
	}
	if got := tbl.Rows[0].Cells[0].Nested; len(got) != 1 || len(got[0].Rows) != 1 {
		t.Fatalf("nested = %#v", got)
	}
}

func TestTableInvalidSpanDiagnostic(t *testing.T) {
	table := StructElement{Type: "Table", Children: []StructElement{{Type: "TR", Children: []StructElement{{
		Type: "TD", Attributes: []StructAttribute{{Owner: "Table", Values: map[string]StructValue{"RowSpan": numberValue(0)}}},
	}}}}}
	tbl, ok := table.AsTable()
	if !ok || len(tbl.Diagnostics) == 0 || tbl.Diagnostics[0].Code != "TBL-SPAN-INVALID" {
		t.Fatalf("diagnostics = %#v", tbl.Diagnostics)
	}
}

func TestTableHeadersIndirectAndParagraphs(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 5 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 13 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/TH << /MCID 0 >> BDC BT /F1 12 Tf 10 70 Td (Header) Tj ET EMC /TD << /MCID 1 >> BDC BT /F1 12 Tf 10 50 Td (Para1) Tj ET EMC /TD << /MCID 2 >> BDC BT /F1 12 Tf 10 30 Td (Para2) Tj ET EMC")),
		testkit.Raw(5, "<< /Type /StructTreeRoot /K [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /StructElem /S /Table /K [7 0 R] >>"),
		testkit.Raw(7, "<< /Type /StructElem /S /TR /Pg 3 0 R /K [8 0 R 9 0 R] >>"),
		testkit.Raw(8, "<< /Type /StructElem /S /TH /Pg 3 0 R /ID (h1) /K 0 >>"),
		testkit.Raw(9, "<< /Type /StructElem /S /TD /Pg 3 0 R /A << /O /Table /Headers [12 0 R] >> /K [10 0 R 11 0 R] >>"),
		testkit.Raw(10, "<< /Type /StructElem /S /Span /Pg 3 0 R /K 1 >>"),
		testkit.Raw(11, "<< /Type /StructElem /S /Span /Pg 3 0 R /K 2 >>"),
		testkit.Raw(12, "(h1)"),
		testkit.Raw(13, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	p, _ := d.Page(0)
	tables, err := p.Tables(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || len(tables[0].Rows) != 1 || len(tables[0].Rows[0].Cells) != 2 {
		t.Fatalf("tables = %#v", tables)
	}
	cell := tables[0].Rows[0].Cells[1]
	if len(cell.Headers) != 1 || cell.Headers[0] != "h1" || len(cell.HeaderRefs) != 1 {
		t.Fatalf("headers = %#v refs = %#v", cell.Headers, cell.HeaderRefs)
	}
	if cell.HeaderRefs[0] != (TableCellRef{Row: 0, Cell: 0}) {
		t.Fatalf("header ref = %#v", cell.HeaderRefs[0])
	}
	if len(cell.Paragraphs) != 2 || cell.Text != "Para1\nPara2" {
		t.Fatalf("paragraphs = %#v text = %q", cell.Paragraphs, cell.Text)
	}
	if tables[0].Page != 0 {
		t.Fatalf("page = %d", tables[0].Page)
	}
}

func TestTableRowsAcrossPages(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 7 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 13 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/TD << /MCID 0 >> BDC BT /F1 12 Tf 10 50 Td (Cell A) Tj ET EMC")),
		testkit.Raw(5, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 13 0 R >> >> /Contents 6 0 R >>"),
		testkit.Stream(6, "", []byte("/TD << /MCID 1 >> BDC BT /F1 12 Tf 10 50 Td (Cell B) Tj ET EMC")),
		testkit.Raw(7, "<< /Type /StructTreeRoot /K [8 0 R] >>"),
		testkit.Raw(8, "<< /Type /StructElem /S /Table /K [9 0 R 11 0 R] >>"),
		testkit.Raw(9, "<< /Type /StructElem /S /TR /Pg 3 0 R /K [10 0 R] >>"),
		testkit.Raw(10, "<< /Type /StructElem /S /TD /Pg 3 0 R /K 0 >>"),
		testkit.Raw(11, "<< /Type /StructElem /S /TR /Pg 5 0 R /K [12 0 R] >>"),
		testkit.Raw(12, "<< /Type /StructElem /S /TD /Pg 5 0 R /K 1 >>"),
		testkit.Raw(13, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	for page, want := range []string{"Cell A", "Cell B"} {
		p, _ := d.Page(page)
		tables, err := p.Tables(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(tables) != 1 || len(tables[0].Rows) != 1 || tables[0].Rows[0].Cells[0].Text != want {
			t.Fatalf("page %d tables = %#v", page, tables)
		}
	}
	all, err := d.Tables(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || len(all[0].Rows) != 2 || all[0].Page != 0 || all[0].EndPage != 1 {
		t.Fatalf("merged = %#v", all)
	}
	if all[0].Rows[0].Cells[0].Text != "Cell A" || all[0].Rows[1].Cells[0].Text != "Cell B" {
		t.Fatalf("merged rows = %#v", all[0].Rows)
	}
}

func TestRepeatedHeaderAcrossPages(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 7 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 17 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/TH << /MCID 0 >> BDC BT /F1 12 Tf 10 70 Td (Header) Tj ET EMC /TD << /MCID 1 >> BDC BT /F1 12 Tf 10 50 Td (A) Tj ET EMC")),
		testkit.Raw(5, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 17 0 R >> >> /Contents 6 0 R >>"),
		testkit.Stream(6, "", []byte("/TH << /MCID 2 >> BDC BT /F1 12 Tf 10 70 Td (Header) Tj ET EMC /TD << /MCID 3 >> BDC BT /F1 12 Tf 10 50 Td (B) Tj ET EMC")),
		testkit.Raw(7, "<< /Type /StructTreeRoot /K [8 0 R] >>"),
		testkit.Raw(8, "<< /Type /StructElem /S /Table /K [9 0 R 10 0 R 11 0 R 12 0 R] >>"),
		testkit.Raw(9, "<< /Type /StructElem /S /TR /Pg 3 0 R /K [13 0 R] >>"),
		testkit.Raw(10, "<< /Type /StructElem /S /TR /Pg 3 0 R /K [14 0 R] >>"),
		testkit.Raw(11, "<< /Type /StructElem /S /TR /Pg 5 0 R /K [15 0 R] >>"),
		testkit.Raw(12, "<< /Type /StructElem /S /TR /Pg 5 0 R /K [16 0 R] >>"),
		testkit.Raw(13, "<< /Type /StructElem /S /TH /Pg 3 0 R /ID (h1) /K 0 >>"),
		testkit.Raw(14, "<< /Type /StructElem /S /TD /Pg 3 0 R /K 1 >>"),
		testkit.Raw(15, "<< /Type /StructElem /S /TH /Pg 5 0 R /ID (h1) /K 2 >>"),
		testkit.Raw(16, "<< /Type /StructElem /S /TD /Pg 5 0 R /K 3 >>"),
		testkit.Raw(17, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	all, err := d.Tables(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].RepeatedHeaders != 1 || len(all[0].Rows) != 3 {
		t.Fatalf("merged = %#v", all)
	}
	if len(all[0].HeaderRows) != 1 || all[0].HeaderRows[0] != 0 {
		t.Fatalf("header rows = %#v", all[0].HeaderRows)
	}
	if all[0].Rows[1].Cells[0].Text != "A" || all[0].Rows[2].Cells[0].Text != "B" {
		t.Fatalf("rows = %#v", all[0].Rows)
	}
}

func TestTablesHeuristicMerge(t *testing.T) {
	objs := []testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 7 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 18 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/TH << /MCID 0 >> BDC BT /F1 12 Tf 10 70 Td (Header) Tj ET EMC /TD << /MCID 1 >> BDC BT /F1 12 Tf 10 50 Td (A) Tj ET EMC")),
		testkit.Raw(5, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 18 0 R >> >> /Contents 6 0 R >>"),
		testkit.Stream(6, "", []byte("/TH << /MCID 2 >> BDC BT /F1 12 Tf 10 70 Td (Header) Tj ET EMC /TD << /MCID 3 >> BDC BT /F1 12 Tf 10 50 Td (B) Tj ET EMC")),
		testkit.Raw(7, "<< /Type /StructTreeRoot /K [8 0 R 11 0 R] >>"),
		testkit.Raw(8, "<< /Type /StructElem /S /Table /K [9 0 R 10 0 R] >>"),
		testkit.Raw(9, "<< /Type /StructElem /S /TR /Pg 3 0 R /K [14 0 R] >>"),
		testkit.Raw(10, "<< /Type /StructElem /S /TR /Pg 3 0 R /K [15 0 R] >>"),
		testkit.Raw(11, "<< /Type /StructElem /S /Table /K [12 0 R 13 0 R] >>"),
		testkit.Raw(12, "<< /Type /StructElem /S /TR /Pg 5 0 R /K [16 0 R] >>"),
		testkit.Raw(13, "<< /Type /StructElem /S /TR /Pg 5 0 R /K [17 0 R] >>"),
		testkit.Raw(14, "<< /Type /StructElem /S /TH /Pg 3 0 R /ID (h1) /K 0 >>"),
		testkit.Raw(15, "<< /Type /StructElem /S /TD /Pg 3 0 R /K 1 >>"),
		testkit.Raw(16, "<< /Type /StructElem /S /TH /Pg 5 0 R /ID (h1) /K 2 >>"),
		testkit.Raw(17, "<< /Type /StructElem /S /TD /Pg 5 0 R /K 3 >>"),
		testkit.Raw(18, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	}
	d := openTest(t, testkit.Build(objs, "/Root 1 0 R"))
	defer d.Close()
	tables, err := d.Tables(context.Background())
	if err != nil || len(tables) != 2 {
		t.Fatalf("tables = %#v err=%v", tables, err)
	}
	merged, err := d.TablesMerged(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 1 || !merged[0].Heuristic || merged[0].RepeatedHeaders != 1 || len(merged[0].Rows) != 3 {
		t.Fatalf("merged = %#v", merged)
	}
	if merged[0].Rows[1].Cells[0].Text != "A" || merged[0].Rows[2].Cells[0].Text != "B" {
		t.Fatalf("rows = %#v", merged[0].Rows)
	}
}
