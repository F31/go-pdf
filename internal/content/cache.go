package content

import (
	"github.com/F31/go-pdf/displaylist"
	"github.com/F31/go-pdf/internal/diag"
)

func estimateItems(items []displaylist.Item) int64 {
	var n int64
	for _, it := range items {
		n += estimateItem(it)
	}
	return n
}

func estimateItem(it displaylist.Item) int64 {
	switch v := it.(type) {
	case *displaylist.TextRun:
		return 160 + int64(len(v.Glyphs))*64 + int64(len(v.Spans))*48
	case *displaylist.Path:
		return 160 + int64(len(v.Segments))*40 + int64(len(v.Line.Dash))*8
	case *displaylist.Image:
		return 512 + int64(len(v.Info.Decode))*8 + int64(len(v.Decode))*8
	case *displaylist.Shading:
		n := int64(192 + len(v.Stops)*32 + len(v.Raw) + len(v.Decode)*8)
		if v.Grid != nil {
			n += int64(len(v.Grid.Colors)) * 32
		}
		return n
	case *displaylist.Group:
		return 192 + estimateItems(v.Items) + estimateClips(v.Clips)
	}
	return 128
}

func estimatePattern(p *displaylist.Pattern) int64 {
	if p == nil {
		return 0
	}
	return 192 + int64(len(p.Content)) + estimateItems(p.Items) + estimateClips(p.Clips)
}

func estimateSoftMask(sm *displaylist.SoftMask) int64 {
	if sm == nil {
		return 0
	}
	return 160 + estimateItems(sm.Items) + estimateClips(sm.Clips)
}

func estimateClips(clips []displaylist.Clip) int64 {
	var n int64
	for _, c := range clips {
		n += 64 + int64(len(c.Path))*32
	}
	return n
}

func (ip *interp) cacheLease(n int64, code string) bool {
	if n <= 0 || ip.pd.Lease == nil {
		return true
	}
	if _, err := ip.pd.Lease(n); err != nil {
		ip.diagOnce(diag.Degraded, code, "cache budget exceeded")
		return false
	}
	return true
}
