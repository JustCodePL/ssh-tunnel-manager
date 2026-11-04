package clipboard

import (
	"bytes"
	"encoding/base64"
	"os"
	"testing"
)

// This test replaces the machine's clipboard, so it runs only on request:
// STM_CLIPBOARD_TEST=1 go test ./internal/clipboard
func TestWriteThenReadImageOnThisMachine(t *testing.T) {
	if os.Getenv("STM_CLIPBOARD_TEST") != "1" {
		t.Skip("set STM_CLIPBOARD_TEST=1 to touch the real clipboard")
	}
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M/wHwAEBgIApD5fRAAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteImage(png); err != nil {
		t.Fatal(err)
	}
	got, err := ReadImage()
	if err != nil {
		t.Fatal(err)
	}
	if !IsPNG(got) {
		t.Fatalf("read back %d bytes that are not a PNG", len(got))
	}
	if !bytes.Equal(got[:8], png[:8]) {
		t.Fatalf("signature differs: % x", got[:8])
	}
}
