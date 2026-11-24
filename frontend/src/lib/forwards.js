import { effectiveForwardProtocol } from "./portForwards.js";

export const PORTLESS_TLD = "ssh-local";

/**
 * @param {import("../types").PortForward} pf
 * @returns {string} the address a forward is reached at locally
 */
export function forwardAddress(pf) {
  if (!pf.portless || !pf.domain) return `127.0.0.1:${pf.localPort}`;
  const port = pf.exposePort && pf.exposePort > 0 ? pf.exposePort : pf.remotePort;
  const host = `${pf.domain}.${PORTLESS_TLD}`;
  if (port === 80) return `http://${host}`;
  if (port === 443) return `https://${host}`;
  return `${host}:${port}`;
}

/**
 * Attach never expands on a forward: it has one type, so one session.
 * @param {import("../types").PortForward} pf
 * @returns {{tool: string, label: string} | null}
 */
export function forwardAttach(pf) {
  const protocol = effectiveForwardProtocol(pf);
  if (protocol === "rdp") return { tool: "rdp", label: "Remote desktop" };
  if (protocol === "vnc") return { tool: "vnc", label: "Screen sharing" };
  if (protocol === "ssh") return { tool: "terminal", label: "Terminal" };
  return null;
}

/** @param {import("../types").PortForward} pf @returns {string[]} labels to show */
export function forwardTags(pf) {
  const tags = [];
  if (pf.portless) tags.push("portless");
  const protocol = effectiveForwardProtocol(pf);
  if (protocol === "rdp") tags.push("RDP");
  if (protocol === "vnc") tags.push("VNC");
  if (protocol === "ssh") tags.push("SSH");
  return tags;
}

/**
 * @param {{protocol: string, label: string}[]} options what a tunnel can open
 * @returns {boolean} whether Attach should show a menu instead of acting at once
 */
export function attachExpands(options) {
  return options.length > 1;
}

/** @param {string} protocol @returns {string} the tool that serves it */
export function toolForProtocol(protocol) {
  return protocol === "rdp" || protocol === "vnc" ? protocol : "terminal";
}

/** @param {string} tool @returns {string} the icon shown for it */
export function toolIcon(tool) {
  return tool === "rdp" || tool === "vnc" ? "rdp" : "terminal";
}

/** @param {string} protocol @returns {string} the key that opens it from a tunnel row */
export function attachKey(protocol) {
  if (protocol === "rdp") return "R";
  if (protocol === "vnc") return "V";
  return "T";
}
