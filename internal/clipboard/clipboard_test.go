package clipboard

import (
	"bytes"
	"testing"
)

func TestParseAppleScriptPNG(t *testing.T) {
	got, err := parseAppleScriptPNG("«data PNGf89504E470D0A1A0A»\n")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}) || !IsPNG(got) {
		t.Fatalf("got % x", got)
	}
	if got, err := parseAppleScriptPNG("some text"); got != nil || err != nil {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestIsPNG(t *testing.T) {
	if IsPNG([]byte("hello")) {
		t.Fatal("text is not a PNG")
	}
}
