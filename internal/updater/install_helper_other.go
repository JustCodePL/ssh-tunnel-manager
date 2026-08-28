//go:build !darwin

package updater

func handleInstallHelper([]string) (bool, error) {
	return false, nil
}
