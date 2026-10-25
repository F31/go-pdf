package pdf

import (
	"context"
	"strings"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/doc"
	pdftext "github.com/F31/go-pdf/text"
)

type StructValue struct {
	Kind   string
	Text   string
	Number float64
	Bool   bool
	Items  []StructValue
	Fields map[string]StructValue
}

type StructAttribute struct {
	Owner  string
	Values map[string]StructValue
}

// StructElement is a lightweight tagged-PDF structure tree node.
type StructElement struct {
	ID              int
	Type            string
	RawType         string
	Title           string
	Alt             string
	ActualText      string
	Lang            string
	IDName          string
	Classes         []string
	Attributes      []StructAttribute
	ClassAttributes []StructAttribute
	Page            int
	MCIDs           []int
	Children        []StructElement
}

// StructureTree returns the tagged-PDF structure tree when present.
func (d *Document) StructureTree(ctx context.Context) ([]StructElement, error) {
	if d == nil || d.inner == nil {
		return nil, nil
	}
	d.structureMu.Lock()
	defer d.structureMu.Unlock()
	if d.structureDone {
		return d.structure, d.structureErr
	}
	src, err := d.inner.StructureTree(ctx)
	if err != nil {
		d.structureErr = err
		return nil, err
	}
	d.structure = convertStructure(src)
	d.structureDone = true
	return d.structure, nil
}

func structOrder(tree []StructElement, page int) []int {
	var out []int
	var walk func([]StructElement)
	walk = func(nodes []StructElement) {
		for _, n := range nodes {
			if n.ID != 0 && n.Page == page {
				out = append(out, n.ID)
			}
			walk(n.Children)
		}
	}
	walk(tree)
	return out
}

func structInfo(tree []StructElement, page int) map[int]pdftext.StructInfo {
	out := map[int]pdftext.StructInfo{}
	var walk func([]StructElement)
	walk = func(nodes []StructElement) {
		for _, n := range nodes {
			if n.ID != 0 && n.Page == page {
				out[n.ID] = pdftext.StructInfo{Type: n.Type, Title: n.Title, Alt: n.Alt, Lang: n.Lang}
			}
			walk(n.Children)
		}
	}
	walk(tree)
	return out
}

func convertAttributes(src []doc.StructAttribute) []StructAttribute {
	out := make([]StructAttribute, len(src))
	for i, a := range src {
		out[i] = StructAttribute{Owner: a.Owner, Values: convertStructValues(a.Values)}
	}
	return out
}

func convertStructValues(src map[string]doc.StructValue) map[string]StructValue {
	if src == nil {
		return nil
	}
	out := make(map[string]StructValue, len(src))
	for k, v := range src {
		out[k] = convertStructValue(v)
	}
	return out
}

func convertStructValue(src doc.StructValue) StructValue {
	v := StructValue{Kind: src.Kind, Text: src.Text, Number: src.Number, Bool: src.Bool}
	if len(src.Items) > 0 {
		v.Items = make([]StructValue, len(src.Items))
		for i := range src.Items {
			v.Items[i] = convertStructValue(src.Items[i])
		}
	}
	if len(src.Fields) > 0 {
		v.Fields = convertStructValues(src.Fields)
	}
	return v
}

func convertStructure(src []doc.StructureItem) []StructElement {
	out := make([]StructElement, len(src))
	for i, s := range src {
		out[i] = StructElement{
			ID: s.ID, Type: s.Type, RawType: s.RawType, Title: s.Title, Alt: s.Alt, ActualText: s.ActualText,
			Lang: s.Lang, IDName: s.IDName, Classes: append([]string(nil), s.Classes...),
			Attributes: convertAttributes(s.Attributes), ClassAttributes: convertAttributes(s.ClassAttributes),
			Page: s.Page, MCIDs: append([]int(nil), s.MCIDs...), Children: convertStructure(s.Children),
		}
	}
	return out
}

// Attribute returns the first matching attribute. Direct /A values take
// precedence over /ClassMap values; classes are checked in /C order.
func (e *StructElement) Attribute(owner, name string) (StructValue, bool) {
	for _, a := range e.Attributes {
		if owner != "" && a.Owner != owner {
			continue
		}
		if v, ok := a.Values[name]; ok {
			return v, true
		}
	}
	for _, a := range e.ClassAttributes {
		if owner != "" && a.Owner != owner {
			continue
		}
		if v, ok := a.Values[name]; ok {
			return v, true
		}
	}
	return StructValue{}, false
}

// NumberAttribute returns a numeric attribute by owner and name.
func (e *StructElement) NumberAttribute(owner, name string) (float64, bool) {
	v, ok := e.Attribute(owner, name)
	if !ok || v.Kind != "number" {
		return 0, false
	}
	return v.Number, true
}

// NameAttribute returns a name attribute by owner and name.
func (e *StructElement) NameAttribute(owner, name string) (string, bool) {
	v, ok := e.Attribute(owner, name)
	if !ok || v.Kind != "name" {
		return "", false
	}
	return v.Text, true
}

// BBox returns the Layout or Table BBox attribute.
func (e *StructElement) BBox() (displaylist.Rect, bool) {
	v, ok := e.Attribute("Layout", "BBox")
	if !ok {
		v, ok = e.Attribute("Table", "BBox")
	}
	if !ok || v.Kind != "array" || len(v.Items) < 4 {
		return displaylist.Rect{}, false
	}
	for i := 0; i < 4; i++ {
		if v.Items[i].Kind != "number" {
			return displaylist.Rect{}, false
		}
	}
	return displaylist.Rect{
		X0: v.Items[0].Number, Y0: v.Items[1].Number,
		X1: v.Items[2].Number, Y1: v.Items[3].Number,
	}, true
}

// ColSpan returns a Table ColSpan attribute.
func (e *StructElement) ColSpan() (int, bool) {
	n, ok := e.NumberAttribute("Table", "ColSpan")
	return int(n), ok
}

// RowSpan returns a Table RowSpan attribute.
func (e *StructElement) RowSpan() (int, bool) {
	n, ok := e.NumberAttribute("Table", "RowSpan")
	return int(n), ok
}

// Scope returns a Table Scope attribute.
func (e *StructElement) Scope() (string, bool) {
	return e.NameAttribute("Table", "Scope")
}

// Placement returns a Layout Placement attribute.
func (e *StructElement) Placement() (string, bool) {
	return e.NameAttribute("Layout", "Placement")
}

// ListNumbering returns a List ListNumbering attribute.
func (e *StructElement) ListNumbering() (string, bool) {
	return e.NameAttribute("List", "ListNumbering")
}

// TableDiagnostic reports a malformed table grid.
type TableDiagnostic struct {
	Code    string
	Row     int
	Col     int
	Message string
}

// Table is a lightweight semantic view of a tagged Table element.
type Table struct {
	Element         StructElement
	Rows            []TableRow
	Columns         int
	Page            int
	EndPage         int
	HeaderRows      []int
	RepeatedHeaders int
	Heuristic       bool
	Diagnostics     []TableDiagnostic
}

// TableRow is one tagged TR element.
type TableRow struct {
	Element StructElement
	Page    int
	Cells   []TableCell
}

type TableCellRef struct {
	Row  int
	Cell int
}

// TableCell is one tagged TH or TD element.
type TableCell struct {
	Element    StructElement
	Page       int
	Header     bool
	Row        int
	Col        int
	RowSpan    int
	ColSpan    int
	Headers    []string
	HeaderRefs []TableCellRef
	BBox       displaylist.Rect
	HasBBox    bool
	Text       string
	Paragraphs []string
	Nested     []Table
}

// AttachText fills each cell with text blocks whose structure ID belongs to the
// cell's own subtree. Blocks are joined in reading order.
func (t *Table) AttachText(pg *pdftext.Page) {
	if t == nil || pg == nil {
		return
	}
	for ri := range t.Rows {
		for ci := range t.Rows[ri].Cells {
			cell := &t.Rows[ri].Cells[ci]
			ids := elementIDs(&cell.Element)
			var parts []string
			for _, block := range pg.Blocks {
				if ids[block.StructRef] && block.Text != "" {
					parts = append(parts, block.Text)
					cell.Paragraphs = append(cell.Paragraphs, block.Text)
				}
			}
			cell.Text = strings.Join(parts, "\n")
			for ni := range cell.Nested {
				cell.Nested[ni].AttachText(pg)
			}
		}
	}
}

// Tables returns tagged tables from every page. Tables with the same non-zero
// structure-element ID are merged across pages; other tables remain separate.
func (d *Document) Tables(ctx context.Context) ([]Table, error) {
	if d == nil || d.inner == nil || d.closed.Load() {
		return nil, nil
	}
	var out []Table
	seen := map[int]int{}
	for i := 0; i < d.NumPages(); i++ {
		p, err := d.Page(i)
		if err != nil {
			return nil, err
		}
		tables, err := p.Tables(ctx)
		if err != nil {
			return nil, err
		}
		for _, table := range tables {
			if table.Element.ID != 0 {
				if at, ok := seen[table.Element.ID]; ok {
					dst := &out[at]
					rows := table.Rows
					if len(dst.HeaderRows) > 0 && len(rows) > 0 && sameTableRow(dst.Rows[dst.HeaderRows[0]], rows[0]) {
						dst.RepeatedHeaders++
						rows = rows[1:]
					}
					dst.Rows = append(dst.Rows, rows...)
					dst.EndPage = table.EndPage
					dst.Diagnostics = nil
					dst.Columns = 0
					assignTableGrid(dst)
					resolveTableHeaders(dst)
					continue
				}
				seen[table.Element.ID] = len(out)
			}
			out = append(out, table)
		}
	}
	return out, nil
}

func elementIDs(e *StructElement) map[int]bool {
	ids := map[int]bool{}
	var walk func(*StructElement)
	walk = func(n *StructElement) {
		if n.ID != 0 {
			ids[n.ID] = true
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	if e != nil {
		walk(e)
	}
	return ids
}

// TablesMerged is an explicit heuristic pass over Tables. It merges adjacent
// pages when both tables have the same column count and identical first header
// row. Use Tables when structure IDs are authoritative.
func (d *Document) TablesMerged(ctx context.Context) ([]Table, error) {
	tables, err := d.Tables(ctx)
	if err != nil {
		return nil, err
	}
	var out []Table
	for _, t := range tables {
		if len(out) > 0 {
			prev := &out[len(out)-1]
			if canMergeTables(prev, &t) {
				rows := t.Rows
				if len(rows) > 0 && sameTableRow(prev.Rows[prev.HeaderRows[0]], rows[0]) {
					prev.RepeatedHeaders++
					rows = rows[1:]
				}
				prev.Rows = append(prev.Rows, rows...)
				prev.EndPage = t.EndPage
				prev.Heuristic = true
				prev.Columns = 0
				prev.Diagnostics = nil
				assignTableGrid(prev)
				resolveTableHeaders(prev)
				continue
			}
		}
		out = append(out, t)
	}
	return out, nil
}

func canMergeTables(a, b *Table) bool {
	if a == nil || b == nil || a.Columns != b.Columns || a.EndPage+1 != b.Page {
		return false
	}
	if len(a.HeaderRows) == 0 || len(b.HeaderRows) == 0 {
		return false
	}
	return sameTableRow(a.Rows[a.HeaderRows[0]], b.Rows[b.HeaderRows[0]])
}

// AsTable returns a table view when e is a Table with at least one TR child.
func (e *StructElement) AsTable() (Table, bool) {
	if e == nil || e.Type != "Table" {
		return Table{}, false
	}
	table := Table{Element: *e}
	for i := range e.Children {
		row := &e.Children[i]
		if row.Type != "TR" {
			continue
		}
		tr := TableRow{Element: *row, Page: row.Page}
		for j := range row.Children {
			cell := &row.Children[j]
			if cell.Type != "TD" && cell.Type != "TH" {
				continue
			}
			c := TableCell{Element: *cell, Page: cell.Page, Header: cell.Type == "TH", RowSpan: 1, ColSpan: 1}
			if n, ok := cell.RowSpan(); ok {
				if n > 0 {
					c.RowSpan = n
				} else {
					table.Diagnostics = append(table.Diagnostics, TableDiagnostic{Code: "TBL-SPAN-INVALID", Row: -1, Col: -1, Message: "non-positive RowSpan"})
				}
			}
			if n, ok := cell.ColSpan(); ok {
				if n > 0 {
					c.ColSpan = n
				} else {
					table.Diagnostics = append(table.Diagnostics, TableDiagnostic{Code: "TBL-SPAN-INVALID", Row: -1, Col: -1, Message: "non-positive ColSpan"})
				}
			}
			if v, ok := cell.Attribute("Table", "Headers"); ok && v.Kind == "array" {
				for _, item := range v.Items {
					if item.Kind == "string" {
						c.Headers = append(c.Headers, item.Text)
					}
				}
			}
			if box, ok := cell.BBox(); ok {
				c.BBox, c.HasBBox = box, true
			}
			for k := range cell.Children {
				if child := &cell.Children[k]; child.Type == "Table" {
					if nested, ok := child.AsTable(); ok {
						c.Nested = append(c.Nested, nested)
					}
				}
			}
			tr.Cells = append(tr.Cells, c)
		}
		if tr.Page < 0 && len(tr.Cells) > 0 {
			tr.Page = tr.Cells[0].Page
		}
		if len(tr.Cells) > 0 {
			table.Rows = append(table.Rows, tr)
		}
	}
	if len(table.Rows) == 0 {
		return Table{}, false
	}
	assignTableGrid(&table)
	resolveTableHeaders(&table)
	return table, true
}

// FilterPage keeps rows whose page matches page. It is used when one tagged
// Table element contains rows split across multiple pages.
func (t *Table) FilterPage(page int) {
	if t == nil {
		return
	}
	rows := t.Rows[:0]
	for _, row := range t.Rows {
		rp := row.Page
		if rp < 0 && len(row.Cells) > 0 {
			rp = row.Cells[0].Page
		}
		if rp == page {
			rows = append(rows, row)
		}
	}
	t.Rows = rows
	t.Columns = 0
	t.Diagnostics = nil
	assignTableGrid(t)
	resolveTableHeaders(t)
}

func assignTableGrid(table *Table) {
	occupied := map[[2]int]bool{}
	for ri := range table.Rows {
		col := 0
		for ci := range table.Rows[ri].Cells {
			cell := &table.Rows[ri].Cells[ci]
			for occupied[[2]int{ri, col}] {
				col++
			}
			cell.Row, cell.Col = ri, col
			for r := 0; r < cell.RowSpan; r++ {
				for c := 0; c < cell.ColSpan; c++ {
					key := [2]int{ri + r, col + c}
					if occupied[key] {
						table.Diagnostics = append(table.Diagnostics, TableDiagnostic{Code: "TBL-SPAN-OVERLAP", Row: key[0], Col: key[1], Message: "cell span overlaps another cell"})
					}
					occupied[key] = true
				}
			}
			col += cell.ColSpan
			if end := cell.Col + cell.ColSpan; end > table.Columns {
				table.Columns = end
			}
		}
	}
	table.HeaderRows = table.HeaderRows[:0]
	for ri := range table.Rows {
		all := len(table.Rows[ri].Cells) > 0
		for ci := range table.Rows[ri].Cells {
			if !table.Rows[ri].Cells[ci].Header {
				all = false
				break
			}
		}
		if all {
			table.HeaderRows = append(table.HeaderRows, ri)
		}
	}
}

func sameTableRow(a, b TableRow) bool {
	if len(a.Cells) != len(b.Cells) {
		return false
	}
	for i := range a.Cells {
		if a.Cells[i].Header != b.Cells[i].Header || a.Cells[i].Text != b.Cells[i].Text || a.Cells[i].Element.IDName != b.Cells[i].Element.IDName {
			return false
		}
	}
	return true
}

func resolveTableHeaders(table *Table) {
	byID := map[string]TableCellRef{}
	for ri := range table.Rows {
		for ci := range table.Rows[ri].Cells {
			cell := &table.Rows[ri].Cells[ci]
			if cell.Header && cell.Element.IDName != "" {
				if _, exists := byID[cell.Element.IDName]; !exists {
					byID[cell.Element.IDName] = TableCellRef{Row: ri, Cell: ci}
				}
			}
		}
	}
	for ri := range table.Rows {
		for ci := range table.Rows[ri].Cells {
			cell := &table.Rows[ri].Cells[ci]
			cell.HeaderRefs = nil
			for _, id := range cell.Headers {
				if ref, ok := byID[id]; ok {
					cell.HeaderRefs = append(cell.HeaderRefs, ref)
				}
			}
		}
	}
}
