package content

import (
	"testing"

	"github.com/F31/go-pdf/displaylist"
)

func move(x, y float64) displaylist.Segment {
	return displaylist.Segment{Kind: displaylist.SegmentMove, P0: displaylist.Point{X: x, Y: y}}
}
func line(x0, y0, x1, y1 float64) displaylist.Segment {
	return displaylist.Segment{Kind: displaylist.SegmentLine, P0: displaylist.Point{X: x0, Y: y0}, P3: displaylist.Point{X: x1, Y: y1}}
}
func cubic(x0, y0, x1, y1 float64) displaylist.Segment {
	return displaylist.Segment{Kind: displaylist.SegmentCubic, P0: displaylist.Point{X: x0, Y: y0}, P3: displaylist.Point{X: x1, Y: y1}}
}
func closeSeg(x, y float64) displaylist.Segment {
	return displaylist.Segment{Kind: displaylist.SegmentClose, P0: displaylist.Point{X: x, Y: y}}
}

func TestPathHint(t *testing.T) {
	rect := []displaylist.Segment{
		move(0, 0), line(0, 0, 10, 0), line(10, 0, 10, 10), line(10, 10, 0, 10), closeSeg(0, 10),
	}
	if got := pathHint(rect, true); got != displaylist.PathRect {
		t.Fatalf("rect %d", got)
	}
	if got := pathHint([]displaylist.Segment{move(0, 0), line(0, 0, 10, 10)}, false); got != displaylist.PathLine {
		t.Fatalf("line %d", got)
	}
	if got := pathHint([]displaylist.Segment{move(0, 0), line(0, 0, 10, 0), line(10, 0, 10, 10)}, false); got != displaylist.PathPolyline {
		t.Fatalf("polyline %d", got)
	}
	ellipse := []displaylist.Segment{
		move(1, 0),
		cubic(1, 0, 0, 1),
		cubic(0, 1, -1, 0),
		cubic(-1, 0, 0, -1),
		cubic(0, -1, 1, 0),
		closeSeg(1, 0),
	}
	if got := pathHint(ellipse, true); got != displaylist.PathEllipse {
		t.Fatalf("ellipse %d", got)
	}
	rounded := []displaylist.Segment{
		move(2, 0),
		line(2, 0, 8, 0),
		cubic(8, 0, 10, 2),
		line(10, 2, 10, 8),
		cubic(10, 8, 8, 10),
		line(8, 10, 2, 10),
		cubic(2, 10, 0, 8),
		line(0, 8, 0, 2),
		cubic(0, 2, 2, 0),
		closeSeg(2, 0),
	}
	if got := pathHint(rounded, true); got != displaylist.PathRoundRect {
		t.Fatalf("roundrect %d", got)
	}
	if got := pathHint([]displaylist.Segment{move(0, 0), cubic(0, 0, 1, 1)}, false); got != displaylist.PathGeneric {
		t.Fatalf("generic %d", got)
	}
}
