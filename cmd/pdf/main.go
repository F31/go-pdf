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
	"strings"

	"github.com/F31/go-pdf"
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
	case "outlines":
		return outlinesCmd(args[1:], stdout, stderr)
	case "forms":
		return formsCmd(args[1:], stdout, stderr)
	case "attachments":
		return attachmentsCmd(args[1:], stdout, stderr)
	case "preflight":
		return preflightCmd(args[1:], stdout, stderr)
	case "capabilities":
		return capabilitiesCmd(args[1:], stdout)
	case "compare":
		return compareCmd(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: pdf <info|text|search|render|links|outlines|forms|attachments|preflight|capabilities|compare> [options] file.pdf")
	fmt.Fprintln(w, "  -password may be supplied to any PDF-reading command")
	fmt.Fprintln(w, "  info       document metadata and page count")
	fmt.Fprintln(w, "  text       extract text (optional -page N, 1-based)")
	fmt.Fprintln(w, "  search     find text (-page N, -case, -whole)")
	fmt.Fprintln(w, "  render     render a page to PNG (-o file, -page N, -dpi N)")
	fmt.Fprintln(w, "  links      list page links (-page N, default all)")
	fmt.Fprintln(w, "  outlines   print the outline tree")
	fmt.Fprintln(w, "  forms      print AcroForm fields")
	fmt.Fprintln(w, "  attachments print embedded files")
	fmt.Fprintln(w, "  preflight  scan pages and report capability/outcome by domain")
	fmt.Fprintln(w, "  capabilities  print the static feature report")
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
	for i := 0; i < d.NumPages(); i++ {
		p, err := d.Page(i)
		if err != nil {
			return err
		}
		t, err := p.Text(context.Background())
		if err != nil {
			return err
		}
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		fmt.Fprintln(stdout, t.Text)
	}
	return nil
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
			target := l.URI
			if target == "" {
				target = fmt.Sprintf("page %d", l.Page+1)
			}
			fmt.Fprintf(stdout, "%d\t%.1f\t%.1f\t%.1f\t%.1f\t%s\n", i+1, l.Rect.X0, l.Rect.Y0, l.Rect.X1, l.Rect.Y1, target)
		}
	}
	return nil
}

func outlinesCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("outlines", flag.ContinueOnError)
	fs.SetOutput(stderr)
	password := fs.String("password", "", "document password")
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

func formsCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("forms", flag.ContinueOnError)
	fs.SetOutput(stderr)
	password := fs.String("password", "", "document password")
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

func capabilitiesCmd(args []string, stdout io.Writer) error {
	if len(args) != 0 {
		return errors.New("capabilities takes no arguments")
	}
	for _, c := range pdf.Capabilities() {
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
	for _, a := range atts {
		fmt.Fprintf(stdout, "%s\t%s\t%d\t%s\n", a.Name, a.Description, a.Size, a.ModDate)
	}
	return nil
}

func searchCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	password := fs.String("password", "", "document password")
	page := fs.Int("page", 0, "1-based page (default all)")
	caseSensitive := fs.Bool("case", false, "case-sensitive match")
	whole := fs.Bool("whole", false, "match whole words")
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
			fmt.Fprintf(stdout, "%d\t%.2f %.2f %.2f %.2f\t%s\n", i+1, h.Bounds.X0, h.Bounds.Y0, h.Bounds.X1, h.Bounds.Y1, h.Text)
		}
	}
	return nil
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

func preflightCmd(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("preflight", flag.ContinueOnError)
	fs.SetOutput(stderr)
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
	rep, err := d.Preflight(context.Background())
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Pages: %d\nScanned: %d\nEncrypted: %t\nRepaired: %t\n", rep.Pages, rep.Scanned, rep.Encrypted, rep.Repaired)
	for _, domain := range []pdf.Domain{pdf.DomainParse, pdf.DomainExtract, pdf.DomainRender, pdf.DomainEdit, pdf.DomainExport, pdf.DomainVerify} {
		r := rep.Domains[domain]
		fmt.Fprintf(stdout, "%s\t%s/%s\n", domain, r.Status, r.Outcome)
		for _, note := range r.Notes {
			fmt.Fprintf(stdout, "  %s\n", note)
		}
	}
	return nil
}
