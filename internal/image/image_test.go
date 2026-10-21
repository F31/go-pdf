package image

import (
	"testing"

	"github.com/F31/go-pdf/displaylist"
)

func FuzzImageDecode(f *testing.F) {
	f.Add([]byte{255, 0, 0}, uint8(1), uint8(1), uint8(3), uint8(1))
	f.Add([]byte{0, 0, 0, 0}, uint8(1), uint8(1), uint8(4), uint8(2))
	f.Add([]byte{0x40}, uint8(2), uint8(1), uint8(0), uint8(3))
	f.Add([]byte{}, uint8(0), uint8(0), uint8(0), uint8(0))
	f.Fuzz(func(t *testing.T, data []byte, w, h, bpc, cs uint8) {
		if w == 0 || h == 0 || w > 64 || h > 64 {
			return
		}
		bits := []int{1, 2, 4, 8, 16}[int(bpc)%5]
		spaces := []string{"DeviceGray", "DeviceRGB", "DeviceCMYK", "ImageMask", "Indexed"}
		r := &Ref{info: displaylist.ImageInfo{
			Width: int(w), Height: int(h), BitsPerComponent: bits,
			ColorSpace: spaces[int(cs)%len(spaces)],
		}}
		_, _ = r.decodeSamples(data)
	})
}
