package engine

import (
	"image"
	"image/color"
	"strconv"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Colors
var (
	ColorBackground = parseHex("#1E1E2E")
	ColorTrackLane  = parseHex("#2A2A3E")
	ColorTrackAlt   = parseHex("#252538")
	ColorRuler      = parseHex("#3A3A4E")
	ColorRulerTick  = parseHex("#555570")
	ColorVideoClip  = parseHex("#6C5CE7")
	ColorAudioClip  = parseHex("#00B894")
	ColorIntroSlide = parseHex("#2D3436")
	ColorOutroSlide = parseHex("#636E72")
	ColorDisabled   = color.RGBA{80, 80, 80, 180}
	ColorText       = color.RGBA{255, 255, 255, 255}
	ColorTextDim    = color.RGBA{176, 176, 192, 255}
	ColorBorder     = parseHex("#444466")
	ColorArrow      = parseHex("#888899")
	ColorStoryBg    = parseHex("#1A1A2A")
	ColorStoryFrame = parseHex("#3A3A5A")
)

// Layout constants
const (
	TrackLaneHeight    = 60
	TrackLabelWidth    = 60
	RulerHeight        = 28
	TimelineWidth      = 1200
	ThumbnailHeight    = 48
	Padding            = 8
	StoryThumbWidth    = 160
	StoryThumbHeight   = 90
	StoryLabelHeight   = 36
	StoryArrowWidth    = 30
	StoryPadding       = 12
)

// DefaultFont returns the basic 7x13 font face
func DefaultFont() font.Face {
	return basicfont.Face7x13
}

// LayoutMetrics computes canvas dimensions and coordinate mapping
type LayoutMetrics struct {
	TotalDuration float64
	TrackCount    int
	CanvasWidth   int
	CanvasHeight  int
	ContentWidth  int
}

func NewLayoutMetrics(totalDuration float64, trackCount int) *LayoutMetrics {
	if trackCount < 1 {
		trackCount = 1
	}
	contentW := TimelineWidth - TrackLabelWidth - Padding*2
	canvasH := RulerHeight + trackCount*TrackLaneHeight + Padding*2
	return &LayoutMetrics{
		TotalDuration: totalDuration,
		TrackCount:    trackCount,
		CanvasWidth:   TimelineWidth,
		CanvasHeight:  canvasH,
		ContentWidth:  contentW,
	}
}

// XForTime maps a time in seconds to a pixel X coordinate
func (m *LayoutMetrics) XForTime(seconds float64) int {
	if m.TotalDuration <= 0 {
		return TrackLabelWidth + Padding
	}
	ratio := seconds / m.TotalDuration
	return TrackLabelWidth + Padding + int(ratio*float64(m.ContentWidth))
}

// TrackY returns the top Y pixel for a track by index
func (m *LayoutMetrics) TrackY(index int) int {
	return RulerHeight + index*TrackLaneHeight
}

// Drawing helpers

func DrawRect(img *image.RGBA, x, y, w, h int, c color.Color) {
	r := image.Rect(x, y, x+w, y+h)
	r = r.Intersect(img.Bounds())
	cr, cg, cb, ca := c.RGBA()
	for py := r.Min.Y; py < r.Max.Y; py++ {
		for px := r.Min.X; px < r.Max.X; px++ {
			if ca == 0xFFFF {
				img.Set(px, py, c)
			} else {
				// Alpha blend
				bg := img.RGBAAt(px, py)
				a := float64(ca) / 0xFFFF
				img.SetRGBA(px, py, color.RGBA{
					R: uint8(float64(cr>>8)*a + float64(bg.R)*(1-a)),
					G: uint8(float64(cg>>8)*a + float64(bg.G)*(1-a)),
					B: uint8(float64(cb>>8)*a + float64(bg.B)*(1-a)),
					A: 255,
				})
			}
		}
	}
}

func DrawRectOutline(img *image.RGBA, x, y, w, h int, c color.Color, thickness int) {
	DrawRect(img, x, y, w, thickness, c)           // top
	DrawRect(img, x, y+h-thickness, w, thickness, c) // bottom
	DrawRect(img, x, y, thickness, h, c)             // left
	DrawRect(img, x+w-thickness, y, thickness, h, c) // right
}

func DrawText(img *image.RGBA, x, y int, label string, c color.Color) {
	face := DefaultFont()
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(label)
}

func DrawTextTruncated(img *image.RGBA, x, y int, label string, c color.Color, maxWidth int) {
	charW := 7 // basicfont.Face7x13 is 7px wide
	maxChars := maxWidth / charW
	if maxChars < 1 {
		return
	}
	if len(label) > maxChars {
		if maxChars > 2 {
			label = label[:maxChars-2] + ".."
		} else {
			label = label[:maxChars]
		}
	}
	DrawText(img, x, y, label, c)
}

// FormatTimecode formats seconds as MM:SS
func FormatTimecode(seconds float64) string {
	m := int(seconds) / 60
	s := int(seconds) % 60
	return strconv.Itoa(m) + ":" + padZero(s)
}

func padZero(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func parseHex(hex string) color.RGBA {
	if len(hex) > 0 && hex[0] == '#' {
		hex = hex[1:]
	}
	if len(hex) != 6 {
		return color.RGBA{128, 128, 128, 255}
	}
	r, _ := strconv.ParseUint(hex[0:2], 16, 8)
	g, _ := strconv.ParseUint(hex[2:4], 16, 8)
	b, _ := strconv.ParseUint(hex[4:6], 16, 8)
	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
}
