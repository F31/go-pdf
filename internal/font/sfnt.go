package font

import (
	"context"
	"encoding/binary"
	"math"
	"sort"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/diag"
	"github.com/F31/go-pdf/internal/syntax"
)

type fontProgram struct {
	face     *sfnt.Font
	upem     int
	avgWidth int
}

func (f *Font) loadFontProgram(ctx context.Context, file *syntax.File, fd syntax.Dict, info *displaylist.FontInfo, dc *diag.Collector) {
	if file == nil {
		return
	}
	for _, key := range []string{"FontFile2", "FontFile3"} {
		obj := fd.Get(key)
		if obj == nil {
			continue
		}
		if ref, ok := obj.(syntax.Ref); ok {
			v, err := file.Resolve(ctx, ref)
			if err != nil {
				dc.Addf(diag.Warn, "FNT-SFNT-PARSE", "cannot resolve font program", -1, 0, -1)
				continue
			}
			obj = v
		}
		stm, ok := obj.(*syntax.Stream)
		if !ok {
			continue
		}
		if key == "FontFile3" {
			sub, _ := stm.Dict.NameValue("Subtype")
			if sub != "" && sub != "OpenType" {
				dc.Addf(diag.Degraded, "FNT-SFNT-PROGRAM", "font program format does not provide sfnt metrics", -1, 0, -1)
				continue
			}
		}
		data, err := file.DecodeStream(ctx, stm)
		if err != nil {
			dc.Addf(diag.Warn, "FNT-SFNT-PARSE", "cannot decode font program", -1, 0, -1)
			continue
		}
		face, err := sfnt.Parse(data)
		if err != nil {
			if repaired, changed := normalizeSFNTTables(data); changed {
				if face, err = sfnt.Parse(repaired); err == nil {
					dc.Addf(diag.Info, "FNT-SFNT-REPAIR", "normalized font tables for sfnt parsing", -1, 0, -1)
				}
				data = repaired
			}
		}
		if err != nil {
			if repaired, changed := repairMetricTables(data); changed {
				if face, err = sfnt.Parse(repaired); err == nil {
					dc.Addf(diag.Degraded, "FNT-SFNT-TABLES", "synthesized cmap/post tables for sfnt metrics", -1, 0, -1)
				}
				data = repaired
			}
		}
		if err != nil {
			dc.Addf(diag.Warn, "FNT-SFNT-PARSE", "font program is not a supported sfnt font", -1, 0, -1)
			continue
		}
		upem := int(face.UnitsPerEm())
		if upem <= 0 || face.NumGlyphs() <= 0 {
			dc.Addf(diag.Warn, "FNT-SFNT-PARSE", "font program has invalid metrics", -1, 0, -1)
			continue
		}
		avgWidth, _ := os2AvgWidth(data)
		f.program = &fontProgram{face: face, upem: upem, avgWidth: avgWidth}
		info.Embedded = true
		return
	}
}

type sfntRecord struct {
	tag      uint32
	checksum uint32
	offset   uint32
	length   uint32
}

func readSFNTRecords(data []byte) ([]sfntRecord, bool) {
	if len(data) < 12 {
		return nil, false
	}
	magic := binary.BigEndian.Uint32(data[:4])
	if magic != 0x00010000 && magic != 0x4F54544F && magic != 0x74727565 {
		return nil, false
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	if numTables <= 0 || 12+16*numTables > len(data) {
		return nil, false
	}
	out := make([]sfntRecord, numTables)
	for i := 0; i < numTables; i++ {
		rec := data[12+16*i : 28+16*i]
		r := sfntRecord{
			tag:      binary.BigEndian.Uint32(rec[0:4]),
			checksum: binary.BigEndian.Uint32(rec[4:8]),
			offset:   binary.BigEndian.Uint32(rec[8:12]),
			length:   binary.BigEndian.Uint32(rec[12:16]),
		}
		if uint64(r.offset)+uint64(r.length) > uint64(len(data)) {
			return nil, false
		}
		out[i] = r
	}
	return out, true
}

// normalizeSFNTTables rebuilds an SFNT with sorted table records and 4-byte
// aligned table offsets. Some PDF producers embed otherwise-valid TrueType
// subsets with unaligned or unsorted tables, which x/image/font/sfnt rejects.
func normalizeSFNTTables(data []byte) ([]byte, bool) {
	records, ok := readSFNTRecords(data)
	if !ok {
		return nil, false
	}
	changed := false
	for i, r := range records {
		if r.offset%4 != 0 {
			changed = true
		}
		if i > 0 && records[i-1].tag > r.tag {
			changed = true
		}
	}
	if !changed {
		return data, false
	}
	return rebuildSFNT(records, data, nil), true
}

// repairMetricTables rebuilds an SFNT with minimal cmap and post tables.
// Embedded subsets often omit or corrupt these tables while retaining the
// head/hhea/hmtx metrics needed for width compensation.
func repairMetricTables(data []byte) ([]byte, bool) {
	records, ok := readSFNTRecords(data)
	if !ok {
		return nil, false
	}
	hasCmap, hasPost := false, false
	for _, r := range records {
		switch r.tag {
		case 0x636d6170: // "cmap"
			hasCmap = true
		case 0x706f7374: // "post"
			hasPost = true
		}
	}
	if !hasCmap {
		records = append(records, sfntRecord{tag: 0x636d6170, length: uint32(len(emptyCMap))})
	}
	if !hasPost {
		records = append(records, sfntRecord{tag: 0x706f7374, length: uint32(len(emptyPost))})
	}
	return rebuildSFNT(records, data, map[uint32][]byte{
		0x636d6170: emptyCMap,
		0x706f7374: emptyPost,
	}), true
}

func rebuildSFNT(records []sfntRecord, data []byte, replace map[uint32][]byte) []byte {
	records = append([]sfntRecord(nil), records...)
	sort.Slice(records, func(i, j int) bool { return records[i].tag < records[j].tag })
	headerLen := 12 + 16*len(records)
	offsets := make([]uint32, len(records))
	lengths := make([]uint32, len(records))
	pos := uint32(headerLen)
	for i, r := range records {
		length := r.length
		if repl, ok := replace[r.tag]; ok {
			length = uint32(len(repl))
		}
		pos = (pos + 3) &^ 3
		offsets[i] = pos
		lengths[i] = length
		pos += length
	}
	out := make([]byte, pos)
	copy(out[:12], data[:12])
	numTables := len(records)
	binary.BigEndian.PutUint16(out[4:6], uint16(numTables))
	pow2 := 1
	selector := 0
	for pow2*2 <= numTables {
		pow2 *= 2
		selector++
	}
	searchRange := pow2 * 16
	binary.BigEndian.PutUint16(out[6:8], uint16(searchRange))
	binary.BigEndian.PutUint16(out[8:10], uint16(selector))
	binary.BigEndian.PutUint16(out[10:12], uint16(numTables*16-searchRange))
	for i, r := range records {
		rec := out[12+16*i : 28+16*i]
		binary.BigEndian.PutUint32(rec[0:4], r.tag)
		binary.BigEndian.PutUint32(rec[4:8], r.checksum)
		binary.BigEndian.PutUint32(rec[8:12], offsets[i])
		binary.BigEndian.PutUint32(rec[12:16], lengths[i])
		if repl, ok := replace[r.tag]; ok {
			copy(out[offsets[i]:], repl)
			continue
		}
		copy(out[offsets[i]:offsets[i]+r.length], data[r.offset:r.offset+r.length])
	}
	return out
}

var emptyPost = []byte{0, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

var emptyCMap = []byte{
	0, 0, 0, 1, // version, numTables
	0, 3, 0, 1, 0, 0, 0, 12, // Windows Unicode BMP subtable
	0, 4, 0, 24, // format 4, length 24
	0, 0, // language
	0, 2, 0, 2, 0, 0, 0, 0, // segCountX2, searchRange, entrySelector, rangeShift
	0xFF, 0xFF, 0, 0, // endCode, reservedPad
	0xFF, 0xFF, 0, 1, 0, 0, // startCode, idDelta, idRangeOffset
}

func os2AvgWidth(data []byte) (int, bool) {
	records, ok := readSFNTRecords(data)
	if !ok {
		return 0, false
	}
	for _, r := range records {
		if r.tag == 0x4F532F32 && r.length >= 4 { // "OS/2"
			v := int(int16(binary.BigEndian.Uint16(data[r.offset+2 : r.offset+4])))
			if v > 0 {
				return v, true
			}
		}
	}
	return 0, false
}

func (f *Font) loadCIDToGID(ctx context.Context, file *syntax.File, cid syntax.Dict, info *displaylist.FontInfo, dc *diag.Collector) {
	obj := cid.Get("CIDToGIDMap")
	if obj == nil {
		if info.Kind == "CIDFontType2" {
			f.cidToGIDIdentity = true
		}
		return
	}
	if name, ok := obj.(syntax.Name); ok {
		if name == "Identity" {
			f.cidToGIDIdentity = true
		} else {
			dc.Addf(diag.Degraded, "FNT-CIDTOGID", "unsupported CIDToGIDMap", -1, 0, -1)
		}
		return
	}
	if file == nil {
		return
	}
	if ref, ok := obj.(syntax.Ref); ok {
		v, err := file.Resolve(ctx, ref)
		if err != nil {
			dc.Addf(diag.Warn, "FNT-CIDTOGID", "cannot resolve CIDToGIDMap", -1, 0, -1)
			return
		}
		obj = v
	}
	stm, ok := obj.(*syntax.Stream)
	if !ok {
		dc.Addf(diag.Warn, "FNT-CIDTOGID", "invalid CIDToGIDMap", -1, 0, -1)
		return
	}
	data, err := file.DecodeStream(ctx, stm)
	if err != nil {
		dc.Addf(diag.Warn, "FNT-CIDTOGID", "cannot decode CIDToGIDMap", -1, 0, -1)
		return
	}
	if len(data)%2 != 0 {
		dc.Addf(diag.Warn, "FNT-CIDTOGID", "CIDToGIDMap has an odd length", -1, 0, -1)
		return
	}
	f.cidToGID = data
}

func (f *Font) fillMetrics(info *displaylist.FontInfo, dc *diag.Collector) {
	if f.program == nil {
		info.Metrics.Substituted = true
		return
	}
	p := f.program
	var buf sfnt.Buffer
	ppem := fixed.I(p.upem)
	m, err := p.face.Metrics(&buf, ppem, xfont.HintingNone)
	if err != nil {
		dc.Addf(diag.Warn, "FNT-METRICS", "cannot read sfnt metrics", -1, 0, -1)
		info.Metrics.Substituted = true
		return
	}
	ascent := int(m.Ascent) / 64
	descent := int(m.Descent) / 64
	if descent < 0 {
		descent = -descent
	}
	metrics := displaylist.FontMetrics{
		UnitsPerEm: p.upem,
		Ascent:     ascent,
		Descent:    descent,
		CapHeight:  int(m.CapHeight) / 64,
		XHeight:    int(m.XHeight) / 64,
	}
	metrics.Advances = f.fontAdvances(&buf, p, ppem)
	metrics.AvgWidth = p.avgWidth
	if metrics.AvgWidth <= 0 && len(metrics.Advances) > 0 {
		sum := 0
		for _, v := range metrics.Advances {
			sum += v
		}
		perMille := float64(sum) / float64(len(metrics.Advances))
		metrics.AvgWidth = int(math.Round(perMille * float64(p.upem) / 1000))
	}
	info.Metrics = metrics
	if info.Ascent == 0 && info.Descent == 0 {
		info.Ascent = float64(metrics.Ascent) * 1000 / float64(p.upem)
		info.Descent = -float64(metrics.Descent) * 1000 / float64(p.upem)
	}
}

func (f *Font) fontAdvances(buf *sfnt.Buffer, p *fontProgram, ppem fixed.Int26_6) map[uint32]int {
	if f.isCID {
		return f.cidAdvances(buf, p, ppem)
	}
	out := map[uint32]int{}
	for code := 0; code < 256; code++ {
		text, ok := f.Decode(byte(code))
		if !ok {
			continue
		}
		r := firstRune(text)
		if r == 0 {
			continue
		}
		gid, err := p.face.GlyphIndex(buf, r)
		if err != nil || gid == 0 {
			continue
		}
		if adv, ok := advancePerMille(buf, p, ppem, gid); ok {
			out[uint32(code)] = adv
		}
	}
	return out
}

func (f *Font) cidAdvances(buf *sfnt.Buffer, p *fontProgram, ppem fixed.Int26_6) map[uint32]int {
	out := map[uint32]int{}
	if f.toUnicode != nil {
		for code, text := range f.toUnicode.Map {
			cid, ok := f.cidForValue(code)
			if !ok {
				continue
			}
			if _, seen := out[cid]; seen {
				continue
			}
			gid, ok := f.gidForCID(cid)
			if !ok {
				r := firstRune(text)
				if r == 0 {
					continue
				}
				var err error
				gid, err = p.face.GlyphIndex(buf, r)
				if err != nil || gid == 0 {
					continue
				}
			}
			if adv, ok := advancePerMille(buf, p, ppem, gid); ok {
				out[cid] = adv
			}
		}
	}
	for cid := range f.cidWidths {
		if _, seen := out[cid]; seen {
			continue
		}
		gid, ok := f.gidForCID(cid)
		if !ok {
			continue
		}
		if adv, ok := advancePerMille(buf, p, ppem, gid); ok {
			out[cid] = adv
		}
	}
	return out
}

func (f *Font) cidForValue(code uint32) (uint32, bool) {
	if f.identity {
		return code, true
	}
	if f.predefined == nil {
		return 0, false
	}
	for n := 1; n <= f.predefined.maxBytes; n++ {
		if n < 4 && code >= 1<<(8*n) {
			continue
		}
		if f.predefined.matches(n, code) {
			return f.predefined.cidFor(n, code), true
		}
	}
	return 0, false
}

func (f *Font) gidForCID(cid uint32) (sfnt.GlyphIndex, bool) {
	if cid > 0xFFFF {
		return 0, false
	}
	if f.cidToGIDIdentity {
		return sfnt.GlyphIndex(cid), true
	}
	off := uint64(cid) * 2
	if off+2 > uint64(len(f.cidToGID)) {
		return 0, false
	}
	gid := uint16(f.cidToGID[off])<<8 | uint16(f.cidToGID[off+1])
	return sfnt.GlyphIndex(gid), true
}

func advancePerMille(buf *sfnt.Buffer, p *fontProgram, ppem fixed.Int26_6, gid sfnt.GlyphIndex) (int, bool) {
	adv, err := p.face.GlyphAdvance(buf, gid, ppem, xfont.HintingNone)
	if err != nil {
		return 0, false
	}
	return int(math.Round(float64(adv) / 64 * 1000 / float64(p.upem))), true
}

func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}
