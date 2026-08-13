/** @typedef {import("../types").PortForward} PortForward */

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
    }))
    .filter((pf) => pf.remotePort > 0 && (pf.portless || pf.localPort > 0));
}
