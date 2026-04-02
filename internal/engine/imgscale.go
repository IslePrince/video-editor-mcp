package engine

import (
	"image"
	"image/color"
)

// ScaleImage performs nearest-neighbor scaling of an image.
func ScaleImage(src image.Image, targetW, targetH int) *image.RGBA {
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))

	for y := 0; y < targetH; y++ {
		srcY := srcBounds.Min.Y + y*srcH/targetH
		for x := 0; x < targetW; x++ {
			srcX := srcBounds.Min.X + x*srcW/targetW
			r, g, b, a := src.At(srcX, srcY).RGBA()
			dst.SetRGBA(x, y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)})
		}
	}

	return dst
}
