// Command pdf provides basic inspection, text extraction and page rendering.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/F31/go-pdf"
	"github.com/F31/go-pdf/extension/cairo"
	"github.com/F31/go-pdf/internal/quality"
	"github.com/F31/go-pdf/render"
	pdftext "github.com/F31/go-pdf/text"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "pdf:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stdout)
		return errors.New("missing command")
	}
	switch args[0] {
	case "info":
		return infoCmd(args[1:], stdout, stderr)
	case "text":
		return textCmd(args[1:], stdout, stderr)
	case "search":
		return searchCmd(args[1:], stdout, stderr)
	case "render":
		return renderCmd(args[1:], stdout, stderr)
	case "links":
		return linksCmd(args[1:], stdout, stderr)
	case "annotations":
		return annotationsCmd(args[1:], stdout, stderr)
	case "outlines":
		return outlinesCmd(args[1:], stdout, stderr)
	case "labels":
		return labelsCmd(args[1:], stdout, stderr)
	case "forms":
		return formsCmd(args[1:], stdout, stderr)
	case "tables":
		return tablesCmd(args[1:], stdout, stderr)
	case "attachments":
		return attachmentsCmd(args[1:], stdout, stderr)
	case "preflight":
		return preflightCmd(args[1:], stdout, stderr)
	case "capabilities":
		return capabilitiesCmd(args[1:], stdout, stderr)
	case "compare":
		return compareCmd(args[1:], stdout, stderr)
	case "svg":
		return svgCmd(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: pdf <info|text|search|render|svg|links|annotations|outlines|labels|forms|tables|attachments|preflight|capabilities|compare> [options] file.pdf")
	fmt.Fprintln(w, "  -password may be supplied to any PDF-reading command")
	fmt.Fprintln(w, "  info       document metadata and page count (-json)")
	fmt.Fprintln(w, "  text       extract text (-page N, -jsonl, -workers N)")
	fmt.Fprintln(w, "  search     find text (-page N, -case, -whole, -jsonl)")
	fmt.Fprintln(w, "  render     render a page to PNG (-o file, -page N, -dpi N)")
	fmt.Fprintln(w, "  svg        render a page to SVG through pdftocairo (-o file, -page N)")
	fmt.Fprintln(w, "  links      list page links (-page N, -jsonl, default all)")
	fmt.Fprintln(w, "  annotations list page annotations (-page N, -jsonl, default all)")
	fmt.Fprintln(w, "  outlines   print the outline tree (-json)")
	fmt.Fprintln(w, "  labels     print page labels (-json)")
	fmt.Fprintln(w, "  forms      print AcroForm fields (-json)")
	fmt.Fprintln(w, "  tables     print tagged table cells (-page N, -jsonl, -json)")
	fmt.Fprintln(w, "  attachments print or extract embedded files (-json, -extract dir)")
	fmt.Fprintln(w, "  preflight  scan pages and report capability/outcome by domain")
	fmt.Fprintln(w, "  capabilities  print the static feature report (-profile core|preview|extended)")
	fmt.Fprintln(w, "  compare    compare two rendered images (-json, quality thresholds)")
}

func openDoc(path, password string) (*pdf.Document, error) {
	return pdf.OpenFile(context.Background(), path, pdf.WithPassword(password))
}

func pageArg(d *pdf.Document, n int) (int, error) {
	if n < 1 || n > d.NumPages() {
		return 0, fmt.Errorf("page %d out of range 1..%d", n, d.NumPages())
	}
	return n - 1, nil
}

func infoCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("info", flag.ContinueOnError)
	fs.SetOutput(stderr)
	password := fs.String("password", "", "document password")
	asJSON := fs.Bool("json", false, "emit document metadata as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New("info requires one PDF path")
	}
	d, err := openDoc(rest[0], *password)
	if err != nil {
		return err
	}
	defer d.Close()
	info := d.Info()
	if *asJSON {
		item := struct {
			Pages        int    `json:"pages"`
			Version      string `json:"version"`
			Encrypted    bool   `json:"encrypted"`
			Repaired     bool   `json:"repaired"`
			Title        string `json:"title,omitempty"`
			Author       string `json:"author,omitempty"`
			Subject      string `json:"subject,omitempty"`
			Keywords     string `json:"keywords,omitempty"`
			Creator      string `json:"creator,omitempty"`
			Producer     string `json:"producer,omitempty"`
			CreationDate string `json:"creationDate,omitempty"`
			ModDate      string `json:"modDate,omitempty"`
			HasXMP       bool   `json:"hasXMP"`
		}{
			Pages: d.NumPages(), Version: d.Version(), Encrypted: d.Encrypted(), Repaired: d.Repaired(),
			Title: info.Title, Author: info.Author, Subject: info.Subject, Keywords: info.Keywords,
			Creator: info.Creator, Producer: info.Producer, CreationDate: info.CreationDate,
			ModDate: info.ModDate, HasXMP: len(info.XMP) > 0,
		}
		return json.NewEncoder(stdout).Encode(item)
	}
	fmt.Fprintf(stdout, "Pages: %d\nVersion: %s\nEncrypted: %t\nRepaired: %t\n", d.NumPages(), d.Version(), d.Encrypted(), d.Repaired())
	if info.Title != "" {
		fmt.Fprintf(stdout, "Title: %s\n", info.Title)
	}
	if info.Author != "" {
		fmt.Fprintf(stdout, "Author: %s\n", info.Author)
	}
	if info.Producer != "" {
		fmt.Fprintf(stdout, "Producer: %s\n", info.Producer)
	}
	return nil
}

func textCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("text", flag.ContinueOnError)
	fs.SetOutput(stderr)
	page := fs.Int("page", 0, "1-based page number; 0 means all pages")
	jsonl := fs.Bool("jsonl", false, "emit one JSON object per page")
	workers := fs.Int("workers", 0, "bounded page workers; 0 uses the default")
	password := fs.String("password", "", "document password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New("text requires one PDF path")
	}
	d, err := openDoc(rest[0], *password)
	if err != nil {
		return err
	}
	defer d.Close()
	if *page > 0 {
		idx, err := pageArg(d, *page)
		if err != nil {
			return err
		}
		p, err := d.Page(idx)
		if err != nil {
			return err
		}
		t, err := p.Text(context.Background())
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, t.Text)
		return nil
	}
	result, err := d.ExtractText(context.Background(), pdf.ExtractOptions{Workers: *workers})
	if result == nil {
		return err
	}
	if *jsonl {
		enc := json.NewEncoder(stdout)
		for _, row := range result.Pages {
			item := struct {
				Page  int    `json:"page"`
				Text  string `json:"text"`
				Error string `json:"error,omitempty"`
				Code  string `json:"code,omitempty"`
			}{Page: row.Index + 1}
			if row.Err != nil {
				item.Error = row.Err.Error()
				if pe, ok := row.Err.(*pdf.Error); ok {
					item.Code = pe.Code
				}
			} else if row.Text != nil {
				item.Text = row.Text.Text
			}
			if encErr := enc.Encode(item); encErr != nil {
				return encErr
			}
		}
		return err
	}
	for i, row := range result.Pages {
		if row.Err != nil {
			return row.Err
		}
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		if row.Text != nil {
			fmt.Fprintln(stdout, row.Text.Text)
		}
	}
	return err
}

func renderCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "output PNG path, or - for stdout")
	page := fs.Int("page", 1, "1-based page number")
	dpi := fs.Int("dpi", 150, "render DPI")
	bg := fs.String("background", "white", "white or transparent")
	annots := fs.Bool("annotations", false, "include annotation appearances")
	password := fs.String("password", "", "document password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 || *out == "" {
		return errors.New("render requires -o output and one PDF path")
	}
	d, err := openDoc(rest[0], *password)
	if err != nil {
		return err
	}
	defer d.Close()
	idx, err := pageArg(d, *page)
	if err != nil {
		return err
	}
	p, err := d.Page(idx)
	if err != nil {
		return err
	}
	img, err := render.RenderPage(context.Background(), p, render.PageOptions{
		DPI: *dpi, Background: *bg, IncludeText: true, Annotations: annotationMode(*annots),
	})
	if err != nil {
		return err
	}
	var w io.Writer = stdout
	var f *os.File
	if *out != "-" {
		f, err = os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	return render.EncodePNG(w, img)
}

func linksCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("links", flag.ContinueOnError)
	fs.SetOutput(stderr)
	page := fs.Int("page", 0, "1-based page number; 0 means all pages")
	jsonl := fs.Bool("jsonl", false, "emit one JSON object per link")
	password := fs.String("password", "", "document password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 1 {
		return errors.New("links requires one PDF path")
	}
	d, err := openDoc(fs.Args()[0], *password)
	if err != nil {
		return err
	}
	defer d.Close()
	start, end := 0, d.NumPages()
	if *page > 0 {
		idx, err := pageArg(d, *page)
		if err != nil {
			return err
		}
		start, end = idx, idx+1
	}
	var enc *json.Encoder
	if *jsonl {
		enc = json.NewEncoder(stdout)
	}
	for i := start; i < end; i++ {
		p, err := d.Page(i)
		if err != nil {
			return err
		}
		links, err := p.Links(context.Background())
		if err != nil {
			return err
		}
		for _, l := range links {
			if enc != nil {
				item := struct {
					Page     int     `json:"page"`
					URI      string  `json:"uri,omitempty"`
					DestPage int     `json:"destPage,omitempty"`
					X0       float64 `json:"x0"`
					Y0       float64 `json:"y0"`
					X1       float64 `json:"x1"`
					Y1       float64 `json:"y1"`
				}{Page: i + 1, URI: l.URI, X0: l.Rect.X0, Y0: l.Rect.Y0, X1: l.Rect.X1, Y1: l.Rect.Y1}
				if l.Page >= 0 {
					item.DestPage = l.Page + 1
				}
				if err := enc.Encode(item); err != nil {
					return err
				}
				continue
			}
			target := l.URI
			if target == "" {
				target = fmt.Sprintf("page %d", l.Page+1)
			}
			fmt.Fprintf(stdout, "%d\t%.1f\t%.1f\t%.1f\t%.1f\t%s\n", i+1, l.Rect.X0, l.Rect.Y0, l.Rect.X1, l.Rect.Y1, target)
		}
	}
	return nil
}

func annotationsCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("annotations", flag.ContinueOnError)
	fs.SetOutput(stderr)
	password := fs.String("password", "", "document password")
	page := fs.Int("page", 0, "1-based page (default all)")
	jsonl := fs.Bool("jsonl", false, "emit one JSON object per annotation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New("annotations requires one PDF path")
	}
	d, err := openDoc(rest[0], *password)
	if err != nil {
		return err
	}
	defer d.Close()
	start, end := 0, d.NumPages()
	if *page > 0 {
		idx, err := pageArg(d, *page)
		if err != nil {
			return err
		}
		start, end = idx, idx+1
	}
	var enc *json.Encoder
	if *jsonl {
		enc = json.NewEncoder(stdout)
	}
	for i := start; i < end; i++ {
		p, err := d.Page(i)
		if err != nil {
			return err
		}
		annots, err := p.Annotations(context.Background())
		if err != nil {
			return err
		}
		for _, a := range annots {
			if enc != nil {
				item := struct {
					Page       int     `json:"page"`
					Subtype    string  `json:"subtype"`
					X0         float64 `json:"x0"`
					Y0         float64 `json:"y0"`
					X1         float64 `json:"x1"`
					Y1         float64 `json:"y1"`
					Contents   string  `json:"contents,omitempty"`
					URI        string  `json:"uri,omitempty"`
					DestPage   int     `json:"destPage,omitempty"`
					Flags      int     `json:"flags,omitempty"`
					FieldName  string  `json:"fieldName,omitempty"`
					FieldValue string  `json:"fieldValue,omitempty"`
					FieldType  string  `json:"fieldType,omitempty"`
				}{
					Page: i + 1, Subtype: a.Subtype, X0: a.Rect.X0, Y0: a.Rect.Y0, X1: a.Rect.X1, Y1: a.Rect.Y1,
					Contents: a.Contents, URI: a.URI, Flags: a.Flags,
					FieldName: a.FieldName, FieldValue: a.FieldValue, FieldType: a.FieldType,
				}
				if a.Page >= 0 {
					item.DestPage = a.Page + 1
				}
				if err := enc.Encode(item); err != nil {
					return err
				}
				continue
			}
			target := a.Contents
			if target == "" {
				target = a.URI
			}
			if target == "" {
				target = a.FieldName
			}
			fmt.Fprintf(stdout, "%d\t%s\t%.1f %.1f %.1f %.1f\t%s\n", i+1, a.Subtype, a.Rect.X0, a.Rect.Y0, a.Rect.X1, a.Rect.Y1, target)
		}
	}
	return nil
}

func outlinesCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("outlines", flag.ContinueOnError)
	fs.SetOutput(stderr)
	password := fs.String("password", "", "document password")
	asJSON := fs.Bool("json", false, "emit a JSON outline tree")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New("outlines requires one PDF path")
	}
	d, err := openDoc(rest[0], *password)
	if err != nil {
		return err
	}
	defer d.Close()
	outline, err := d.Outlines(context.Background())
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(stdout).Encode(outlineJSON(outline))
	}
	var walk func([]pdf.Outline, int)
	walk = func(nodes []pdf.Outline, depth int) {
		for _, n := range nodes {
			target := n.URI
			if target == "" {
				target = fmt.Sprintf("page %d", n.Page+1)
			}
			fmt.Fprintf(stdout, "%s%s\t%s\n", strings.Repeat("  ", depth), n.Title, target)
			walk(n.Children, depth+1)
		}
	}
	walk(outline, 0)
	return nil
}

func labelsCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("labels", flag.ContinueOnError)
	fs.SetOutput(stderr)
	password := fs.String("password", "", "document password")
	asJSON := fs.Bool("json", false, "emit page labels as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New("labels requires one PDF path")
	}
	d, err := openDoc(rest[0], *password)
	if err != nil {
		return err
	}
	defer d.Close()
	type labelJSON struct {
		Page  int    `json:"page"`
		Label string `json:"label"`
	}
	var items []labelJSON
	for i := 0; i < d.NumPages(); i++ {
		label, err := d.PageLabel(context.Background(), i)
		if err != nil {
			return err
		}
		if *asJSON {
			items = append(items, labelJSON{Page: i + 1, Label: label})
			continue
		}
		fmt.Fprintf(stdout, "%d\t%s\n", i+1, label)
	}
	if *asJSON {
		return json.NewEncoder(stdout).Encode(items)
	}
	return nil
}

func formsCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("forms", flag.ContinueOnError)
	fs.SetOutput(stderr)
	password := fs.String("password", "", "document password")
	asJSON := fs.Bool("json", false, "emit a JSON field tree")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New("forms requires one PDF path")
	}
	d, err := openDoc(rest[0], *password)
	if err != nil {
		return err
	}
	defer d.Close()
	fields, err := d.FormFields(context.Background())
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(stdout).Encode(formJSON(fields))
	}
	var walk func([]pdf.FormField, int)
	walk = func(nodes []pdf.FormField, depth int) {
		for _, f := range nodes {
			fmt.Fprintf(stdout, "%s%s\t%s\t%s\tpage %d\n", strings.Repeat("  ", depth), f.Name, f.Type, f.Value, f.Page+1)
			walk(f.Children, depth+1)
		}
	}
	walk(fields, 0)
	return nil
}

type outlineJSONNode struct {
	Title    string            `json:"title"`
	Page     int               `json:"page,omitempty"`
	URI      string            `json:"uri,omitempty"`
	Children []outlineJSONNode `json:"children,omitempty"`
}

func outlineJSON(nodes []pdf.Outline) []outlineJSONNode {
	out := make([]outlineJSONNode, 0, len(nodes))
	for _, n := range nodes {
		item := outlineJSONNode{Title: n.Title, URI: n.URI}
		if n.Page >= 0 {
			item.Page = n.Page + 1
		}
		item.Children = outlineJSON(n.Children)
		out = append(out, item)
	}
	return out
}

type formJSONNode struct {
	Name     string         `json:"name"`
	Type     string         `json:"type,omitempty"`
	Value    string         `json:"value,omitempty"`
	Page     int            `json:"page,omitempty"`
	Flags    int            `json:"flags,omitempty"`
	Children []formJSONNode `json:"children,omitempty"`
}

func formJSON(nodes []pdf.FormField) []formJSONNode {
	out := make([]formJSONNode, 0, len(nodes))
	for _, n := range nodes {
		item := formJSONNode{Name: n.Name, Type: n.Type, Value: n.Value, Flags: n.Flags}
		if n.Page >= 0 {
			item.Page = n.Page + 1
		}
		item.Children = formJSON(n.Children)
		out = append(out, item)
	}
	return out
}

type attachmentJSON struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MIME        string `json:"mime,omitempty"`
	ModDate     string `json:"modDate,omitempty"`
	Size        int64  `json:"size"`
}

func tablesCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("tables", flag.ContinueOnError)
	fs.SetOutput(stderr)
	password := fs.String("password", "", "document password")
	page := fs.Int("page", 0, "1-based page (default all)")
	jsonl := fs.Bool("jsonl", false, "emit one JSON object per table cell")
	asJSON := fs.Bool("json", false, "emit full table trees as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New("tables requires one PDF path")
	}
	if *jsonl && *asJSON {
		return errors.New("tables: choose -jsonl or -json")
	}
	d, err := openDoc(rest[0], *password)
	if err != nil {
		return err
	}
	defer d.Close()
	start, end := 0, d.NumPages()
	if *page > 0 {
		idx, err := pageArg(d, *page)
		if err != nil {
			return err
		}
		start, end = idx, idx+1
	}
	var enc *json.Encoder
	if *jsonl {
		enc = json.NewEncoder(stdout)
	}
	var all []tableJSONData
	for i := start; i < end; i++ {
		p, err := d.Page(i)
		if err != nil {
			return err
		}
		tables, err := p.Tables(context.Background())
		if err != nil {
			return err
		}
		for ti, table := range tables {
			if *asJSON {
				all = append(all, tableJSON(table))
				continue
			}
			for ri, row := range table.Rows {
				for _, cell := range row.Cells {
					if enc != nil {
						item := struct {
							Page       int      `json:"page"`
							Table      int      `json:"table"`
							Row        int      `json:"row"`
							Col        int      `json:"col"`
							Header     bool     `json:"header"`
							RowSpan    int      `json:"rowSpan"`
							ColSpan    int      `json:"colSpan"`
							Text       string   `json:"text"`
							Headers    []string `json:"headers,omitempty"`
							Paragraphs []string `json:"paragraphs,omitempty"`
						}{
							Page: cell.Page + 1, Table: ti, Row: ri, Col: cell.Col, Header: cell.Header,
							RowSpan: cell.RowSpan, ColSpan: cell.ColSpan, Text: cell.Text,
							Headers: cell.Headers, Paragraphs: cell.Paragraphs,
						}
						if err := enc.Encode(item); err != nil {
							return err
						}
						continue
					}
					fmt.Fprintf(stdout, "%d\t%d\t%d\t%d\t%t\t%d\t%d\t%s\n",
						cell.Page+1, ti, ri, cell.Col, cell.Header, cell.RowSpan, cell.ColSpan, cell.Text)
				}
			}
		}
	}
	if *asJSON {
		return json.NewEncoder(stdout).Encode(all)
	}
	return nil
}

type tableRect struct {
	X0 float64 `json:"x0"`
	Y0 float64 `json:"y0"`
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
}

type tableDiagnosticJSON struct {
	Code    string `json:"code"`
	Row     int    `json:"row"`
	Col     int    `json:"col"`
	Message string `json:"message,omitempty"`
}

type tableCellRefJSON struct {
	Row  int `json:"row"`
	Cell int `json:"cell"`
}

type tableCellJSONData struct {
	Page       int                `json:"page"`
	Row        int                `json:"row"`
	Col        int                `json:"col"`
	Header     bool               `json:"header"`
	RowSpan    int                `json:"rowSpan"`
	ColSpan    int                `json:"colSpan"`
	Headers    []string           `json:"headers,omitempty"`
	HeaderRefs []tableCellRefJSON `json:"headerRefs,omitempty"`
	BBox       *tableRect         `json:"bbox,omitempty"`
	Text       string             `json:"text"`
	Paragraphs []string           `json:"paragraphs,omitempty"`
	Nested     []tableJSONData    `json:"nested,omitempty"`
}

type tableRowJSONData struct {
	Page  int                 `json:"page"`
	Cells []tableCellJSONData `json:"cells"`
}

type tableJSONData struct {
	Page            int                   `json:"page"`
	EndPage         int                   `json:"endPage"`
	Columns         int                   `json:"columns"`
	HeaderRows      []int                 `json:"headerRows,omitempty"`
	RepeatedHeaders int                   `json:"repeatedHeaders,omitempty"`
	Heuristic       bool                  `json:"heuristic,omitempty"`
	Diagnostics     []tableDiagnosticJSON `json:"diagnostics,omitempty"`
	Rows            []tableRowJSONData    `json:"rows"`
}

func tablePage(n int) int {
	if n < 0 {
		return 0
	}
	return n + 1
}

func tableJSON(t pdf.Table) tableJSONData {
	out := tableJSONData{
		Page: tablePage(t.Page), EndPage: tablePage(t.EndPage), Columns: t.Columns,
		HeaderRows: append([]int(nil), t.HeaderRows...), RepeatedHeaders: t.RepeatedHeaders, Heuristic: t.Heuristic,
	}
	for _, d := range t.Diagnostics {
		out.Diagnostics = append(out.Diagnostics, tableDiagnosticJSON{Code: d.Code, Row: d.Row, Col: d.Col, Message: d.Message})
	}
	for _, row := range t.Rows {
		r := tableRowJSONData{Page: tablePage(row.Page)}
		for _, cell := range row.Cells {
			c := tableCellJSONData{
				Page: tablePage(cell.Page), Row: cell.Row, Col: cell.Col, Header: cell.Header,
				RowSpan: cell.RowSpan, ColSpan: cell.ColSpan, Headers: append([]string(nil), cell.Headers...),
				Text: cell.Text, Paragraphs: append([]string(nil), cell.Paragraphs...),
			}
			if cell.HasBBox {
				c.BBox = &tableRect{X0: cell.BBox.X0, Y0: cell.BBox.Y0, X1: cell.BBox.X1, Y1: cell.BBox.Y1}
			}
			for _, ref := range cell.HeaderRefs {
				c.HeaderRefs = append(c.HeaderRefs, tableCellRefJSON{Row: ref.Row, Cell: ref.Cell})
			}
			for _, nested := range cell.Nested {
				c.Nested = append(c.Nested, tableJSON(nested))
			}
			r.Cells = append(r.Cells, c)
		}
		out.Rows = append(out.Rows, r)
	}
	return out
}

func capabilitiesCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("capabilities", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profileName := fs.String("profile", string(pdf.ProfilePreview), "core, preview or extended")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("capabilities takes no positional arguments")
	}
	profile, err := pdf.ParseProfile(*profileName)
	if err != nil {
		return err
	}
	caps, err := pdf.CapabilitiesFor(profile)
	if err != nil {
		return err
	}
	for _, c := range caps {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", c.Area, c.Feature, c.Status, c.Note)
	}
	return nil
}

func annotationMode(view bool) render.AnnotationMode {
	if view {
		return render.AnnotationView
	}
	return render.AnnotationNone
}

func attachmentsCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("attachments", flag.ContinueOnError)
	fs.SetOutput(stderr)
	password := fs.String("password", "", "document password")
	extract := fs.String("extract", "", "extract embedded files to this directory")
	asJSON := fs.Bool("json", false, "emit attachment metadata as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New("attachments requires one PDF path")
	}
	d, err := openDoc(rest[0], *password)
	if err != nil {
		return err
	}
	defer d.Close()
	atts, err := d.Attachments(context.Background())
	if err != nil {
		return err
	}
	if *extract == "" {
		if *asJSON {
			items := make([]attachmentJSON, 0, len(atts))
			for _, a := range atts {
				items = append(items, attachmentJSON{Name: a.Name, Description: a.Description, MIME: a.MIME, ModDate: a.ModDate, Size: a.Size})
			}
			return json.NewEncoder(stdout).Encode(items)
		}
		for _, a := range atts {
			fmt.Fprintf(stdout, "%s\t%s\t%d\t%s\n", a.Name, a.Description, a.Size, a.ModDate)
		}
		return nil
	}
	if err := os.MkdirAll(*extract, 0o755); err != nil {
		return err
	}
	used := map[string]bool{}
	for _, a := range atts {
		name := safeAttachmentName(a.Name)
		base := strings.TrimSuffix(name, filepath.Ext(name))
		ext := filepath.Ext(name)
		for i := 1; used[name]; i++ {
			name = fmt.Sprintf("%s-%d%s", base, i, ext)
		}
		used[name] = true
		path := filepath.Join(*extract, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, writeErr := a.WriteTo(context.Background(), f)
		closeErr := f.Close()
		if writeErr == nil {
			writeErr = closeErr
		}
		if writeErr != nil {
			_ = os.Remove(path)
			return fmt.Errorf("attachment %q: %w", a.Name, writeErr)
		}
		fmt.Fprintln(stdout, path)
	}
	return nil
}

func safeAttachmentName(name string) string {
	name = strings.ReplaceAll(name, "\x00", "")
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "" || name == "." || name == ".." {
		return "attachment"
	}
	return name
}

func searchCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	password := fs.String("password", "", "document password")
	page := fs.Int("page", 0, "1-based page (default all)")
	caseSensitive := fs.Bool("case", false, "case-sensitive match")
	whole := fs.Bool("whole", false, "match whole words")
	jsonl := fs.Bool("jsonl", false, "emit one JSON object per hit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return errors.New("search requires a query and one PDF path")
	}
	d, err := openDoc(rest[1], *password)
	if err != nil {
		return err
	}
	defer d.Close()
	opts := pdftext.SearchOptions{CaseSensitive: *caseSensitive, WholeWord: *whole}
	pages := []int{}
	if *page != 0 {
		p, err := pageArg(d, *page)
		if err != nil {
			return err
		}
		pages = append(pages, p)
	} else {
		for i := 0; i < d.NumPages(); i++ {
			pages = append(pages, i)
		}
	}
	var enc *json.Encoder
	if *jsonl {
		enc = json.NewEncoder(stdout)
	}
	for _, i := range pages {
		p, err := d.Page(i)
		if err != nil {
			return err
		}
		hits, err := p.Search(context.Background(), rest[0], opts)
		if err != nil {
			return err
		}
		for _, h := range hits {
			if enc != nil {
				if err := enc.Encode(searchJSON(i+1, h)); err != nil {
					return err
				}
				continue
			}
			fmt.Fprintf(stdout, "%d\t%.2f %.2f %.2f %.2f\t%s\n", i+1, h.Bounds.X0, h.Bounds.Y0, h.Bounds.X1, h.Bounds.Y1, h.Text)
		}
	}
	return nil
}

func searchJSON(page int, h pdftext.Hit) any {
	type point struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	type rect struct {
		X0 float64 `json:"x0"`
		Y0 float64 `json:"y0"`
		X1 float64 `json:"x1"`
		Y1 float64 `json:"y1"`
	}
	item := struct {
		Page      int        `json:"page"`
		Text      string     `json:"text"`
		Bounds    rect       `json:"bounds"`
		Quads     []rect     `json:"quads,omitempty"`
		CharQuads [][4]point `json:"charQuads,omitempty"`
	}{Page: page, Text: h.Text, Bounds: rect{h.Bounds.X0, h.Bounds.Y0, h.Bounds.X1, h.Bounds.Y1}}
	for _, q := range h.Quads {
		item.Quads = append(item.Quads, rect{q.X0, q.Y0, q.X1, q.Y1})
	}
	for _, q := range h.CharQuads {
		item.CharQuads = append(item.CharQuads, [4]point{{q[0].X, q[0].Y}, {q[1].X, q[1].Y}, {q[2].X, q[2].Y}, {q[3].X, q[3].Y}})
	}
	return item
}

func compareCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "emit JSON")
	minPSNR := fs.Float64("min-psnr", 0, "fail below this PSNR")
	minSSIM := fs.Float64("min-ssim", 0, "fail below this SSIM")
	maxDeltaE := fs.Float64("max-deltae", 0, "fail above this mean DeltaE")
	maxP95 := fs.Float64("max-deltae-p95", 0, "fail above this p95 DeltaE")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return errors.New("compare requires two image paths")
	}
	load := func(path string) (image.Image, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		return img, err
	}
	a, err := load(rest[0])
	if err != nil {
		return err
	}
	b, err := load(rest[1])
	if err != nil {
		return err
	}
	rep, err := quality.Compare(a, b)
	if err != nil {
		return err
	}
	if *asJSON {
		out := struct {
			Width, Height int
			PSNR          any
			SSIM          float64
			MeanDeltaE    float64
			P95DeltaE     float64
		}{Width: rep.Width, Height: rep.Height, SSIM: rep.SSIM, MeanDeltaE: rep.MeanDeltaE, P95DeltaE: rep.P95DeltaE}
		if math.IsInf(rep.PSNR, 0) {
			out.PSNR = nil
		} else {
			out.PSNR = rep.PSNR
		}
		if err := json.NewEncoder(stdout).Encode(out); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(stdout, "size\t%d x %d\nPSNR\t%.4f\nSSIM\t%.6f\nDeltaE mean\t%.4f\nDeltaE p95\t%.4f\n",
			rep.Width, rep.Height, rep.PSNR, rep.SSIM, rep.MeanDeltaE, rep.P95DeltaE)
	}
	return quality.Thresholds{
		MinPSNR: *minPSNR, MinSSIM: *minSSIM,
		MaxMeanDeltaE: *maxDeltaE, MaxP95DeltaE: *maxP95,
	}.Check(rep)
}

func svgCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("svg", flag.ContinueOnError)
	fs.SetOutput(stderr)
	password := fs.String("password", "", "document password")
	page := fs.Int("page", 1, "page number (1-based)")
	output := fs.String("o", "", "output SVG file (default stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New("usage: pdf svg [-page N] [-o file] file.pdf")
	}
	d, err := openDoc(rest[0], *password)
	if err != nil {
		return err
	}
	idx, err := pageArg(d, *page)
	_ = d.Close()
	if err != nil {
		return err
	}
	r, err := cairo.New("")
	if err != nil {
		return fmt.Errorf("svg: %w", err)
	}
	svg, err := r.RenderSVG(context.Background(), rest[0], idx)
	if err != nil {
		return err
	}
	if *output == "" {
		_, err = stdout.Write(svg)
		return err
	}
	return os.WriteFile(*output, svg, 0o644)
}

func preflightCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("preflight", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workers := fs.Int("workers", 0, "bounded page workers; 0 uses the default")
	jsonOut := fs.Bool("json", false, "emit JSON")
	password := fs.String("password", "", "document password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New("preflight requires one PDF path")
	}
	d, err := openDoc(rest[0], *password)
	if err != nil {
		return err
	}
	defer d.Close()
	rep, err := d.PreflightWithOptions(context.Background(), pdf.PreflightOptions{Workers: *workers})
	if err != nil {
		return err
	}
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		return enc.Encode(rep)
	}
	fmt.Fprintf(stdout, "Pages: %d\nScanned: %d\nFailed: %d\nEncrypted: %t\nRepaired: %t\n", rep.Pages, rep.Scanned, rep.Failed, rep.Encrypted, rep.Repaired)
	if len(rep.FailedPages) > 0 {
		fmt.Fprintf(stdout, "FailedPages: %v\n", rep.FailedPages)
	}
	for _, f := range rep.Failures {
		fmt.Fprintf(stdout, "Failure: page %d\t%s\n", f.Page+1, f.Code)
	}
	for _, domain := range []pdf.Domain{pdf.DomainParse, pdf.DomainExtract, pdf.DomainRender} {
		r := rep.Domains[domain]
		fmt.Fprintf(stdout, "%s\t%s/%s\n", domain, r.Status, r.Outcome)
		for _, note := range r.Notes {
			fmt.Fprintf(stdout, "  %s\n", note)
		}
	}
	return nil
}
