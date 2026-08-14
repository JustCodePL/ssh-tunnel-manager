// Package netcap deals with the Linux CAP_NET_BIND_SERVICE capability that
// Portless needs to bind privileged ports without running as root. macOS uses
// a PF redirect to an unprivileged listener instead; Windows binds directly.
package netcap

// CapName is the capability portless mode requires to bind privileged ports.
const CapName = "cap_net_bind_service"
