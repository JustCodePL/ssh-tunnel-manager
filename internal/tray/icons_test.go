package tray

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"
)

func TestGenerateIcon_ProducesValidPNG(t *testing.T) {
	for _, sc := range []StatusColor{ColorGray, ColorGreen, ColorRed} {
		data := GenerateIcon(sc, 0)
		if len(data) == 0 {
			t.Fatalf("GenerateIcon(%d, 0) returned empty data", sc)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("GenerateIcon(%d, 0) produced invalid PNG: %v", sc, err)
		}
		bounds := img.Bounds()
		if bounds.Dx() != iconSize || bounds.Dy() != iconSize {
			t.Errorf("icon size = %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), iconSize, iconSize)
		}
	}
}

func TestGenerateIcon_WithBadge(t *testing.T) {
	for badge := 1; badge <= 9; badge++ {
		data := GenerateIcon(ColorGreen, badge)
		if len(data) == 0 {
			t.Fatalf("GenerateIcon(Green, %d) returned empty data", badge)
		}
		_, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("GenerateIcon(Green, %d) produced invalid PNG: %v", badge, err)
		}
	}
}

func TestGenerateIcon_BadgeZeroAndAboveNine(t *testing.T) {
	// Badge 0 and >9 should still produce valid icons (just no badge drawn)
	for _, badge := range []int{0, 10, 99} {
		data := GenerateIcon(ColorGray, badge)
		_, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("GenerateIcon(Gray, %d) produced invalid PNG: %v", badge, err)
		}
	}
}

func TestGenerateIconWithDots(t *testing.T) {
	dots := []color.RGBA{
		{R: 239, G: 68, B: 68, A: 255},   // red
		{R: 34, G: 197, B: 94, A: 255},    // green
		{R: 59, G: 130, B: 246, A: 255},   // blue
	}
	data := GenerateIconWithDots(ColorGreen, dots)
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("GenerateIconWithDots produced invalid PNG: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != iconSize || bounds.Dy() != iconSize {
		t.Errorf("icon size = %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), iconSize, iconSize)
	}
}

func TestGenerateIconWithDots_Empty(t *testing.T) {
	data := GenerateIconWithDots(ColorGray, nil)
	_, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("GenerateIconWithDots(nil dots) produced invalid PNG: %v", err)
	}
}

func TestParseHexColor(t *testing.T) {
	tests := []struct {
		hex  string
		want color.RGBA
	}{
		{"#ef4444", color.RGBA{R: 239, G: 68, B: 68, A: 255}},
		{"#3b82f6", color.RGBA{R: 59, G: 130, B: 246, A: 255}},
		{"ef4444", color.RGBA{R: 239, G: 68, B: 68, A: 255}}, // no # prefix
		{"invalid", DotGray},  // fallback
		{"", DotGray},         // empty
		{"#fff", DotGray},     // too short
	}
	for _, tt := range tests {
		got := ParseHexColor(tt.hex)
		if got != tt.want {
			t.Errorf("ParseHexColor(%q) = %v, want %v", tt.hex, got, tt.want)
		}
	}
}

func TestStatusDotIcon_ProducesValidPNG(t *testing.T) {
	for _, c := range []struct {
		name string
		dot  func() []byte
	}{
		{"green", func() []byte { return StatusDotIcon(DotGreen) }},
		{"blue", func() []byte { return StatusDotIcon(DotBlue) }},
		{"red", func() []byte { return StatusDotIcon(DotRed) }},
		{"gray", func() []byte { return StatusDotIcon(DotGray) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			data := c.dot()
			if len(data) == 0 {
				t.Fatal("StatusDotIcon returned empty data")
			}
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("StatusDotIcon produced invalid PNG: %v", err)
			}
			bounds := img.Bounds()
			if bounds.Dx() != 16 || bounds.Dy() != 16 {
				t.Errorf("dot icon size = %dx%d, want 16x16", bounds.Dx(), bounds.Dy())
			}
		})
	}
}
