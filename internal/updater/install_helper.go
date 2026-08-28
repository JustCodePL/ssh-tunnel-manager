package updater

// InstallHelperArg selects the one-shot privileged macOS update helper. The
// main package checks this before initializing the GUI and exits immediately
// after the helper finishes.
const InstallHelperArg = "--install-update"

// HandleInstallHelper handles a platform-specific one-shot update invocation.
// It returns handled=false during an ordinary application launch.
func HandleInstallHelper(args []string) (handled bool, err error) {
	return handleInstallHelper(args)
}
