// Package netcap deals with the Linux CAP_NET_BIND_SERVICE capability that
// portless mode needs in order to bind privileged ports (< 1024) on loopback
// without running as root.
//
// On macOS and Windows there is no such restriction, so every function here is
// a no-op / "always allowed" stub (see netcap_other.go). The real logic lives
// in netcap_linux.go.
package netcap

// CapName is the capability portless mode requires to bind privileged ports.
const CapName = "cap_net_bind_service"
