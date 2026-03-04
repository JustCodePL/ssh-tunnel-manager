//go:build windows

package tray

import (
	"bytes"
	"encoding/binary"
)

func init() {
	iconSize = 32
}

// wrapInICO wraps raw PNG bytes in a minimal ICO container of the given size.
// On Windows, energye/systray writes icon bytes to a temp file and loads it
// with LoadImageW(IMAGE_ICON), which requires ICO format — not plain PNG.
// Windows Vista+ supports PNG-inside-ICO natively.
func wrapInICO(pngBytes []byte, size int) []byte {
	var buf bytes.Buffer

	// ICO file header (6 bytes)
	binary.Write(&buf, binary.LittleEndian, uint16(0)) // idReserved
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // idType: 1 = ICO
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // idCount: 1 image

	// Image directory entry (16 bytes)
	buf.WriteByte(byte(size)) // bWidth  (0 = 256)
	buf.WriteByte(byte(size)) // bHeight (0 = 256)
	buf.WriteByte(0)          // bColorCount (0 = true color)
	buf.WriteByte(0)          // bReserved
	binary.Write(&buf, binary.LittleEndian, uint16(1))             // wPlanes
	binary.Write(&buf, binary.LittleEndian, uint16(32))            // wBitCount
	binary.Write(&buf, binary.LittleEndian, uint32(len(pngBytes))) // dwBytesInRes
	binary.Write(&buf, binary.LittleEndian, uint32(6+16))          // dwImageOffset

	// PNG data
	buf.Write(pngBytes)

	return buf.Bytes()
}

// wrapIconBytes wraps the main tray icon PNG (32x32) in an ICO container.
func wrapIconBytes(pngBytes []byte) []byte { return wrapInICO(pngBytes, iconSize) }

// wrapDotIconBytes wraps menu-item dot icon PNG (16x16) in an ICO container.
func wrapDotIconBytes(pngBytes []byte) []byte { return wrapInICO(pngBytes, 16) }
