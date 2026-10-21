package font

import (
	"errors"
	"sort"
)

var errEmbeddedCMap = errors.New("font: invalid embedded CMap")

// parseEmbeddedCMap parses a CMap stream used as a Type0 /Encoding. The CMap
// may inherit code spaces, CIDs, and Unicode semantics from a predefined base
// via usecmap.
func parseEmbeddedCMap(data []byte) (*predefinedCMap, error) {
	if len(data) == 0 {
		return nil, errEmbeddedCMap
	}
	ownSpaces := []codeSpace{}
	ownCID := [5]map[uint32]uint32{}
	ownNotdef := [5]map[uint32]uint32{}
	for n := 1; n <= 4; n++ {
		ownCID[n] = map[uint32]uint32{}
		ownNotdef[n] = map[uint32]uint32{}
	}
	var base *predefinedCMap
	wmode, haveWMode := 0, false
	lastName := ""
	l := &cmapLexer{data: data}
	for {
		t, ok, err := l.next()
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		switch t.kind {
		case 's':
			lastName = t.kw
		case 'k':
			switch t.kw {
			case "usecmap":
				base = cmapBaseByName(lastName)
				if base == nil {
					return nil, errEmbeddedCMap
				}
			case "WMode":
				n, ok, err := l.next()
				if err != nil || !ok || n.kind != 'n' {
					return nil, errEmbeddedCMap
				}
				wmode, haveWMode = int(n.num), true
			case "begincodespacerange":
				spaces, err := parseEmbeddedCodeSpace(l)
				if err != nil {
					return nil, err
				}
				ownSpaces = append(ownSpaces, spaces...)
			case "begincidrange":
				if err := parseEmbeddedCIDRange(l, ownCID); err != nil {
					return nil, err
				}
			case "begincidchar":
				if err := parseEmbeddedCIDChar(l, ownCID); err != nil {
					return nil, err
				}
			case "beginnotdefrange":
				if err := parseEmbeddedNotdefRange(l, ownNotdef); err != nil {
					return nil, err
				}
			}
		}
	}

	cm := &predefinedCMap{name: "embedded", kind: unicodeNone}
	cidMaps, notdefMaps := ownCID, ownNotdef
	if base != nil {
		cm.kind = base.kind
		cm.spaces = append([]codeSpace(nil), base.spaces...)
		cm.wmode = base.wmode
		for n := 1; n <= 4; n++ {
			cm.unicode[n] = append([]unicodeRange(nil), base.unicode[n]...)
			mergedCID := make(map[uint32]uint32, len(base.cid[n])+len(ownCID[n]))
			mergedNotdef := make(map[uint32]uint32, len(base.notdef[n])+len(ownNotdef[n]))
			mergeCIDRanges(mergedCID, base.cid[n])
			mergeCIDMap(mergedCID, ownCID[n])
			mergeCIDRanges(mergedNotdef, base.notdef[n])
			mergeCIDMap(mergedNotdef, ownNotdef[n])
			cidMaps[n], notdefMaps[n] = mergedCID, mergedNotdef
		}
	}
	if len(ownSpaces) > 0 {
		cm.spaces = ownSpaces
	}
	if haveWMode {
		cm.wmode = wmode
	}
	if len(cm.spaces) == 0 {
		return nil, errEmbeddedCMap
	}
	for _, s := range cm.spaces {
		if s.Bytes > cm.maxBytes {
			cm.maxBytes = s.Bytes
		}
	}
	for n := 1; n <= 4; n++ {
		cm.cid[n] = compressCIDMap(cidMaps[n])
		cm.notdef[n] = compressCIDMap(notdefMaps[n])
	}
	return cm, nil
}

func parseEmbeddedCodeSpace(l *cmapLexer) ([]codeSpace, error) {
	var out []codeSpace
	for {
		t, ok, err := l.next()
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errEmbeddedCMap
		}
		if t.kind == 'k' && t.kw == "endcodespacerange" {
			return out, nil
		}
		if t.kind != 'h' {
			return nil, errEmbeddedCMap
		}
		hi, ok, err := l.next()
		if err != nil || !ok || hi.kind != 'h' {
			return nil, errEmbeddedCMap
		}
		n := len(t.hex)
		if n == 0 || n > 4 || len(hi.hex) != n {
			return nil, errEmbeddedCMap
		}
		lo, high := codeOf(t.hex), codeOf(hi.hex)
		if high < lo {
			return nil, errEmbeddedCMap
		}
		out = append(out, codeSpace{Bytes: n, Lo: lo, Hi: high})
	}
}

func parseEmbeddedCIDRange(l *cmapLexer, dst [5]map[uint32]uint32) error {
	for {
		t, ok, err := l.next()
		if err != nil {
			return err
		}
		if !ok {
			return errEmbeddedCMap
		}
		if t.kind == 'k' && t.kw == "endcidrange" {
			return nil
		}
		if t.kind != 'h' {
			return errEmbeddedCMap
		}
		hi, ok, err := l.next()
		if err != nil || !ok || hi.kind != 'h' {
			return errEmbeddedCMap
		}
		cid, ok, err := l.next()
		if err != nil || !ok || cid.kind != 'n' {
			return errEmbeddedCMap
		}
		n := len(t.hex)
		if n == 0 || n > 4 || len(hi.hex) != n || cid.num < 0 || uint64(cid.num) > 0xFFFFFFFF {
			return errEmbeddedCMap
		}
		if err := addCIDRange(dst[n], n, codeOf(t.hex), codeOf(hi.hex), uint32(cid.num)); err != nil {
			return err
		}
	}
}

func parseEmbeddedCIDChar(l *cmapLexer, dst [5]map[uint32]uint32) error {
	for {
		t, ok, err := l.next()
		if err != nil {
			return err
		}
		if !ok {
			return errEmbeddedCMap
		}
		if t.kind == 'k' && t.kw == "endcidchar" {
			return nil
		}
		if t.kind != 'h' {
			return errEmbeddedCMap
		}
		cid, ok, err := l.next()
		if err != nil || !ok || cid.kind != 'n' || cid.num < 0 || uint64(cid.num) > 0xFFFFFFFF {
			return errEmbeddedCMap
		}
		n := len(t.hex)
		if n == 0 || n > 4 {
			return errEmbeddedCMap
		}
		if err := addCIDRange(dst[n], n, codeOf(t.hex), codeOf(t.hex), uint32(cid.num)); err != nil {
			return err
		}
	}
}

func parseEmbeddedNotdefRange(l *cmapLexer, dst [5]map[uint32]uint32) error {
	for {
		t, ok, err := l.next()
		if err != nil {
			return err
		}
		if !ok {
			return errEmbeddedCMap
		}
		if t.kind == 'k' && t.kw == "endnotdefrange" {
			return nil
		}
		if t.kind != 'h' {
			return errEmbeddedCMap
		}
		hi, ok, err := l.next()
		if err != nil || !ok || hi.kind != 'h' {
			return errEmbeddedCMap
		}
		cid, ok, err := l.next()
		if err != nil || !ok || cid.kind != 'n' || cid.num < 0 || uint64(cid.num) > 0xFFFFFFFF {
			return errEmbeddedCMap
		}
		n := len(t.hex)
		if n == 0 || n > 4 || len(hi.hex) != n {
			return errEmbeddedCMap
		}
		if err := addCIDRange(dst[n], n, codeOf(t.hex), codeOf(hi.hex), uint32(cid.num)); err != nil {
			return err
		}
	}
}

const maxEmbeddedCIDEntries = 2 << 20

func addCIDRange(dst map[uint32]uint32, n int, lo, hi, cid uint32) error {
	if hi < lo || hi-lo > maxEmbeddedCIDEntries {
		return errEmbeddedCMap
	}
	if uint64(cid)+uint64(hi-lo) > 0xFFFFFFFF {
		return errEmbeddedCMap
	}
	for code := lo; ; code++ {
		dst[code] = cid + code - lo
		if len(dst) > maxEmbeddedCIDEntries {
			return errEmbeddedCMap
		}
		if code == hi {
			break
		}
	}
	return nil
}

func mergeCIDMap(dst, src map[uint32]uint32) {
	for code, cid := range src {
		dst[code] = cid
	}
}

func mergeCIDRanges(dst map[uint32]uint32, ranges []cidRange) {
	for _, r := range ranges {
		for code := r.lo; ; code++ {
			dst[code] = r.cid + code - r.lo
			if code == r.hi {
				break
			}
		}
	}
}

func compressCIDMap(m map[uint32]uint32) []cidRange {
	if len(m) == 0 {
		return nil
	}
	keys := make([]uint32, 0, len(m))
	for code := range m {
		keys = append(keys, code)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	out := make([]cidRange, 0, len(keys))
	for _, code := range keys {
		cid := m[code]
		if n := len(out); n > 0 {
			last := &out[n-1]
			if last.hi != 0xFFFFFFFF && last.hi+1 == code && uint64(last.cid)+uint64(code-last.lo) == uint64(cid) {
				last.hi = code
				continue
			}
		}
		out = append(out, cidRange{lo: code, hi: code, cid: cid})
	}
	return out
}

func identityCMap(wmode int) *predefinedCMap {
	return &predefinedCMap{
		name: "Identity", wmode: wmode, kind: unicodeNone, maxBytes: 2,
		spaces: []codeSpace{{Bytes: 2, Lo: 0, Hi: 0xFFFF}},
		cid:    [5][]cidRange{2: {{lo: 0, hi: 0xFFFF, cid: 0}}},
	}
}

func cmapBaseByName(name string) *predefinedCMap {
	switch name {
	case "Identity-H":
		return identityCMap(0)
	case "Identity-V":
		return identityCMap(1)
	}
	return predefinedCMapByName(name)
}
