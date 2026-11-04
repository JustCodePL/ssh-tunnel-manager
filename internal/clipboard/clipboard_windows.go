//go:build windows

package clipboard

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

const readScript = `Add-Type -AssemblyName System.Windows.Forms,System.Drawing;` +
	`$i=[Windows.Forms.Clipboard]::GetImage();if($i -ne $null){$m=New-Object IO.MemoryStream;` +
	`$i.Save($m,[Drawing.Imaging.ImageFormat]::Png);[Convert]::ToBase64String($m.ToArray())}`

const writeScript = `Add-Type -AssemblyName System.Windows.Forms,System.Drawing;` +
	`$b=[Convert]::FromBase64String([Console]::In.ReadToEnd());$m=New-Object IO.MemoryStream(,$b);` +
	`[Windows.Forms.Clipboard]::SetImage([Drawing.Image]::FromStream($m))`

func powershell(script string) *exec.Cmd {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

// ReadImage returns the PNG on the clipboard, or nil when there is none.
func ReadImage() ([]byte, error) {
	out, err := powershell(readScript).Output()
	if err != nil {
		return nil, fmt.Errorf("reading clipboard image: %w", err)
	}
	encoded := strings.TrimSpace(string(out))
	if encoded == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(encoded)
}

// WriteImage puts a PNG on the clipboard.
func WriteImage(data []byte) error {
	cmd := powershell(writeScript)
	cmd.Stdin = strings.NewReader(base64.StdEncoding.EncodeToString(data))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("setting clipboard image: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
