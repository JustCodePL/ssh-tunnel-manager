package dns

import "context"

// SystemServiceStatus is safe to expose through Wails. The service is
// machine-wide, while this snapshot is read by whichever user is running the
// GUI.
type SystemServiceStatus struct {
	Available        bool   `json:"available"`
	Installed        bool   `json:"installed"`
	ApprovalRequired bool   `json:"approvalRequired"`
	Current          bool   `json:"current"`
	State            string `json:"state"`
	Message          string `json:"message"`
	// Problem means Portless may not work here until something is done. A setup
	// that has not happened yet is no problem: the first Portless forward asks.
	Problem bool `json:"problem"`
}

// GetSystemServiceStatus returns the state of what Portless set up on this system.
func GetSystemServiceStatus() SystemServiceStatus {
	status := getSystemServiceStatus()
	status.Problem = status.Problem || detectProblem(status)
	return status
}

func detectProblem(status SystemServiceStatus) bool {
	switch status.State {
	case "unsupported", "unavailable":
		return true
	}
	return status.ApprovalRequired || (status.Installed && !status.Current)
}

func InstallSystemService(ctx context.Context) error {
	return installSystemService(ctx)
}

func UninstallSystemService(ctx context.Context) error {
	return uninstallSystemService(ctx)
}

func OpenSystemServiceSettings() error {
	return openSystemServiceSettings()
}
