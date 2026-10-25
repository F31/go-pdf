package render

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/F31/go-pdf/displaylist"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

const maxSystemFontFile = 128 << 20

type systemFontEntry struct {
	path       string
	collection bool
	index      int
	family     string
	bold       bool
	italic     bool
	mono       bool
	serif      bool
	cjk        bool
	locale     string

	mu     sync.Mutex
	face   *sfnt.Font
	loaded bool
	err    error
}

type systemFontProvider struct {
	families map[string][]*systemFontEntry
	all      []*systemFontEntry
}

// SystemFonts scans the platform font directories and returns a provider that
// matches embedded PDF font names to installed fonts. Optional dirs override
// the platform defaults. TrueType/OpenType singles and TTC/OTC collections are
// supported.
func SystemFonts(dirs ...string) (FontProvider, error) {
	if len(dirs) == 0 {
		dirs = defaultSystemFontDirs()
	}
	p := &systemFontProvider{families: map[string][]*systemFontEntry{}}
	seen := map[string]bool{}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".ttf" && ext != ".otf" && ext != ".ttc" && ext != ".otc" {
				return nil
			}
			abs, err := filepath.Abs(path)
			if err != nil || seen[abs] {
				return nil
			}
			seen[abs] = true
			metas, err := readFontMetadata(path)
			if err != nil {
				return nil
			}
			for _, meta := range metas {
				if meta.family == "" {
					continue
				}
				entry := &systemFontEntry{
					path: abs, collection: meta.collection, index: meta.index,
					family: normalizeFontName(meta.family),
				}
				style := strings.ToLower(meta.sub)
				entry.bold = strings.Contains(style, "bold") || strings.Contains(style, "semibold") || strings.Contains(style, "heavy")
				entry.italic = strings.Contains(style, "italic") || strings.Contains(style, "oblique")
				entry.mono = isMonoFamily(meta.family)
				entry.serif = isSerifFamily(meta.family)
				entry.cjk = isCJKFamily(meta.family)
				entry.locale = cjkLocaleFromName(meta.family)
				p.families[entry.family] = append(p.families[entry.family], entry)
				p.all = append(p.all, entry)
			}
			return nil
		})
	}
	if len(p.all) == 0 {
		return nil, fmt.Errorf("render: no system fonts found")
	}
	return p, nil
}

func defaultSystemFontDirs() []string {
	home, _ := os.UserHomeDir()
	var dirs []string
	if home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".fonts"),
			filepath.Join(home, ".local", "share", "fonts"),
			filepath.Join(home, "Library", "Fonts"),
		)
	}
	if windir := os.Getenv("WINDIR"); windir != "" {
		dirs = append(dirs, filepath.Join(windir, "Fonts"))
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		dirs = append(dirs, filepath.Join(local, "Microsoft", "Windows", "Fonts"))
	}
	dirs = append(dirs,
		"/usr/share/fonts",
		"/usr/local/share/fonts",
		"/System/Library/Fonts",
		"/Library/Fonts",
	)
	return dirs
}

func (p *systemFontProvider) GlyphOutline(info *displaylist.FontInfo, glyph displaylist.Glyph) ([]displaylist.Segment, bool, error) {
	if p == nil || info == nil || info.Kind == "Type3" {
		return nil, false, nil
	}
	r, ok := fallbackRune(glyph)
	if !ok {
		return nil, false, nil
	}
	for _, e := range p.candidates(info, r) {
		face, err := e.load()
		if err != nil {
			continue
		}
		var buf sfnt.Buffer
		gid, err := face.GlyphIndex(&buf, r)
		if err != nil || gid == 0 {
			continue
		}
		upem := int(face.UnitsPerEm())
		segs, err := face.LoadGlyph(&buf, gid, fixed.I(upem), nil)
		if err != nil {
			continue
		}
		out := fallbackSegments(segs, upem)
		if len(out) > 0 {
			return out, true, nil
		}
	}
	return nil, false, nil
}

func (e *systemFontEntry) load() (*sfnt.Font, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.loaded {
		return e.face, e.err
	}
	e.loaded = true
	st, err := os.Stat(e.path)
	if err != nil || st.Size() > maxSystemFontFile {
		e.err = fmt.Errorf("render: cannot load system font")
		return nil, e.err
	}
	data, err := os.ReadFile(e.path)
	if err != nil {
		e.err = fmt.Errorf("render: cannot read system font")
		return nil, e.err
	}
	if e.collection {
		var col *sfnt.Collection
		col, err = sfnt.ParseCollection(data)
		if err == nil {
			e.face, err = col.Font(e.index)
		}
	} else {
		e.face, err = sfnt.Parse(data)
	}
	if err != nil {
		e.err = fmt.Errorf("render: cannot parse system font")
		return nil, e.err
	}
	return e.face, nil
}

func (p *systemFontProvider) candidates(info *displaylist.FontInfo, r rune) []*systemFontEntry {
	locale := cjkLocaleFromName(info.Family)
	if locale == "" {
		locale = cjkLocaleFromName(info.BaseFont)
	}
	if locale == "" {
		locale = cjkLocaleFromRune(r)
	}
	keys := []string{normalizeFontName(info.Family), normalizeFontName(info.BaseFont)}
	for _, key := range append([]string(nil), keys...) {
		for _, alias := range systemAliases[key] {
			keys = append(keys, normalizeFontName(alias))
		}
	}
	seen := map[*systemFontEntry]bool{}
	var out []*systemFontEntry
	add := func(list []*systemFontEntry) {
		list = append([]*systemFontEntry(nil), list...)
		sort.SliceStable(list, func(i, j int) bool {
			return fontStyleScore(list[i], info, locale) > fontStyleScore(list[j], info, locale)
		})
		for _, e := range list {
			if e != nil && !seen[e] {
				seen[e] = true
				out = append(out, e)
			}
		}
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		add(p.families[key])
	}
	if isCJKRune(r) {
		var cjk []*systemFontEntry
		for _, e := range p.all {
			if e.cjk {
				cjk = append(cjk, e)
			}
		}
		add(cjk)
	}
	var generic []*systemFontEntry
	for _, e := range p.all {
		if info.Mono != e.mono && (info.Mono || e.mono) {
			continue
		}
		if info.Serif != e.serif && (info.Serif || e.serif) {
			continue
		}
		generic = append(generic, e)
	}
	if len(generic) == 0 {
		for _, e := range p.all {
			if e.mono == info.Mono || e.serif == info.Serif {
				generic = append(generic, e)
			}
		}
	}
	add(generic)
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}

func fontStyleScore(e *systemFontEntry, info *displaylist.FontInfo, locale string) int {
	score := 0
	if e.bold == info.Bold {
		score += 2
	}
	if e.italic == info.Italic {
		score += 2
	}
	if e.mono == info.Mono {
		score++
	}
	if e.serif == info.Serif {
		score++
	}
	if locale != "" {
		if e.locale == locale {
			score += 6
		} else if e.locale != "" {
			score -= 4
		}
	}
	return score
}

func normalizeFontName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isMonoFamily(family string) bool {
	f := strings.ToLower(family)
	for _, s := range []string{"mono", "courier", "consolas", "menlo", "monaco", "code", "terminal"} {
		if strings.Contains(f, s) {
			return true
		}
	}
	return false
}

func isSerifFamily(family string) bool {
	f := strings.ToLower(family)
	for _, s := range []string{"serif", "times", "georgia", "garamond", "book", "roman", "song", "ming", "宋", "明"} {
		if strings.Contains(f, s) {
			return true
		}
	}
	return false
}

func cjkLocaleFromName(name string) string {
	f := strings.ToLower(strings.TrimSpace(name))
	if f == "" {
		return ""
	}
	switch {
	case strings.Contains(f, "simsun"), strings.Contains(f, "simhei"), strings.Contains(f, "fangsong"), strings.Contains(f, "kaiti"), strings.Contains(f, "yahei"), strings.Contains(f, "dengxian"), strings.Contains(f, "songti"):
		return "zh-Hans"
	case strings.Contains(f, "mingliu"), strings.Contains(f, "msung"), strings.Contains(f, "dfkai"), strings.Contains(f, "lihei"), strings.Contains(f, "bopomofo"):
		return "zh-Hant"
	case strings.Contains(f, "mincho"), strings.Contains(f, "meiryo"), strings.Contains(f, "hiragino"), strings.Contains(f, "osaka"), strings.Contains(f, "yu gothic"):
		return "ja"
	case strings.Contains(f, "batang"), strings.Contains(f, "gulim"), strings.Contains(f, "malgun"), strings.Contains(f, "dotum"), strings.Contains(f, "gungsuh"), strings.Contains(f, "nanum"):
		return "ko"
	case strings.Contains(f, "pingfang"):
		return "zh-Hans"
	case strings.Contains(f, "hiragino"):
		return "ja"
	}
	if strings.Contains(f, "cjk") || strings.Contains(f, "source han") || strings.Contains(f, "noto sans") || strings.Contains(f, "noto serif") {
		switch {
		case hasFontToken(f, "sc"):
			return "zh-Hans"
		case hasFontToken(f, "tc"), hasFontToken(f, "hk"):
			return "zh-Hant"
		case hasFontToken(f, "jp"):
			return "ja"
		case hasFontToken(f, "kr"):
			return "ko"
		}
	}
	return ""
}

func hasFontToken(name, token string) bool {
	if strings.HasSuffix(name, token) {
		return true
	}
	for _, part := range strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if part == token {
			return true
		}
	}
	return false
}

func cjkLocaleFromRune(r rune) string {
	switch {
	case unicode.In(r, unicode.Hiragana, unicode.Katakana):
		return "ja"
	case unicode.In(r, unicode.Hangul):
		return "ko"
	case unicode.In(r, unicode.Bopomofo):
		return "zh-Hant"
	default:
		return ""
	}
}

func isCJKRune(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Bopomofo)
}

func isCJKFamily(family string) bool {
	f := strings.ToLower(family)
	for _, s := range []string{"cjk", "wenquanyi", "wqy", "source han", "droid sans fallback", "simsun", "simhei", "yahei", "mincho", "gothic", "pingfang", "hiragino", "song", "hei", "kai", "ming", "宋", "黑", "楷", "明"} {
		if strings.Contains(f, s) {
			return true
		}
	}
	return false
}

var systemAliases = map[string][]string{
	"helvetica":     {"Arial", "Liberation Sans", "Nimbus Sans", "DejaVu Sans"},
	"arial":         {"Helvetica", "Liberation Sans", "Nimbus Sans", "DejaVu Sans"},
	"times":         {"Times New Roman", "Liberation Serif", "Nimbus Roman", "DejaVu Serif"},
	"timesnewroman": {"Times", "Liberation Serif", "Nimbus Roman", "DejaVu Serif"},
	"courier":       {"Courier New", "Liberation Mono", "Nimbus Mono PS", "DejaVu Sans Mono"},
	"couriernew":    {"Courier", "Liberation Mono", "Nimbus Mono PS", "DejaVu Sans Mono"},
}

type fontMetadata struct {
	family     string
	sub        string
	collection bool
	index      int
}

func readFontMetadata(path string) ([]fontMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() > maxSystemFontFile {
		return nil, fmt.Errorf("render: invalid font file")
	}
	header := make([]byte, 12)
	if _, err := io.ReadFull(f, header); err != nil {
		return nil, err
	}
	magic := binary.BigEndian.Uint32(header[:4])
	if magic == 0x74746366 { // "ttcf"
		count := int(binary.BigEndian.Uint32(header[8:12]))
		if count <= 0 || count > 4096 {
			return nil, fmt.Errorf("render: invalid font collection")
		}
		offsets := make([]byte, 4*count)
		if _, err := io.ReadFull(f, offsets); err != nil {
			return nil, err
		}
		out := make([]fontMetadata, 0, count)
		for i := 0; i < count; i++ {
			off := int64(binary.BigEndian.Uint32(offsets[4*i : 4*i+4]))
			family, sub, err := readNameAt(f, st.Size(), off)
			if err != nil || family == "" {
				continue
			}
			out = append(out, fontMetadata{family: family, sub: sub, collection: true, index: i})
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("render: no usable fonts in collection")
		}
		return out, nil
	}
	if magic != 0x00010000 && magic != 0x4f54544f && magic != 0x74727565 {
		return nil, fmt.Errorf("render: unsupported font container")
	}
	family, sub, err := readNameAt(f, st.Size(), 0)
	if err != nil || family == "" {
		return nil, fmt.Errorf("render: missing font family")
	}
	return []fontMetadata{{family: family, sub: sub, index: -1}}, nil
}

func readNameAt(f *os.File, size, fontOffset int64) (string, string, error) {
	header := make([]byte, 12)
	if _, err := f.ReadAt(header, fontOffset); err != nil {
		return "", "", err
	}
	magic := binary.BigEndian.Uint32(header[:4])
	if magic != 0x00010000 && magic != 0x4f54544f && magic != 0x74727565 {
		return "", "", fmt.Errorf("render: unsupported font container")
	}
	count := int(binary.BigEndian.Uint16(header[4:6]))
	if count <= 0 || count > 256 {
		return "", "", fmt.Errorf("render: invalid font table count")
	}
	records := make([]byte, 16*count)
	if _, err := f.ReadAt(records, fontOffset+12); err != nil {
		return "", "", err
	}
	var nameOff, nameLen int64
	for i := 0; i < count; i++ {
		rec := records[16*i : 16*i+16]
		if binary.BigEndian.Uint32(rec[:4]) == 0x6e616d65 {
			nameOff = int64(binary.BigEndian.Uint32(rec[8:12]))
			nameLen = int64(binary.BigEndian.Uint32(rec[12:16]))
			break
		}
	}
	if nameOff <= 0 || nameLen < 6 || nameOff+nameLen > size {
		return "", "", fmt.Errorf("render: missing name table")
	}
	data := make([]byte, nameLen)
	if _, err := f.ReadAt(data, nameOff); err != nil {
		return "", "", err
	}
	family, sub := parseNameTable(data)
	if family == "" {
		return "", "", fmt.Errorf("render: missing font family")
	}
	return family, sub, nil
}

func parseNameTable(data []byte) (string, string) {
	if len(data) < 6 {
		return "", ""
	}
	count := int(binary.BigEndian.Uint16(data[2:4]))
	base := int(binary.BigEndian.Uint16(data[4:6]))
	if count <= 0 || count > 4096 || 6+12*count > len(data) {
		return "", ""
	}
	type best struct {
		value string
		score int
	}
	var family, sub best
	for i := 0; i < count; i++ {
		rec := data[6+12*i : 18+12*i]
		platform := binary.BigEndian.Uint16(rec[0:2])
		encoding := binary.BigEndian.Uint16(rec[2:4])
		language := binary.BigEndian.Uint16(rec[4:6])
		nameID := binary.BigEndian.Uint16(rec[6:8])
		length := int(binary.BigEndian.Uint16(rec[8:10]))
		offset := int(binary.BigEndian.Uint16(rec[10:12]))
		if length == 0 || base+offset < 0 || base+offset+length > len(data) {
			continue
		}
		if platform != 3 && platform != 0 {
			continue
		}
		s := decodeUTF16BE(data[base+offset : base+offset+length])
		s = strings.TrimSpace(strings.TrimRight(s, "\x00"))
		if s == "" {
			continue
		}
		score := 0
		if platform == 3 {
			score += 4
		}
		if encoding == 1 {
			score += 2
		}
		if language == 0x0409 {
			score += 2
		}
		switch nameID {
		case 16:
			if score >= family.score {
				family = best{value: s, score: score + 2}
			}
		case 1:
			if score >= family.score {
				family = best{value: s, score: score}
			}
		case 17:
			if score >= sub.score {
				sub = best{value: s, score: score + 2}
			}
		case 2:
			if score >= sub.score {
				sub = best{value: s, score: score}
			}
		}
	}
	return family.value, sub.value
}

func decodeUTF16BE(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, binary.BigEndian.Uint16(b[i:i+2]))
	}
	return string(utf16Decode(u))
}

func utf16Decode(u []uint16) []rune {
	out := make([]rune, 0, len(u))
	for i := 0; i < len(u); i++ {
		c := u[i]
		if c < 0xD800 || c > 0xDFFF {
			out = append(out, rune(c))
			continue
		}
		if c >= 0xD800 && c <= 0xDBFF && i+1 < len(u) {
			d := u[i+1]
			if d >= 0xDC00 && d <= 0xDFFF {
				out = append(out, rune(0x10000+(uint32(c-0xD800)<<10)+uint32(d-0xDC00)))
				i++
				continue
			}
		}
		out = append(out, unicode.ReplacementChar)
	}
	return out
}
