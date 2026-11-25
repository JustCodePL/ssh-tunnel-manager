import { effectiveForwardProtocol } from "./portForwards.js";
import { forwardAddress } from "./forwards.js";

export const NO_GROUP = "No group";

/** @param {string} protocol @returns {string} the tool that opens it */
export function clientTool(protocol) {
  return protocol === "rdp" || protocol === "vnc" ? protocol : "terminal";
}

/**
 * Builds the rows of the Clients tab: remote desktops reached without SSH,
 * forwards with the rdp, vnc or ssh protocol, and what Attach offers on each
 * host. A row through SSH takes its host's group and pin.
 * @param {any[]} tunnels SSH hosts
 * @param {any[]} desktops remote desktops without SSH
 * @param {Record<string, {protocol: string, label: string}[]>} attachOptions by tunnel ID
 * @returns {any[]}
 */
export function clientRows(tunnels, desktops, attachOptions) {
  const rows = [];
  for (const d of desktops ?? []) {
    rows.push({
      key: d.id,
      kind: "desktop",
      protocol: d.protocol,
      name: d.name,
      detail: `${d.address}:${d.port}`,
      route: "",
      group: d.group ?? "",
      pinned: !!d.pinned,
      color: d.color ?? "",
      spec: { tool: clientTool(d.protocol), tunnelId: d.id },
      desktop: d,
    });
  }
  for (const t of tunnels ?? []) {
    for (const option of attachOptions?.[t.id] ?? []) {
      rows.push({
        key: `${t.id}|${option.protocol}`,
        kind: "host",
        protocol: option.protocol,
        name: t.name,
        detail: option.label,
        route: t.name,
        group: t.group ?? "",
        pinned: !!t.pinned,
        color: t.color ?? "",
        spec: { tool: clientTool(option.protocol), tunnelId: t.id },
        tunnel: t,
      });
    }
    (t.portForwards ?? []).forEach((pf, index) => {
      const protocol = effectiveForwardProtocol(pf);
      if (protocol !== "rdp" && protocol !== "vnc" && protocol !== "ssh") return;
      const target = `${pf.remoteHost}:${pf.remotePort}`;
      rows.push({
        // Two forwards can reach the same target, so the position tells them apart.
        key: `${t.id}#${index}`,
        kind: "forward",
        protocol,
        name: pf.description || `${t.name} → ${target}`,
        detail: target,
        route: t.name,
        group: t.group ?? "",
        pinned: !!t.pinned,
        color: t.color ?? "",
        spec: { tool: clientTool(protocol), tunnelId: t.id, remoteHost: pf.remoteHost, remotePort: pf.remotePort },
        tunnel: t,
      });
    });
  }
  return rows;
}

/**
 * Builds the rows of the Web tab: portless forwards, which have a name the
 * browser can open. Forwards with only a local port are left out, because what
 * they serve cannot be told from the port.
 * @param {any[]} tunnels
 * @returns {any[]}
 */
export function webRows(tunnels) {
  const rows = [];
  for (const t of tunnels ?? []) {
    (t.portForwards ?? []).forEach((pf, index) => {
      if (!pf.portless || !pf.domain) return;
      const address = forwardAddress(pf);
      rows.push({
        key: `${t.id}#${index}`,
        name: pf.description || address.replace(/^https?:\/\//, ""),
        url: /^https?:\/\//.test(address) ? address : `http://${address}`,
        route: t.name,
        group: t.group ?? "",
        pinned: !!t.pinned,
        color: t.color ?? "",
        tunnelId: t.id,
      });
    });
  }
  return rows;
}

/**
 * @param {any[]} rows
 * @param {string} query
 * @returns {any[]} rows whose name, detail, route or address match
 */
export function filterRows(rows, query) {
  const q = (query ?? "").trim().toLowerCase();
  if (!q) return rows;
  return rows.filter((row) =>
    [row.name, row.detail, row.route, row.url, row.group].some((value) => (value ?? "").toLowerCase().includes(q)),
  );
}

/**
 * Groups rows by their group, in alphabetical order with the rows without a
 * group last. Inside a group pinned rows come first, then by name.
 * @param {any[]} rows
 * @returns {{key: string, label: string, rows: any[]}[]}
 */
export function groupRows(rows) {
  const groups = new Map();
  for (const row of rows) {
    const key = row.group || "";
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(row);
  }
  const byName = (a, b) => (a.pinned === b.pinned ? a.name.localeCompare(b.name) || a.key.localeCompare(b.key) : a.pinned ? -1 : 1);
  return [...groups.entries()]
    .sort(([a], [b]) => (a === "" ? 1 : b === "" ? -1 : a.localeCompare(b)))
    .map(([key, list]) => ({ key: key || "__none__", label: key || NO_GROUP, rows: [...list].sort(byName) }));
}

/**
 * @param {string} protocol
 * @returns {string} the icon of a client row
 */
export function clientIcon(protocol) {
  if (protocol === "rdp") return "rdp";
  if (protocol === "vnc") return "vnc";
  return "terminal";
}

/**
 * @param {string | number} port
 * @param {string} protocol
 * @returns {number} the port to save: the typed one, or the protocol's standard one
 */
export function desktopPort(port, protocol) {
  const n = Number(port);
  if (Number.isInteger(n) && n >= 1 && n <= 65535) return n;
  return protocol === "vnc" ? 5900 : 3389;
}
