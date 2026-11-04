// Package clipboard reads and writes images on the system clipboard. Wails
// handles text only, and a webview cannot reach the clipboard from a background
// timer, so remote desktop sessions use this for pictures.
package clipboard

import (
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

// ErrUnavailable means this system has no tool for clipboard images.
var ErrUnavailable = errors.New("no clipboard image tool available")

// MaxImageSize limits what is carried between clipboards.
const MaxImageSize = 32 << 20

var appleScriptPNG = regexp.MustCompile(`(?i)«data PNGf([0-9a-f]+)»`)

// parseAppleScriptPNG extracts the PNG from osascript's `«data PNGf...»` output.
func parseAppleScriptPNG(output string) ([]byte, error) {
	match := appleScriptPNG.FindStringSubmatch(strings.TrimSpace(output))
	if match == nil {
		return nil, nil
	}
	return hex.DecodeString(match[1])
}

// IsPNG reports whether data starts with the PNG signature.
func IsPNG(data []byte) bool {
	const signature = "\x89PNG\r\n\x1a\n"
	return len(data) >= len(signature) && string(data[:len(signature)]) == signature
}
