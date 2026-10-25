package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/go-pdf/internal/testkit"
)

func writePDF(t *testing.T) string {
	t.Helper()
	data := testkit.Build([]testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /Names 7 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font << /F1 6 0 R >> >> /Annots [5 0 R] /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("BT /F1 12 Tf 10 50 Td (Hi) Tj ET")),
		testkit.Raw(5, "<< /Type /Annot /Subtype /Link /Rect [10 10 50 30] /A << /S /URI /URI (https://example.test/) >> >>"),
		testkit.Raw(6, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
		testkit.Raw(7, "<< /EmbeddedFiles << /Names [(note.txt) 8 0 R] >> >>"),
		testkit.Raw(8, "<< /Type /Filespec /F (note.txt) /UF (note.txt) /EF << /F 9 0 R >> >>"),
		testkit.Stream(9, "", []byte("hello attachment")),
	}, "/Root 1 0 R")
	path := filepath.Join(t.TempDir(), "test.pdf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLIInfoTextRenderLinks(t *testing.T) {
	path := writePDF(t)
	var out, errOut bytes.Buffer
	if err := run([]string{"info", path}, &out, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	if !strings.Contains(out.String(), "Pages: 1") {
		t.Fatalf("info %q", out.String())
	}
	out.Reset()
	if err := run([]string{"info", "-json", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"pages":1`) || !strings.Contains(out.String(), `"version"`) {
		t.Fatalf("info json %q", out.String())
	}
	out.Reset()
	if err := run([]string{"text", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Hi") {
		t.Fatalf("text %q", out.String())
	}
	out.Reset()
	if err := run([]string{"text", "-jsonl", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"page":1`) || !strings.Contains(out.String(), "Hi") {
		t.Fatalf("jsonl text %q", out.String())
	}
	out.Reset()
	if err := run([]string{"search", "hi", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Hi") {
		t.Fatalf("search %q", out.String())
	}
	out.Reset()
	if err := run([]string{"search", "-jsonl", "hi", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"page":1`) || !strings.Contains(out.String(), `"charQuads"`) {
		t.Fatalf("search jsonl %q", out.String())
	}
	out.Reset()
	if err := run([]string{"preflight", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "render\tapproximate/success") {
		t.Fatalf("preflight %q", out.String())
	}
	out.Reset()
	if err := run([]string{"preflight", "-json", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"pages":1`) || !strings.Contains(out.String(), `"domains"`) {
		t.Fatalf("preflight json %q", out.String())
	}
	out.Reset()
	if err := run([]string{"links", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "https://example.test/") {
		t.Fatalf("links %q", out.String())
	}
	out.Reset()
	if err := run([]string{"links", "-jsonl", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"page":1`) || !strings.Contains(out.String(), `"uri":"https://example.test/"`) {
		t.Fatalf("links jsonl %q", out.String())
	}
	var capOut bytes.Buffer
	if err := run([]string{"capabilities"}, &capOut, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(capOut.String(), "security") {
		t.Fatalf("capabilities %q", capOut.String())
	}
	capOut.Reset()
	if err := run([]string{"capabilities", "-profile", "core"}, &capOut, &errOut); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(capOut.String(), "render\t") {
		t.Fatalf("core capabilities contain render: %q", capOut.String())
	}
	capOut.Reset()
	if err := run([]string{"capabilities", "-profile", "extended"}, &capOut, &errOut); err != nil {
		t.Fatal(err)
	}
	pngPath := filepath.Join(t.TempDir(), "page.png")
	if err := run([]string{"render", "-o", pngPath, "-dpi", "72", path}, &out, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	f, err := os.Open(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil || img.Bounds().Dx() != 100 {
		t.Fatalf("png %v %v", img, err)
	}
}

func TestCLIEncryptedPassword(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "encrypted", "rc4-128.pdf")
	var out, errOut bytes.Buffer
	if err := run([]string{"text", "-password", "user", path}, &out, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	if !strings.Contains(out.String(), "Hello encrypted") {
		t.Fatalf("text %q", out.String())
	}
	out.Reset()
	if err := run([]string{"text", path}, &out, &errOut); err == nil {
		t.Fatal("expected password error")
	}
}

func TestCLICompare(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "a.png")
	b := filepath.Join(dir, "b.png")
	for _, path := range []string{a, b} {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	if err := run([]string{"compare", "-json", a, b}, &out, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	if !strings.Contains(out.String(), `"SSIM":1`) {
		t.Fatalf("compare %q", out.String())
	}
}

func TestCLIDocumentJSON(t *testing.T) {
	path := writePDF(t)
	var out, errOut bytes.Buffer
	if err := run([]string{"attachments", "-json", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name":"note.txt"`) || !strings.Contains(out.String(), `"size":0`) {
		t.Fatalf("attachments json %q", out.String())
	}
	out.Reset()
	if err := run([]string{"outlines", "-json", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("outlines json %q", out.String())
	}
	out.Reset()
	if err := run([]string{"forms", "-json", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("forms json %q", out.String())
	}
}

func TestCLISVG(t *testing.T) {
	if _, err := exec.LookPath("pdftocairo"); err != nil {
		t.Skip("pdftocairo not installed")
	}
	path := writePDF(t)
	outPath := filepath.Join(t.TempDir(), "page.svg")
	var out, errOut bytes.Buffer
	if err := run([]string{"svg", "-o", outPath, path}, &out, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	svg, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(svg), "<svg") {
		t.Fatalf("svg = %q", svg[:min(len(svg), 80)])
	}
	if err := run([]string{"svg", "-page", "2", path}, &out, &errOut); err == nil {
		t.Fatal("expected page range error")
	}
}

func TestCLIAttachmentsExtract(t *testing.T) {
	path := writePDF(t)
	dir := filepath.Join(t.TempDir(), "attachments")
	var out, errOut bytes.Buffer
	if err := run([]string{"attachments", "-extract", dir, path}, &out, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello attachment" {
		t.Fatalf("data = %q", data)
	}
	if !strings.Contains(out.String(), "note.txt") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestSafeAttachmentName(t *testing.T) {
	cases := map[string]string{
		"../evil.txt":     "evil.txt",
		"..":              "attachment",
		"":                "attachment",
		"a\\b.txt":        "b.txt",
		"a\x00b.txt":      "ab.txt",
		"nested/note.txt": "note.txt",
	}
	for in, want := range cases {
		if got := safeAttachmentName(in); got != want {
			t.Errorf("safeAttachmentName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCLIPageLabels(t *testing.T) {
	path := writePDF(t)
	var out, errOut bytes.Buffer
	if err := run([]string{"labels", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "1\t1" {
		t.Fatalf("labels %q", out.String())
	}
	out.Reset()
	if err := run([]string{"labels", "-json", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"page":1`) || !strings.Contains(out.String(), `"label":"1"`) {
		t.Fatalf("labels json %q", out.String())
	}
}

func writeBrokenPDF(t *testing.T) string {
	t.Helper()
	data := testkit.Build([]testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"),
		testkit.Raw(5, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 8 0 R >>"),
		testkit.Raw(8, "<< /Length 4 /Filter /FlateDecode >>\nstream\nxxxx\nendstream"),
	}, "/Root 1 0 R")
	path := filepath.Join(t.TempDir(), "broken.pdf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLIPreflightFailedPages(t *testing.T) {
	path := writeBrokenPDF(t)
	var out, errOut bytes.Buffer
	if err := run([]string{"preflight", path}, &out, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	if !strings.Contains(out.String(), "Failed: 1") || !strings.Contains(out.String(), "FailedPages: [1]") || !strings.Contains(out.String(), "Failure: page 2") {
		t.Fatalf("preflight %q", out.String())
	}
	out.Reset()
	if err := run([]string{"preflight", "-json", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"failed":1`) || !strings.Contains(out.String(), `"failedPages":[1]`) || !strings.Contains(out.String(), `"failures":[{"page":1`) {
		t.Fatalf("preflight json %q", out.String())
	}
}

func TestCLITextJSONLPageFailure(t *testing.T) {
	path := writeBrokenPDF(t)
	var out, errOut bytes.Buffer
	if err := run([]string{"text", "-jsonl", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"page":2`) || !strings.Contains(out.String(), `"code":"`) {
		t.Fatalf("text jsonl %q", out.String())
	}
}

func TestCLIAnnotations(t *testing.T) {
	path := writePDF(t)
	var out, errOut bytes.Buffer
	if err := run([]string{"annotations", "-jsonl", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"subtype":"Link"`) || !strings.Contains(out.String(), `"uri":"https://example.test/"`) {
		t.Fatalf("annotations jsonl %q", out.String())
	}
	out.Reset()
	if err := run([]string{"annotations", "-page", "1", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Link") {
		t.Fatalf("annotations %q", out.String())
	}
}

func writeTablePDF(t *testing.T) string {
	t.Helper()
	data := testkit.Build([]testkit.Obj{
		testkit.Raw(1, "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 5 0 R >>"),
		testkit.Raw(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		testkit.Raw(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 9 0 R >> >> /Contents 4 0 R >>"),
		testkit.Stream(4, "", []byte("/TD << /MCID 0 >> BDC BT /F1 12 Tf 10 50 Td (Cell A) Tj ET EMC")),
		testkit.Raw(5, "<< /Type /StructTreeRoot /K [6 0 R] >>"),
		testkit.Raw(6, "<< /Type /StructElem /S /Table /K [7 0 R] >>"),
		testkit.Raw(7, "<< /Type /StructElem /S /TR /K [8 0 R] >>"),
		testkit.Raw(8, "<< /Type /StructElem /S /TD /Pg 3 0 R /K 0 >>"),
		testkit.Raw(9, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	}, "/Root 1 0 R")
	path := filepath.Join(t.TempDir(), "table.pdf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLITables(t *testing.T) {
	path := writeTablePDF(t)
	var out, errOut bytes.Buffer
	if err := run([]string{"tables", "-jsonl", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"text":"Cell A"`) || !strings.Contains(out.String(), `"header":false`) {
		t.Fatalf("tables jsonl %q", out.String())
	}
	out.Reset()
	if err := run([]string{"tables", "-page", "1", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Cell A") {
		t.Fatalf("tables %q", out.String())
	}
	out.Reset()
	if err := run([]string{"tables", "-json", path}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"columns":1`) || !strings.Contains(out.String(), `"text":"Cell A"`) {
		t.Fatalf("tables json %q", out.String())
	}
}
