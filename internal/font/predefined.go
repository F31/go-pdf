package font

import (
	_ "embed"
	"encoding/binary"
	"sync"
	"unicode/utf16"
)

//go:embed data/cmap.bin
var cmapBinary []byte

const cmapMagic = "GPDFCMAP"

const (
	unicodeNone  = 0
	unicodeUCS2  = 1
	unicodeUTF16 = 2
	unicodeTable = 3
)

type cidRange struct {
	lo, hi, cid uint32
}

type unicodeRange struct {
	lo, hi uint32
	dst    rune
}

// predefinedCMap represents either an embedded predefined CMap or a CMap
// parsed from an /Encoding stream. Both share code-space, CID, and Unicode
// lookup behavior.
type predefinedCMap struct {
	name     string
	wmode    int
	kind     int
	spaces   []codeSpace
	maxBytes int
	cid      [5][]cidRange
	notdef   [5][]cidRange
	unicode  [5][]unicodeRange
}

type binaryReader struct {
	data []byte
	pos  int
	err  error
}

func (r *binaryReader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.pos+n > len(r.data) {
		r.err = errCMapData
		return nil
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b
}

func (r *binaryReader) u8() uint8 {
	b := r.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}

func (r *binaryReader) u16() uint16 {
	b := r.take(2)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint16(b)
}

func (r *binaryReader) u32() uint32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}

type cmapDataError struct{}

func (cmapDataError) Error() string { return "font: invalid embedded CMap data" }

var errCMapData = cmapDataError{}

var (
	predefinedOnce sync.Once
	predefined     map[string]*predefinedCMap
	predefinedErr  error
)

func predefinedCMapByName(name string) *predefinedCMap {
	predefinedOnce.Do(func() {
		predefined, predefinedErr = parsePredefinedCMaps(cmapBinary)
	})
	return predefined[name]
}

func predefinedDataError() error {
	predefinedOnce.Do(func() {
		predefined, predefinedErr = parsePredefinedCMaps(cmapBinary)
	})
	return predefinedErr
}

func parsePredefinedCMaps(data []byte) (map[string]*predefinedCMap, error) {
	r := &binaryReader{data: data}
	if string(r.take(len(cmapMagic))) != cmapMagic {
		return nil, errCMapData
	}
	if version := r.u16(); version != 1 {
		return nil, errCMapData
	}
	count := int(r.u16())
	out := make(map[string]*predefinedCMap, count)
	for i := 0; i < count && r.err == nil; i++ {
		nameLen := int(r.u8())
		name := string(r.take(nameLen))
		cm := &predefinedCMap{name: name, wmode: int(r.u8()), kind: int(r.u8())}
		nspaces := int(r.u16())
		cm.spaces = make([]codeSpace, 0, nspaces)
		for j := 0; j < nspaces && r.err == nil; j++ {
			n := int(r.u8())
			lo, hi := r.u32(), r.u32()
			if n < 1 || n > 4 || hi < lo {
				return nil, errCMapData
			}
			if n > cm.maxBytes {
				cm.maxBytes = n
			}
			cm.spaces = append(cm.spaces, codeSpace{Bytes: n, Lo: lo, Hi: hi})
		}
		if err := readCIDRanges(r, &cm.cid); err != nil {
			return nil, err
		}
		if err := readCIDRanges(r, &cm.notdef); err != nil {
			return nil, err
		}
		nuni := int(r.u32())
		for j := 0; j < nuni && r.err == nil; j++ {
			n := int(r.u8())
			lo, hi, dst := r.u32(), r.u32(), r.u32()
			if n < 1 || n > 4 || hi < lo || dst > 0x10FFFF {
				return nil, errCMapData
			}
			cm.unicode[n] = append(cm.unicode[n], unicodeRange{lo: lo, hi: hi, dst: rune(dst)})
		}
		if r.err != nil {
			return nil, errCMapData
		}
		out[name] = cm
	}
	if r.err != nil || r.pos != len(data) {
		return nil, errCMapData
	}
	return out, nil
}

func readCIDRanges(r *binaryReader, dst *[5][]cidRange) error {
	nranges := int(r.u32())
	for j := 0; j < nranges && r.err == nil; j++ {
		n := int(r.u8())
		lo, hi, cid := r.u32(), r.u32(), r.u32()
		if n < 1 || n > 4 || hi < lo {
			return errCMapData
		}
		dst[n] = append(dst[n], cidRange{lo: lo, hi: hi, cid: cid})
	}
	if r.err != nil {
		return errCMapData
	}
	return nil
}

func (cm *predefinedCMap) hasUnicode() bool {
	if cm == nil {
		return false
	}
	if cm.kind == unicodeUCS2 || cm.kind == unicodeUTF16 || cm.kind == unicodeTable {
		return true
	}
	for n := range cm.unicode {
		if len(cm.unicode[n]) > 0 {
			return true
		}
	}
	return false
}

func (cm *predefinedCMap) split(data []byte) ([]Code, int) {
	if cm == nil || len(data) == 0 {
		return nil, 0
	}
	out := make([]Code, 0, len(data))
	pos := 0
	for pos < len(data) {
		matched := false
		for n := cm.maxBytes; n >= 1; n-- {
			if pos+n > len(data) {
				continue
			}
			value := codeOf(data[pos : pos+n])
			if !cm.matches(n, value) {
				continue
			}
			cid := cm.cidFor(n, value)
			out = append(out, Code{Value: value, Bytes: n, CID: cid})
			pos += n
			matched = true
			break
		}
		if !matched {
			break
		}
	}
	return out, pos
}

func (cm *predefinedCMap) matches(n int, code uint32) bool {
	for _, s := range cm.spaces {
		if s.Bytes == n && code >= s.Lo && code <= s.Hi {
			return true
		}
	}
	return false
}

func (cm *predefinedCMap) cidFor(n int, code uint32) uint32 {
	if cid, ok := lookupCID(cm.cid[n], code); ok {
		return cid
	}
	if cid, ok := lookupCID(cm.notdef[n], code); ok {
		return cid
	}
	return 0
}

func lookupCID(ranges []cidRange, code uint32) (uint32, bool) {
	lo, hi := 0, len(ranges)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		r := ranges[mid]
		switch {
		case code < r.lo:
			hi = mid
		case code > r.hi:
			lo = mid + 1
		default:
			return r.cid + code - r.lo, true
		}
	}
	return 0, false
}

func (cm *predefinedCMap) decode(c Code) (string, bool) {
	if cm == nil {
		return "\uFFFD", false
	}
	if r, ok := lookupUnicode(cm.unicode[c.Bytes], c.Value); ok {
		return string(r), true
	}
	switch cm.kind {
	case unicodeUCS2:
		if c.Bytes == 2 && !isSurrogate(uint16(c.Value)) {
			return string(rune(c.Value)), true
		}
	case unicodeUTF16:
		switch c.Bytes {
		case 2:
			v := uint16(c.Value)
			if !isSurrogate(v) {
				return string(rune(v)), true
			}
		case 4:
			hi, lo := uint16(c.Value>>16), uint16(c.Value)
			if hi >= 0xD800 && hi <= 0xDBFF && lo >= 0xDC00 && lo <= 0xDFFF {
				return string(utf16.DecodeRune(rune(hi), rune(lo))), true
			}
		}
	}
	return "\uFFFD", false
}

func lookupUnicode(ranges []unicodeRange, code uint32) (rune, bool) {
	lo, hi := 0, len(ranges)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		r := ranges[mid]
		switch {
		case code < r.lo:
			hi = mid
		case code > r.hi:
			lo = mid + 1
		default:
			return r.dst + rune(code-r.lo), true
		}
	}
	return 0, false
}

func isSurrogate(v uint16) bool {
	return v >= 0xD800 && v <= 0xDFFF
}
