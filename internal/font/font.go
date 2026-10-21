// Package font implements the simple-font subset needed for G1 text
// positioning. CID/CMap/ToUnicode handling belongs to G2.
package font

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/diag"
	"github.com/F31/go-pdf/internal/syntax"
)

type Font struct {
	Info         *displaylist.FontInfo
	firstChar    int
	widths       []float64
	missingWidth float64
	hasWidths    bool
	base14       *[256]uint16
	encoding     *[256]rune
	unicodeOK    *[256]bool

	Type string
}

func Load(ctx context.Context, file *syntax.File, dict syntax.Dict, dc *diag.Collector) (*Font, error) {
	subtype, _ := dict.NameValue("Subtype")
	rawBase, _ := dict.NameValue("BaseFont")
	f := &Font{Type: subtype}
	info := &displaylist.FontInfo{BaseFont: NormalizeBaseFont(rawBase), Subset: isSubsetName(rawBase)}
	base := info.BaseFont
	info.Kind = subtype

	switch base {
	case "Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique":
		info.Family, info.Serif = "Helvetica", false
	case "Times-Roman", "Times-Bold", "Times-Italic", "Times-BoldItalic":
		info.Family, info.Serif = "Times", true
	case "Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique":
		info.Family, info.Mono = "Courier", true
	case "Symbol", "ZapfDingbats":
		info.Family = base
	default:
		info.Family = base
		if i := strings.IndexByte(base, '-'); i > 0 {
			info.Family = base[:i]
		}
	}
	lower := strings.ToLower(base)
	info.Bold = strings.Contains(lower, "bold")
	info.Italic = strings.Contains(lower, "italic") || strings.Contains(lower, "oblique")
	if strings.HasPrefix(strings.ToLower(base), "courier") {
		info.Mono = true
	}
	if strings.HasPrefix(strings.ToLower(base), "times") {
		info.Serif = true
	}

	if subtype == "Type0" {
		dc.Addf(diag.Degraded, "FNT-TYPE0-DEFERRED", "Type0 text uses fallback metrics", -1, 0, -1)
		info.Kind = "Type0"
	} else if subtype == "Type3" {
		dc.Addf(diag.Degraded, "FNT-TYPE3-DEFERRED", "Type3 text uses fallback metrics", -1, 0, -1)
	}

	if table, ok := base14Widths[base]; ok {
		f.base14 = table
	} else if isBase14Name(base) {
		dc.Addf(diag.Warn, "FNT-BASE14-FALLBACK", "no built-in widths for base-14 font", -1, 0, -1)
	}

	if fd, ok := dict.DictValue("FontDescriptor"); ok {
		f.loadDescriptor(ctx, file, fd, info, dc)
	} else if ref, ok := dict.RefValue("FontDescriptor"); ok {
		o, err := file.Resolve(ctx, ref)
		if err == nil {
			if fd, ok := o.(syntax.Dict); ok {
				f.loadDescriptor(ctx, file, fd, info, dc)
			}
		}
	}

	f.encoding = defaultEncoding(subtype, base)
	if v := dict.Get("Encoding"); v != nil {
		if err := f.loadEncoding(ctx, file, v, dc); err != nil {
			return nil, err
		}
	}

	if first, ok := dict.IntValue("FirstChar"); ok {
		f.firstChar = int(first)
	}
	if ref, ok := dict.RefValue("Widths"); ok {
		o, err := file.Resolve(ctx, ref)
		if err == nil {
			f.readWidths(o)
		}
	} else {
		f.readWidths(dict.Get("Widths"))
	}
	if mw, ok := dict.IntValue("MissingWidth"); ok {
		f.missingWidth = float64(mw)
	}
	f.Info = info
	return f, nil
}

func (f *Font) loadDescriptor(ctx context.Context, file *syntax.File, fd syntax.Dict, info *displaylist.FontInfo, dc *diag.Collector) {
	for _, key := range []string{"FontFile", "FontFile2", "FontFile3"} {
		if fd.Has(key) {
			info.Embedded = true
			break
		}
	}
	if v, ok := fd.Get("ItalicAngle").(syntax.Real); ok {
		info.ItalicAngle = float64(v)
	} else if v, ok := fd.Get("ItalicAngle").(syntax.Int); ok {
		info.ItalicAngle = float64(v)
	}
	if fw, ok := fd.IntValue("Mono"); ok && fw != 0 {
		info.Mono = true
	}
	info.Ascent = numeric(fd.Get("Ascent"))
	info.Descent = numeric(fd.Get("Descent"))
	_ = ctx
	_ = file
	_ = dc
}

func (f *Font) readWidths(o syntax.Object) {
	arr, ok := o.(syntax.Array)
	if !ok {
		return
	}
	f.widths = make([]float64, len(arr))
	for i, v := range arr {
		f.widths[i] = numeric(v)
	}
	f.hasWidths = len(arr) > 0
}

func numeric(o syntax.Object) float64 {
	switch v := o.(type) {
	case syntax.Int:
		return float64(v)
	case syntax.Real:
		return float64(v)
	}
	return 0
}

// Width returns the glyph width in 1/1000 text space units.
func (f *Font) Width(code byte) float64 {
	if f == nil {
		return 500
	}
	if f.hasWidths {
		idx := int(code) - f.firstChar
		if idx >= 0 && idx < len(f.widths) {
			if w := f.widths[idx]; w != 0 {
				return w
			}
		}
		if f.missingWidth > 0 {
			return f.missingWidth
		}
	}
	if f.base14 != nil {
		if w := f.base14[code]; w != 0 {
			return float64(w)
		}
	}
	return 500
}

// Decode maps a simple-font code to Unicode.
func (f *Font) Decode(code byte) (string, bool) {
	if f == nil || f.encoding == nil {
		return "\uFFFD", false
	}
	if f.Type == "Type0" || f.Type == "Type3" {
		return "\uFFFD", false
	}
	r := f.encoding[code]
	if r == 0 {
		return "\uFFFD", false
	}
	if f.unicodeOK != nil && !f.unicodeOK[code] {
		return string(r), false
	}
	return string(r), true
}

func NormalizeBaseFont(name string) string {
	if i := strings.IndexByte(name, '+'); i == 6 && len(name) > 7 {
		name = name[7:]
	}
	switch name {
	case "Arial", "ArialMT", "Helvetica":
		return "Helvetica"
	case "Arial-Bold", "Arial-BoldMT", "Helvetica-Bold":
		return "Helvetica-Bold"
	case "Arial-Italic", "Arial-ItalicMT", "Helvetica-Oblique":
		return "Helvetica-Oblique"
	case "Arial-BoldItalic", "Arial-BoldItalicMT", "Helvetica-BoldOblique":
		return "Helvetica-BoldOblique"
	case "TimesNewRoman", "TimesNewRomanPSMT", "Times-Roman":
		return "Times-Roman"
	case "TimesNewRomanPS-BoldMT", "Times-Bold":
		return "Times-Bold"
	case "TimesNewRomanPS-ItalicMT", "Times-Italic":
		return "Times-Italic"
	case "TimesNewRomanPS-BoldItalicMT", "Times-BoldItalic":
		return "Times-BoldItalic"
	case "CourierNew", "CourierNewPSMT", "Courier":
		return "Courier"
	case "CourierNewPS-BoldMT", "Courier-Bold":
		return "Courier-Bold"
	case "CourierNewPS-ItalicMT", "Courier-Oblique":
		return "Courier-Oblique"
	case "CourierNewPS-BoldItalicMT", "Courier-BoldOblique":
		return "Courier-BoldOblique"
	}
	return name
}

func isSubsetName(name string) bool {
	return len(name) > 7 && name[6] == '+' && isUpper(name[:6])
}

func isUpper(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return true
}

func isBase14Name(name string) bool {
	_, ok := base14Widths[name]
	return ok || name == "Symbol" || name == "ZapfDingbats"
}

func (f *Font) loadEncoding(ctx context.Context, file *syntax.File, o syntax.Object, dc *diag.Collector) error {
	if _, ok := o.(syntax.Ref); ok {
		if file == nil {
			return nil
		}
		resolved, err := file.Resolve(ctx, o)
		if err != nil {
			return err
		}
		o = resolved
	}
	switch v := o.(type) {
	case syntax.Name:
		if enc := encodingByName(string(v)); enc != nil {
			f.encoding = enc
			return nil
		}
		dc.Addf(diag.Warn, "FNT-ENCODING", "unknown encoding, using default", -1, 0, -1)
	case syntax.Dict:
		if base, ok := v.NameValue("BaseEncoding"); ok {
			if enc := encodingByName(base); enc != nil {
				f.encoding = enc
			}
		}
		if ref, ok := v.RefValue("Differences"); ok {
			o, err := file.Resolve(ctx, ref)
			if err != nil {
				return err
			}
			v["Differences"] = o
		}
		if diffs, ok := v.ArrayValue("Differences"); ok {
			applyDifferences(f.encoding, diffs)
		}
	}
	return nil
}

func applyDifferences(enc *[256]rune, diffs syntax.Array) {
	code := -1
	for _, item := range diffs {
		switch v := item.(type) {
		case syntax.Int:
			code = int(v)
		case syntax.Name:
			if code >= 0 && code < 256 {
				if r, ok := GlyphNameRune(string(v)); ok {
					enc[code] = r
				}
				code++
			}
		}
	}
}

func defaultEncoding(subtype, base string) *[256]rune {
	if subtype == "TrueType" {
		return winAnsiEncoding()
	}
	if strings.HasPrefix(base, "Symbol") || strings.HasPrefix(base, "Zapf") {
		return standardEncoding()
	}
	return standardEncoding()
}

func encodingByName(name string) *[256]rune {
	switch name {
	case "WinAnsiEncoding":
		return winAnsiEncoding()
	case "StandardEncoding":
		return standardEncoding()
	case "MacRomanEncoding":
		return standardEncoding()
	}
	return nil
}

func standardEncoding() *[256]rune {
	enc := &[256]rune{}
	for i := 32; i < 127; i++ {
		enc[i] = rune(i)
	}
	enc[0x27] = '\u2019'
	enc[0x60] = '\u2018'
	for i := 0xA0; i < 256; i++ {
		enc[i] = rune(i)
	}
	return enc
}

func winAnsiEncoding() *[256]rune {
	enc := &[256]rune{}
	for i := 32; i < 127; i++ {
		enc[i] = rune(i)
	}
	for i := 0xA0; i < 256; i++ {
		enc[i] = rune(i)
	}
	special := map[byte]rune{
		0x80: '\u20AC', 0x82: '\u201A', 0x83: '\u0192', 0x84: '\u201E',
		0x85: '\u2026', 0x86: '\u2020', 0x87: '\u2021', 0x88: '\u02C6',
		0x89: '\u2030', 0x8A: '\u0160', 0x8B: '\u2039', 0x8C: '\u0152',
		0x8E: '\u017D', 0x91: '\u2018', 0x92: '\u2019', 0x93: '\u201C',
		0x94: '\u201D', 0x95: '\u2022', 0x96: '\u2013', 0x97: '\u2014',
		0x98: '\u02DC', 0x99: '\u2122', 0x9A: '\u0161', 0x9B: '\u203A',
		0x9C: '\u0153', 0x9E: '\u017E', 0x9F: '\u0178',
	}
	for k, v := range special {
		enc[k] = v
	}
	return enc
}

var glyphNames = map[string]rune{
	"space": ' ', "exclam": '!', "quotedbl": '"', "numbersign": '#',
	"dollar": '$', "percent": '%', "ampersand": '&', "quotesingle": '\'',
	"parenleft": '(', "parenright": ')', "asterisk": '*', "plus": '+',
	"comma": ',', "hyphen": '-', "period": '.', "slash": '/',
	"zero": '0', "one": '1', "two": '2', "three": '3', "four": '4',
	"five": '5', "six": '6', "seven": '7', "eight": '8', "nine": '9',
	"colon": ':', "semicolon": ';', "less": '<', "equal": '=', "greater": '>',
	"question": '?', "at": '@', "bracketleft": '[', "backslash": '\\',
	"bracketright": ']', "asciicircum": '^', "underscore": '_', "grave": '`',
	"braceleft": '{', "bar": '|', "braceright": '}', "asciitilde": '~',
	"quoteleft": '\u2018', "quoteright": '\u2019', "quotedblleft": '\u201C',
	"quotedblright": '\u201D', "endash": '\u2013', "emdash": '\u2014',
	"bullet": '\u2022', "ellipsis": '\u2026', "euro": '\u20AC', "Euro": '\u20AC',
	"copyright": '\u00A9', "registered": '\u00AE', "trademark": '\u2122',
	"degree": '\u00B0', "plusminus": '\u00B1', "multiply": '\u00D7',
	"divide": '\u00F7', "fi": '\uFB01', "fl": '\uFB02',
	"dagger": '\u2020', "daggerdbl": '\u2021', "section": '\u00A7',
	"paragraph": '\u00B6', "germandbls": '\u00DF', "ae": '\u00E6',
	"AE": '\u00C6', "oe": '\u0153', "OE": '\u0152', "oslash": '\u00F8',
	"Oslash": '\u00D8', "lslash": '\u0142', "Lslash": '\u0141',
	"florin": '\u0192', "circumflex": '\u02C6', "tilde": '\u02DC',
	"breve": '\u02D8', "dotaccent": '\u02D9', "ring": '\u02DA',
	"ogonek": '\u02DB', "hungarumlaut": '\u02DD', "caron": '\u02C7',
	"perthousand": '\u2030', "guillemotleft": '\u00AB', "guillemotright": '\u00BB',
	"guilsinglleft": '\u2039', "guilsinglright": '\u203A',
	"fraction": '\u2044', "currency": '\u00A4', "brokenbar": '\u00A6',
	"mu": '\u00B5', "ordfeminine": '\u00AA', "ordmasculine": '\u00BA',
	"onehalf": '\u00BD', "onequarter": '\u00BC', "threequarters": '\u00BE',
	"onesuperior": '\u00B9', "twosuperior": '\u00B2', "threesuperior": '\u00B3',
	"logicalnot": '\u00AC', "nbspace": '\u00A0', "minus": '\u2212',
	"periodcentered": '\u00B7', "cedilla": '\u00B8', "macron": '\u00AF',
	"acute": '\u00B4', "dieresis": '\u00A8', "notequal": '\u2260',
	"lessequal": '\u2264', "greaterequal": '\u2265', "infinity": '\u221E',
	"summation": '\u2211', "product": '\u220F', "pi": '\u03C0',
	"integral": '\u222B', "radical": '\u221A', "approxequal": '\u2248',
	"Delta": '\u0394', "Omega": '\u03A9', "lozenge": '\u25CA',
}

func GlyphNameRune(name string) (rune, bool) {
	if r, ok := glyphNames[name]; ok {
		return r, true
	}
	if len(name) == 1 {
		return rune(name[0]), true
	}
	if strings.HasPrefix(name, "uni") && len(name) >= 7 {
		if v, err := strconv.ParseUint(name[3:7], 16, 32); err == nil {
			return rune(v), true
		}
	}
	if strings.HasPrefix(name, "u") && len(name) >= 5 && len(name) <= 7 {
		if v, err := strconv.ParseUint(name[1:], 16, 32); err == nil {
			return rune(v), true
		}
	}
	return 0, false
}

// Describe is used in tests and diagnostics.
func (f *Font) Describe() string {
	if f == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s/%s", f.Info.BaseFont, f.Type)
}
