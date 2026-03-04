package tray

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strings"
)

var iconSize = 22

// StatusColor represents the overall tray icon color based on tunnel states.
type StatusColor int

const (
	ColorGray  StatusColor = iota // all disconnected
	ColorGreen                    // at least one connected, no errors
	ColorRed                      // at least one error
)

// GenerateIcon creates a tray icon PNG with a solid colored circle on a
// transparent background. Simple and reliable across all platforms.
func GenerateIcon(sc StatusColor, badge int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))

	// Main circle color
	var base color.RGBA
	switch sc {
	case ColorGreen:
		base = color.RGBA{R: 74, G: 222, B: 128, A: 255} // green-400
	case ColorRed:
		base = color.RGBA{R: 248, G: 113, B: 113, A: 255} // red-400
	default:
		base = color.RGBA{R: 161, G: 161, B: 170, A: 255} // zinc-400
	}

	// Draw a simple solid circle — no highlights, no badges.
	cx, cy := float64(iconSize)/2, float64(iconSize)/2
	radius := float64(iconSize)/2 - 2
	drawSolidCircle(img, cx, cy, radius, base)

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return wrapIconBytes(buf.Bytes())
}

// GenerateIconWithDots creates a tray icon with small colored dots along the
// bottom for each connected tunnel's assigned color.
func GenerateIconWithDots(sc StatusColor, dots []color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))

	var base color.RGBA
	switch sc {
	case ColorGreen:
		base = color.RGBA{R: 74, G: 222, B: 128, A: 255}
	case ColorRed:
		base = color.RGBA{R: 248, G: 113, B: 113, A: 255}
	default:
		base = color.RGBA{R: 161, G: 161, B: 170, A: 255}
	}

	cx, cy := float64(iconSize)/2, float64(iconSize)/2
	radius := float64(iconSize)/2 - 1
	drawFilledCircle(img, cx, cy, radius, base)

	highlight := color.RGBA{R: 255, G: 255, B: 255, A: 40}
	drawFilledCircle(img, cx-1, cy-1, radius-3, highlight)

	// Draw colored dots along the bottom edge
	if len(dots) > 0 {
		dotRadius := 2.5
		maxDots := 5 // limit to avoid overcrowding
		count := len(dots)
		if count > maxDots {
			count = maxDots
		}

		// Evenly space dots across the bottom
		totalWidth := float64(count)*dotRadius*2 + float64(count-1)*1
		startX := (float64(iconSize) - totalWidth) / 2 + dotRadius
		dotY := float64(iconSize) - dotRadius - 0.5

		// Dark background strip behind dots for visibility
		strip := color.RGBA{R: 24, G: 24, B: 27, A: 200}
		for i := 0; i < count; i++ {
			dx := startX + float64(i)*(dotRadius*2+1)
			drawFilledCircle(img, dx, dotY, dotRadius+1, strip)
		}
		for i := 0; i < count; i++ {
			dx := startX + float64(i)*(dotRadius*2+1)
			drawFilledCircle(img, dx, dotY, dotRadius, dots[i])
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// drawSolidCircle draws a hard-edged solid circle with no alpha blending.
// More compatible with Windows system tray which can mishandle semi-transparent pixels.
func drawSolidCircle(img *image.RGBA, cx, cy, r float64, c color.RGBA) {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func drawFilledCircle(img *image.RGBA, cx, cy, r float64, c color.RGBA) {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist <= r {
				if dist > r-1 {
					alpha := uint8(float64(c.A) * (r - dist))
					blended := color.RGBA{R: c.R, G: c.G, B: c.B, A: alpha}
					blendPixel(img, x, y, blended)
				} else {
					blendPixel(img, x, y, c)
				}
			}
		}
	}
}

func blendPixel(img *image.RGBA, x, y int, c color.RGBA) {
	existing := img.RGBAAt(x, y)
	if existing.A == 0 {
		img.SetRGBA(x, y, c)
		return
	}
	sa := float64(c.A) / 255
	da := float64(existing.A) / 255
	outA := sa + da*(1-sa)
	if outA == 0 {
		return
	}
	outR := (float64(c.R)*sa + float64(existing.R)*da*(1-sa)) / outA
	outG := (float64(c.G)*sa + float64(existing.G)*da*(1-sa)) / outA
	outB := (float64(c.B)*sa + float64(existing.B)*da*(1-sa)) / outA
	img.SetRGBA(x, y, color.RGBA{
		R: uint8(outR),
		G: uint8(outG),
		B: uint8(outB),
		A: uint8(outA * 255),
	})
}

// drawBadge draws a small digit (1-9) in the bottom-right corner.
func drawBadge(img *image.RGBA, n int) {
	if n < 1 || n > 9 {
		return
	}

	badgeColor := color.RGBA{R: 30, G: 30, B: 36, A: 230}
	bcx, bcy := float64(iconSize)-4.5, float64(iconSize)-4.5
	drawFilledCircle(img, bcx, bcy, 4.5, badgeColor)

	glyph := digitGlyphs[n]
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	ox, oy := iconSize-7, iconSize-8
	for row := 0; row < 5; row++ {
		for col := 0; col < 3; col++ {
			if glyph[row]&(1<<(2-col)) != 0 {
				px, py := ox+col, oy+row
				if px >= 0 && px < iconSize && py >= 0 && py < iconSize {
					img.SetRGBA(px, py, white)
				}
			}
		}
	}
}

var digitGlyphs = [10][5]byte{
	{0b111, 0b101, 0b101, 0b101, 0b111}, // 0
	{0b010, 0b110, 0b010, 0b010, 0b111}, // 1
	{0b111, 0b001, 0b111, 0b100, 0b111}, // 2
	{0b111, 0b001, 0b111, 0b001, 0b111}, // 3
	{0b101, 0b101, 0b111, 0b001, 0b001}, // 4
	{0b111, 0b100, 0b111, 0b001, 0b111}, // 5
	{0b111, 0b100, 0b111, 0b101, 0b111}, // 6
	{0b111, 0b001, 0b010, 0b010, 0b010}, // 7
	{0b111, 0b101, 0b111, 0b101, 0b111}, // 8
	{0b111, 0b101, 0b111, 0b001, 0b111}, // 9
}

// StatusDotIcon creates a 16x16 PNG with a colored dot — used as menu item icons.
// On Windows the result is wrapped in an ICO container so LoadImageW can load it.
func StatusDotIcon(c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	drawFilledCircle(img, 8, 8, 4, c)

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return wrapDotIconBytes(buf.Bytes())
}

var (
	DotGreen  = color.RGBA{R: 74, G: 222, B: 128, A: 255}
	DotBlue   = color.RGBA{R: 96, G: 165, B: 250, A: 255}
	DotOrange = color.RGBA{R: 251, G: 146, B: 60, A: 255}
	DotRed    = color.RGBA{R: 248, G: 113, B: 113, A: 255}
	DotGray   = color.RGBA{R: 113, G: 113, B: 122, A: 255}
)

// PresetColors are the selectable tunnel colors (name → hex).
var PresetColors = map[string]string{
	"red":    "#ef4444",
	"orange": "#f97316",
	"yellow": "#eab308",
	"green":  "#22c55e",
	"cyan":   "#06b6d4",
	"blue":   "#3b82f6",
	"purple": "#a855f7",
	"pink":   "#ec4899",
}

// ParseHexColor converts a "#rrggbb" string to color.RGBA.
// Returns a default gray on invalid input.
func ParseHexColor(hex string) color.RGBA {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return DotGray
	}
	var r, g, b uint8
	_, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	if err != nil {
		return DotGray
	}
	return color.RGBA{R: r, G: g, B: b, A: 255}
}
