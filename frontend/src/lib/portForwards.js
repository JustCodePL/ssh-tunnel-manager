/** @typedef {import("../types").PortForward} PortForward */

/**
 * @param {number | string | undefined} remotePort
 * @returns {"rdp" | "ssh" | "none"} the protocol implied by a standard port
 */
export function inferForwardProtocol(remotePort) {
  const port = Number(remotePort);
  if (port === 3389) return "rdp";
  if (port === 22) return "ssh";
  return "none";
}

/**
 * @param {PortForward} pf
 * @returns {string} the explicit protocol, or the one inferred from the port
 */
export function effectiveForwardProtocol(pf) {
  return pf.protocol || inferForwardProtocol(pf.remotePort);
}

/**
 * Normalize forward values from form inputs and discard only rows that cannot
 * be saved. Portless forwards may leave localPort empty: the SSH config writer
 * uses remotePort as the plain-SSH fallback in that case.
 *
 * @param {PortForward[]} portForwards
 * @returns {PortForward[]}
 */
export function preparePortForwardsForSave(portForwards) {
  return portForwards
    .map((pf) => ({
      ...pf,
      localPort: Number(pf.localPort) || 0,
      remotePort: Number(pf.remotePort) || 0,
      exposePort: Number(pf.exposePort) || 0,
      hostHeader: (pf.hostHeader ?? "").trim() || undefined,
      hostHeaderOn: pf.hostHeaderOn ? true : undefined,
      // A choice that matches the port stays implicit, so it keeps following the
      // port and the SSH config stays free of redundant metadata.
      protocol: effectiveForwardProtocol(pf) === inferForwardProtocol(pf.remotePort) ? undefined : pf.protocol,
      sshUser: effectiveForwardProtocol(pf) === "ssh" ? (pf.sshUser ?? "").trim() || undefined : undefined,
    }))
    .filter((pf) => pf.remotePort > 0 && (pf.portless || pf.localPort > 0));
}

/**
 * @param {string | undefined} suffix what the user typed
 * @returns {string} the suffix without surrounding dots and spaces
 */
export function normalizePortlessSuffix(suffix) {
  return (suffix ?? "").trim().toLowerCase().replace(/^\.+|\.+$/g, "");
}

/**
 * @param {string | undefined} domain a saved domain, which includes the suffix
 * @param {string} suffix
 * @returns {string} the part of the domain that is edited for one forward
 */
export function domainWithoutSuffix(domain, suffix) {
  const value = domain ?? "";
  return suffix && value.toLowerCase().endsWith(`.${suffix}`) ? value.slice(0, -suffix.length - 1) : value;
}

/**
 * @param {string | undefined} name the part edited for one forward
 * @param {string} suffix
 * @returns {string} the domain to save
 */
export function domainWithSuffix(name, suffix) {
  const value = (name ?? "").trim().toLowerCase();
  return suffix && value ? `${value}.${suffix}` : value;
}
