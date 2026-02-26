//go:build windows

package tray

import (
	"log/slog"
	"os/exec"
)

// notify sends a desktop notification on Windows via PowerShell.
func notify(title, body string) {
	script := `
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$textNodes = $template.GetElementsByTagName('text')
$textNodes.Item(0).AppendChild($template.CreateTextNode('` + title + `')) | Out-Null
$textNodes.Item(1).AppendChild($template.CreateTextNode('` + body + `')) | Out-Null
$toast = [Windows.UI.Notifications.ToastNotification]::new($template)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('SSH Tunnel Manager').Show($toast)
`
	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	if err := cmd.Start(); err != nil {
		slog.Debug("powershell notification failed", "error", err)
		return
	}
	go func() { _ = cmd.Wait() }()
}
