//go:build darwin

package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func handleInstallHelper(args []string) (bool, error) {
	if len(args) == 0 || args[0] != InstallHelperArg {
		return false, nil
	}
	if len(args) != 4 {
		return true, fmt.Errorf("%s requires source, current, and target app paths", InstallHelperArg)
	}
	if os.Geteuid() != 0 {
		return true, fmt.Errorf("%s must run with administrator privileges", InstallHelperArg)
	}
	return true, replaceAppBundles(args[1], args[2], args[3])
}

func runElevatedInstall(ctx context.Context, exe, extractedApp, currentAppPath, installTarget string) error {
	script, err := elevatedInstallScript(exe, []string{
		InstallHelperArg,
		extractedApp,
		currentAppPath,
		installTarget,
	})
	if err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script).CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if strings.Contains(trimmed, "User canceled") || strings.Contains(trimmed, "(-128)") {
			return fmt.Errorf("administrator prompt cancelled")
		}
		return fmt.Errorf("osascript: %w (%s)", err, trimmed)
	}
	return nil
}

func elevatedInstallScript(exe string, args []string) (string, error) {
	values := append([]string{exe}, args...)
	parts := make([]string, 0, len(values)*2-1)
	for i, value := range values {
		if strings.ContainsAny(value, "\x00\r\n") {
			return "", fmt.Errorf("update helper argument contains unsupported control characters")
		}
		if i > 0 {
			parts = append(parts, `" "`)
		}
		escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
		parts = append(parts, fmt.Sprintf(`(quoted form of "%s")`, escaped))
	}
	return `do shell script (` + strings.Join(parts, " & ") + `) with administrator privileges`, nil
}
