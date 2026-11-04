//go:build darwin

package clipboard

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ReadImage returns the PNG on the clipboard, or nil when there is none.
func ReadImage() ([]byte, error) {
	out, err := exec.Command("osascript", "-e", "the clipboard as «class PNGf»").Output()
	if err != nil {
		// osascript fails when the clipboard holds no picture.
		return nil, nil
	}
	return parseAppleScriptPNG(string(out))
}

// WriteImage puts a PNG on the clipboard.
func WriteImage(data []byte) error {
	file, err := os.CreateTemp("", "stm-clipboard-*.png")
	if err != nil {
		return fmt.Errorf("creating clipboard file: %w", err)
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("writing clipboard file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("writing clipboard file: %w", err)
	}
	path := strings.ReplaceAll(file.Name(), `"`, `\"`)
	script := fmt.Sprintf(`set the clipboard to (read (POSIX file "%s") as «class PNGf»)`, path)
	if out, err := exec.Command("osascript", "-e", script).CombinedOutput(); err != nil {
		return fmt.Errorf("setting clipboard image: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
