// Package image implements PDF image XObjects and inline image samples.
package image

import (
	"bytes"
	"context"
	"errors"
	stdimage "image"
	stdcolor "image/color"
	"image/jpeg"
	"io"
	"math"
	"strings"

	"golang.org/x/image/ccitt"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/budget"
	"github.com/F31/go-pdf/internal/diag"
	"github.com/F31/go-pdf/internal/errs"
	"github.com/F31/go-pdf/internal/filter"
	"github.com/F31/go-pdf/internal/syntax"
)

const maxMaskDepth = 4

// Ref implements displaylist.ImageRef for an XObject or inline image.
type Ref struct {
	info    displaylist.ImageInfo
	file    *syntax.File
	stream  *syntax.Stream
	dict    syntax.Dict
	raw     []byte
	tracker *budget.Tracker
	diag    *diag.Collector
}

// NewXObject builds an image reference from an image XObject stream.
func NewXObject(ctx context.Context, file *syntax.File, stm *syntax.Stream, tracker *budget.Tracker, dc *diag.Collector) (*Ref, *displaylist.MaskRef, error) {
	return newRef(ctx, file, stm, stm.Raw, tracker, dc, false, 0)
}

// NewInline builds an image reference from an inline image dictionary and data.
func NewInline(dict syntax.Dict, data []byte, tracker *budget.Tracker, dc *diag.Collector) (*Ref, error) {
	info, err := imageInfo(dict, true, nil)
	if err != nil {
		return nil, err
	}
	return &Ref{info: info, dict: dict, raw: append([]byte(nil), data...), tracker: tracker, diag: dc}, nil
}

func newRef(ctx context.Context, file *syntax.File, stm *syntax.Stream, raw []byte, tracker *budget.Tracker, dc *diag.Collector, inline bool, depth int) (*Ref, *displaylist.MaskRef, error) {
	info, err := imageInfo(stm.Dict, inline, file)
	if err != nil {
		return nil, nil, err
	}
	r := &Ref{info: info, file: file, stream: stm, dict: stm.Dict, raw: append([]byte(nil), raw...), tracker: tracker, diag: dc}
	if depth >= maxMaskDepth {
		diagf(dc, "IMG-MASK-DEPTH", "image mask nesting limit reached")
		return r, nil, nil
	}
	mask, err := imageMask(ctx, file, stm.Dict, tracker, dc, depth)
	if err != nil {
		diagf(dc, "IMG-MASK", "cannot read image mask")
	}
	return r, mask, nil
}

func imageMask(ctx context.Context, file *syntax.File, dict syntax.Dict, tracker *budget.Tracker, dc *diag.Collector, depth int) (*displaylist.MaskRef, error) {
	var smask, mask syntax.Object
	if dict.Has("SMask") {
		smask = dict.Get("SMask")
	}
	if dict.Has("Mask") {
		mask = dict.Get("Mask")
	}
	if smask != nil {
		if mask != nil {
			diagf(dc, "IMG-MASK-CONFLICT", "both SMask and Mask present; using SMask")
		}
		ref, ok := imageStream(ctx, file, smask)
		if !ok {
			return nil, errors.New("mask is not an image stream")
		}
		mr, _, err := newRef(ctx, file, ref, ref.Raw, tracker, dc, false, depth+1)
		if err != nil {
			return nil, err
		}
		return &displaylist.MaskRef{Image: mr}, nil
	}
	if mask == nil {
		return nil, nil
	}
	if file != nil {
		if v, err := file.Resolve(ctx, mask); err == nil {
			mask = v
		}
	}
	if arr, ok := mask.(syntax.Array); ok {
		key := make([]float64, 0, len(arr))
		for _, v := range arr {
			n, ok := number(v)
			if !ok {
				return nil, errors.New("invalid color key mask")
			}
			key = append(key, n)
		}
		return &displaylist.MaskRef{IsColorKey: true, ColorKey: key}, nil
	}
	ref, ok := imageStream(ctx, file, mask)
	if !ok {
		return nil, errors.New("mask is not an image stream")
	}
	mr, _, err := newRef(ctx, file, ref, ref.Raw, tracker, dc, false, depth+1)
	if err != nil {
		return nil, err
	}
	return &displaylist.MaskRef{Image: mr}, nil
}

func imageStream(ctx context.Context, file *syntax.File, obj syntax.Object) (*syntax.Stream, bool) {
	if file != nil {
		if v, err := file.Resolve(ctx, obj); err == nil {
			obj = v
		} else {
			return nil, false
		}
	}
	stm, ok := obj.(*syntax.Stream)
	return stm, ok
}

func imageInfo(dict syntax.Dict, inline bool, file *syntax.File) (displaylist.ImageInfo, error) {
	w, ok := intValue(dict, "Width", "W")
	if !ok || w <= 0 {
		return displaylist.ImageInfo{}, errs.Wrap(errs.ErrCorrupt, "IMG-DIMENSIONS", "image width", -1, 0, -1)
	}
	h, ok := intValue(dict, "Height", "H")
	if !ok || h <= 0 {
		return displaylist.ImageInfo{}, errs.Wrap(errs.ErrCorrupt, "IMG-DIMENSIONS", "image height", -1, 0, -1)
	}
	bpc, _ := intValue(dict, "BitsPerComponent", "BPC")
	if bpc <= 0 {
		bpc = 8
	}
	mask, _ := boolValue(dict, "ImageMask", "IM")
	if mask {
		bpc = 1
	}
	cs := colorSpaceName(getAny(dict, "ColorSpace", "CS"), inline)
	if mask {
		cs = "ImageMask"
	}
	enc := "Raw"
	if filters := filterNames(dict, file); len(filters) > 0 {
		enc = filters[len(filters)-1]
	}
	decode, _ := numberArray(getAny(dict, "Decode", "D"), file)
	interp, _ := boolValue(dict, "Interpolate", "I")
	return displaylist.ImageInfo{
		Width: w, Height: h, BitsPerComponent: bpc, ColorSpace: cs,
		Encoding: displaylist.ImageEncoding(enc), Decode: decode,
		Interpolate: interp, Inline: inline,
	}, nil
}

func colorSpaceName(o syntax.Object, inline bool) string {
	if name, ok := o.(syntax.Name); ok {
		s := string(name)
		if inline {
			switch s {
			case "G":
				return "DeviceGray"
			case "RGB":
				return "DeviceRGB"
			case "CMYK":
				return "DeviceCMYK"
			case "I":
				return "Indexed"
			}
		}
		return s
	}
	if arr, ok := o.(syntax.Array); ok && len(arr) > 0 {
		if name, ok := arr[0].(syntax.Name); ok {
			return string(name)
		}
	}
	return ""
}

// Info returns the image metadata.
func (r *Ref) Info() displaylist.ImageInfo { return r.info }

// Raw returns the encoded image bytes.
func (r *Ref) Raw() ([]byte, error) {
	if r == nil {
		return nil, errs.Wrap(errs.ErrUnsupported, "IMG-NIL", "image is nil", -1, 0, -1)
	}
	return append([]byte(nil), r.raw...), nil
}

// Decode decodes the image samples into an image.Image.
func (r *Ref) Decode(ctx context.Context) (stdimage.Image, error) {
	if r == nil {
		return nil, errs.Wrap(errs.ErrUnsupported, "IMG-NIL", "image is nil", -1, 0, -1)
	}
	if r.info.Width <= 0 || r.info.Height <= 0 {
		return nil, errs.Wrap(errs.ErrCorrupt, "IMG-DIMENSIONS", "decode image", -1, 0, -1)
	}
	if r.tracker != nil {
		pixels := int64(r.info.Width) * int64(r.info.Height)
		if max := r.tracker.Budget().MaxImagePixels; max > 0 && pixels > max {
			return nil, errs.Wrap(errs.ErrBudget, "BUD-IMAGE-PIXELS", "decode image", -1, 0, -1)
		}
	}
	if d := externalDecoder(r.info.Encoding); d != nil {
		img, err := d.Decode(ctx, append([]byte(nil), r.raw...), r.info)
		if err != nil {
			diagf(r.diag, "IMG-EXTERNAL", "external image decoder failed")
			return nil, err
		}
		return img, nil
	}
	if isPassthroughEncoding(r.info.Encoding) {
		return nil, r.fail("IMG-NO-DECODER", "no decoder for image encoding "+string(r.info.Encoding), nil, errs.ErrUnsupported)
	}
	if isCCITT(string(r.info.Encoding)) {
		return r.decodeCCITT()
	}
	data, err := r.decodedBytes(ctx)
	if err != nil {
		return nil, err
	}
	if isDCT(string(r.info.Encoding)) {
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, r.fail("IMG-DECODE", "decode JPEG image", err, errs.ErrCorrupt)
		}
		return r.applyDCTDecode(img), nil
	}
	return r.decodeSamples(data)
}

func (r *Ref) decodedBytes(ctx context.Context) ([]byte, error) {
	filters := filterNames(r.dict, r.file)
	parms := decodeParms(r.dict, r.file)
	if len(filters) > 0 && isDCT(filters[len(filters)-1]) {
		data := r.raw
		if len(filters) > 1 {
			var err error
			data, err = applyFilters(ctx, data, filters[:len(filters)-1], padParms(parms, len(filters)-1), r.tracker)
			if err != nil {
				return nil, r.filterError(err)
			}
		}
		return data, nil
	}
	if r.file != nil && r.stream != nil {
		data, err := r.file.DecodeStream(ctx, r.stream)
		if err != nil {
			return nil, r.filterError(err)
		}
		return data, nil
	}
	data, err := applyFilters(ctx, r.raw, filters, parms, r.tracker)
	if err != nil {
		return nil, r.filterError(err)
	}
	return data, nil
}

func (r *Ref) filterError(err error) error {
	if errors.Is(err, errs.ErrBudget) || errors.Is(err, errs.ErrCanceled) {
		return err
	}
	if errors.Is(err, errs.ErrUnsupported) {
		return r.fail("IMG-NO-DECODER", "decode image filter", err, errs.ErrUnsupported)
	}
	return r.fail("IMG-DECODE", "decode image filter", err, errs.ErrCorrupt)
}

func (r *Ref) fail(code, msg string, cause error, kind error) error {
	diagf(r.diag, code, msg)
	return errs.Wrap(kind, code, msg, -1, 0, -1)
}

func (r *Ref) decodeSamples(data []byte) (stdimage.Image, error) {
	w, h, bpc := r.info.Width, r.info.Height, r.info.BitsPerComponent
	cs := r.info.ColorSpace
	ncomp := 1
	switch cs {
	case "DeviceRGB":
		ncomp = 3
	case "DeviceCMYK":
		ncomp = 4
	case "DeviceGray", "ImageMask", "":
		ncomp = 1
	default:
		return nil, r.fail("IMG-COLORSPACE", "unsupported image color space "+cs, nil, errs.ErrUnsupported)
	}
	if bpc != 1 && bpc != 2 && bpc != 4 && bpc != 8 && bpc != 16 {
		return nil, r.fail("IMG-BPC", "unsupported image bits per component", nil, errs.ErrUnsupported)
	}
	rowBytes := (w*ncomp*bpc + 7) / 8
	if rowBytes <= 0 || len(data) < rowBytes*h {
		return nil, r.fail("IMG-DATA", "image data is truncated", nil, errs.ErrCorrupt)
	}
	maxv := uint32((1 << bpc) - 1)
	if bpc == 16 {
		maxv = 65535
	}
	decode := r.info.Decode
	sample := func(row []byte, index int) uint32 {
		bit := index * bpc
		if bpc == 8 {
			return uint32(row[bit/8])
		}
		if bpc == 16 {
			i := bit / 8
			return uint32(row[i])<<8 | uint32(row[i+1])
		}
		byteIdx := bit / 8
		shift := 8 - bpc - bit%8
		return uint32(row[byteIdx]>>shift) & maxv
	}
	norm := func(v uint32, comp int) float64 {
		s := float64(v) / float64(maxv)
		if len(decode) >= (comp+1)*2 {
			lo, hi := decode[comp*2], decode[comp*2+1]
			s = lo + s*(hi-lo)
		}
		if s < 0 {
			s = 0
		}
		if s > 1 {
			s = 1
		}
		return s
	}
	switch {
	case cs == "ImageMask":
		out := stdimage.NewAlpha(stdimage.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			row := data[y*rowBytes:]
			for x := 0; x < w; x++ {
				v := norm(sample(row, x), 0)
				out.SetAlpha(x, y, stdcolor.Alpha{A: uint8(math.Round((1 - v) * 255))})
			}
		}
		return out, nil
	case ncomp == 1:
		out := stdimage.NewGray(stdimage.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			row := data[y*rowBytes:]
			for x := 0; x < w; x++ {
				out.SetGray(x, y, stdcolor.Gray{Y: uint8(math.Round(norm(sample(row, x), 0) * 255))})
			}
		}
		return out, nil
	default:
		out := stdimage.NewRGBA(stdimage.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			row := data[y*rowBytes:]
			for x := 0; x < w; x++ {
				var c stdcolor.RGBA
				if ncomp == 3 {
					c = stdcolor.RGBA{
						R: uint8(math.Round(norm(sample(row, x*3), 0) * 255)),
						G: uint8(math.Round(norm(sample(row, x*3+1), 1) * 255)),
						B: uint8(math.Round(norm(sample(row, x*3+2), 2) * 255)),
						A: 255,
					}
				} else {
					r8 := uint8(math.Round(norm(sample(row, x*4), 0) * 255))
					g8 := uint8(math.Round(norm(sample(row, x*4+1), 1) * 255))
					b8 := uint8(math.Round(norm(sample(row, x*4+2), 2) * 255))
					k8 := uint8(math.Round(norm(sample(row, x*4+3), 3) * 255))
					cr, cg, cb := stdcolor.CMYKToRGB(r8, g8, b8, k8)
					c = stdcolor.RGBA{R: cr, G: cg, B: cb, A: 255}
				}
				out.SetRGBA(x, y, c)
			}
		}
		return out, nil
	}
}

func (r *Ref) applyDCTDecode(in stdimage.Image) stdimage.Image {
	if len(r.info.Decode) == 0 {
		if cm, ok := in.(*stdimage.CMYK); ok {
			return cmykToRGBA(cm)
		}
		return in
	}
	comp := func(v uint8, i int) uint8 {
		if len(r.info.Decode) < (i+1)*2 {
			return v
		}
		s := float64(v) / 255
		lo, hi := r.info.Decode[i*2], r.info.Decode[i*2+1]
		s = lo + s*(hi-lo)
		if s < 0 {
			s = 0
		}
		if s > 1 {
			s = 1
		}
		return uint8(math.Round(s * 255))
	}
	b := in.Bounds()
	if r.info.ColorSpace == "DeviceGray" {
		out := stdimage.NewGray(b)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				g := stdcolor.GrayModel.Convert(in.At(x, y)).(stdcolor.Gray)
				out.SetGray(x, y, stdcolor.Gray{Y: comp(g.Y, 0)})
			}
		}
		return out
	}
	if r.info.ColorSpace == "DeviceCMYK" {
		out := stdimage.NewRGBA(b)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := stdcolor.CMYKModel.Convert(in.At(x, y)).(stdcolor.CMYK)
				rr, gg, bb := stdcolor.CMYKToRGB(comp(c.C, 0), comp(c.M, 1), comp(c.Y, 2), comp(c.K, 3))
				out.SetRGBA(x, y, stdcolor.RGBA{R: rr, G: gg, B: bb, A: 255})
			}
		}
		return out
	}
	out := stdimage.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := stdcolor.RGBAModel.Convert(in.At(x, y)).(stdcolor.RGBA)
			out.SetRGBA(x, y, stdcolor.RGBA{R: comp(c.R, 0), G: comp(c.G, 1), B: comp(c.B, 2), A: c.A})
		}
	}
	return out
}

func (r *Ref) decodeCCITT() (stdimage.Image, error) {
	params := firstParms(decodeParms(r.dict, r.file))
	k := params.Int("k", 0)
	if k > 0 {
		return nil, r.fail("IMG-CCITT-2D", "CCITT Group 3 2-D decoding is unsupported", nil, errs.ErrUnsupported)
	}
	width, height := r.info.Width, r.info.Height
	if v := params.Int("columns", 0); v > 0 {
		width = v
	}
	if v := params.Int("rows", 0); v > 0 {
		height = v
	}
	sub := ccitt.Group3
	if k < 0 {
		sub = ccitt.Group4
	}
	opts := &ccitt.Options{Align: params.Bool("encodedbytealign", false)}
	cr := ccitt.NewReader(bytes.NewReader(r.raw), ccitt.MSB, sub, width, height, opts)
	bits, err := io.ReadAll(cr)
	if err != nil {
		return nil, r.fail("IMG-CCITT", "decode CCITT image", err, errs.ErrCorrupt)
	}
	rowBytes := (width + 7) / 8
	if len(bits) < rowBytes*height {
		return nil, r.fail("IMG-CCITT", "CCITT image data is truncated", nil, errs.ErrCorrupt)
	}
	blackIs1 := params.Bool("blackis1", false)
	mask := r.info.ColorSpace == "ImageMask"
	if mask {
		out := stdimage.NewAlpha(stdimage.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			row := bits[y*rowBytes:]
			for x := 0; x < width; x++ {
				bit := (row[x/8] >> uint(7-x%8)) & 1
				v := float64(bit)
				if blackIs1 {
					v = 1 - v
				}
				v = applyDecode(r.info.Decode, 0, v)
				out.SetAlpha(x, y, stdcolor.Alpha{A: uint8(math.Round((1 - v) * 255))})
			}
		}
		return out, nil
	}
	out := stdimage.NewGray(stdimage.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		row := bits[y*rowBytes:]
		for x := 0; x < width; x++ {
			bit := (row[x/8] >> uint(7-x%8)) & 1
			v := float64(bit)
			if blackIs1 {
				v = 1 - v
			}
			v = applyDecode(r.info.Decode, 0, v)
			out.SetGray(x, y, stdcolor.Gray{Y: uint8(math.Round(v * 255))})
		}
	}
	return out, nil
}

func applyDecode(decode []float64, comp int, v float64) float64 {
	if len(decode) < (comp+1)*2 {
		return v
	}
	lo, hi := decode[comp*2], decode[comp*2+1]
	v = lo + v*(hi-lo)
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func firstParms(in []filter.Params) filter.Params {
	if len(in) > 0 && in[0] != nil {
		return in[0]
	}
	return filter.Params{}
}

func cmykToRGBA(in *stdimage.CMYK) *stdimage.RGBA {
	b := in.Bounds()
	out := stdimage.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := in.CMYKAt(x, y)
			r, g, bl := stdcolor.CMYKToRGB(c.C, c.M, c.Y, c.K)
			out.SetRGBA(x, y, stdcolor.RGBA{R: r, G: g, B: bl, A: 255})
		}
	}
	return out
}

func getAny(d syntax.Dict, names ...string) syntax.Object {
	for _, name := range names {
		if v := d.Get(name); v != nil {
			if _, null := v.(syntax.Null); !null {
				return v
			}
		}
	}
	return syntax.Null{}
}

func intValue(d syntax.Dict, names ...string) (int, bool) {
	for _, name := range names {
		switch v := d.Get(name).(type) {
		case syntax.Int:
			return int(v), true
		case syntax.Real:
			return int(v), true
		}
	}
	return 0, false
}

func boolValue(d syntax.Dict, names ...string) (bool, bool) {
	for _, name := range names {
		if v, ok := d.Get(name).(syntax.Bool); ok {
			return bool(v), true
		}
	}
	return false, false
}

func number(o syntax.Object) (float64, bool) {
	switch v := o.(type) {
	case syntax.Int:
		return float64(v), true
	case syntax.Real:
		return float64(v), true
	}
	return 0, false
}

func numberArray(o syntax.Object, file *syntax.File) ([]float64, bool) {
	if file != nil {
		if v, err := file.Resolve(context.Background(), o); err == nil {
			o = v
		}
	}
	arr, ok := o.(syntax.Array)
	if !ok {
		return nil, false
	}
	out := make([]float64, 0, len(arr))
	for _, v := range arr {
		n, ok := number(v)
		if !ok {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

func filterNames(d syntax.Dict, file *syntax.File) []string {
	o := getAny(d, "Filter", "F")
	if file != nil {
		if v, err := file.Resolve(context.Background(), o); err == nil {
			o = v
		}
	}
	switch v := o.(type) {
	case syntax.Name:
		return []string{string(v)}
	case syntax.Array:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if name, ok := item.(syntax.Name); ok {
				out = append(out, string(name))
			}
		}
		return out
	}
	return nil
}

func decodeParms(d syntax.Dict, file *syntax.File) []filter.Params {
	o := getAny(d, "DecodeParms", "DP")
	if file != nil {
		if v, err := file.Resolve(context.Background(), o); err == nil {
			o = v
		}
	}
	var items []syntax.Object
	switch v := o.(type) {
	case syntax.Dict:
		items = []syntax.Object{v}
	case syntax.Array:
		items = v
	}
	out := make([]filter.Params, len(items))
	for i, item := range items {
		if d, ok := item.(syntax.Dict); ok {
			out[i] = params(d)
		}
	}
	return out
}

func params(d syntax.Dict) filter.Params {
	out := filter.Params{}
	for k, v := range d {
		key := strings.ToLower(string(k))
		switch x := v.(type) {
		case syntax.Int:
			out[key] = int(x)
		case syntax.Real:
			out[key] = float64(x)
		case syntax.Bool:
			out[key] = bool(x)
		case syntax.Name:
			out[key] = string(x)
		case syntax.String:
			out[key] = string(x)
		}
	}
	return out
}

func padParms(in []filter.Params, n int) []filter.Params {
	if len(in) >= n {
		return in[:n]
	}
	out := make([]filter.Params, n)
	copy(out, in)
	return out
}

func applyFilters(ctx context.Context, data []byte, names []string, parms []filter.Params, tracker *budget.Tracker) ([]byte, error) {
	if len(names) == 0 {
		return data, nil
	}
	r, err := filter.DecodeChain(ctx, bytes.NewReader(data), names, parms, tracker, int64(len(data)))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

func isDCT(enc string) bool {
	return strings.EqualFold(enc, "DCTDecode") || strings.EqualFold(enc, "DCT")
}

func isCCITT(enc string) bool {
	return strings.EqualFold(enc, "CCITTFaxDecode") || strings.EqualFold(enc, "CCF")
}

func isPassthroughEncoding(enc displaylist.ImageEncoding) bool {
	s := strings.ToLower(string(enc))
	return s == "jbig2decode" || s == "jpxdecode"
}

func diagf(dc *diag.Collector, code, msg string) {
	if dc != nil {
		dc.Addf(diag.Degraded, code, msg, -1, 0, -1)
	}
}

var _ displaylist.ImageRef = (*Ref)(nil)
