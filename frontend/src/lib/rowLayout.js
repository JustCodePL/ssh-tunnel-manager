/** Tools that can sit in a row or in the expanded panel, in display order. */
export const ROW_TOOLS = [
  { key: "files", tool: "files", label: "Files", shortcut: "f" },
  { key: "logs", tool: "logs", label: "Logs", shortcut: "l" },
  { key: "disk", tool: "disk", label: "Disk", shortcut: "d" },
  { key: "docker", tool: "docker", label: "Docker", shortcut: "o" },
  { key: "htop", tool: "htop", label: "htop", shortcut: "h" },
];

/**
 * @param {string} key a row tool's key
 * @param {{docker?: boolean, htop?: boolean} | undefined} caps detected tools
 * @returns {boolean} whether the host can run this tool
 */
export function toolAvailable(key, caps) {
  if (key === "docker") return !!caps?.docker;
  if (key === "htop") return !!caps?.htop;
  if (key === "disk") return !!caps;
  return true;
}

/**
 * @param {Record<string, string>} row module placement
 * @param {"row" | "panel"} place
 * @param {{docker?: boolean, htop?: boolean} | undefined} caps
 */
export function toolsIn(row, place, caps) {
  return ROW_TOOLS.filter((tool) => {
    if (!toolAvailable(tool.key, caps)) return false;
    const where = row[tool.key] ?? "panel";
    // The panel lists every tool that is not switched off, including those also
    // shown in the row, so expanding never hides anything.
    return place === "row" ? where === "row" : where !== "off";
  });
}

/** @param {string | undefined} error @returns {string} one short line */
export function shortError(error) {
  const line = (error ?? "").split("\n")[0].trim();
  if (!line) return "connection failed";
  return line.length > 80 ? line.slice(0, 79) + "…" : line;
}

/** @returns {boolean} whether a tunnel counts as on for its switch */
export function isOn(status) {
  return status === "connected" || status === "connecting" || status === "reconnecting";
}

/**
 * Groups tunnels for the list: pinned first, then named groups alphabetically,
 * then unassigned.
 * @template {{id: string, name: string, group?: string, pinned?: boolean}} T
 * @param {T[]} tunnels
 * @returns {{key: string, label: string, group: string, tunnels: T[]}[]}
 */
export function groupTunnels(tunnels, emptyGroups = []) {
  const pinned = tunnels.filter((t) => t.pinned);
  const rest = tunnels.filter((t) => !t.pinned);
  const named = new Map();
  const unassigned = [];
  for (const tunnel of rest) {
    if (!tunnel.group) {
      unassigned.push(tunnel);
    } else if (named.has(tunnel.group)) {
      named.get(tunnel.group).push(tunnel);
    } else {
      named.set(tunnel.group, [tunnel]);
    }
  }
  const sections = [];
  if (pinned.length) sections.push({ key: "__pinned__", label: "Pinned", group: "", tunnels: pinned });
  const pinnedGroups = new Set(pinned.map((t) => t.group));
  const extra = emptyGroups.filter((name) => !named.has(name) && !pinnedGroups.has(name));
  for (const name of [...named.keys(), ...extra].sort((a, b) => a.localeCompare(b))) {
    sections.push({ key: name, label: name, group: name, tunnels: named.get(name) ?? [] });
  }
  if (unassigned.length) {
    sections.push({ key: "__unassigned__", label: "Unassigned", group: "", tunnels: unassigned });
  }
  return sections;
}

/**
 * @template {{id: string, name: string, host?: string, user?: string, group?: string, pinned?: boolean, sourceFile?: string}} T
 * @param {T[]} tunnels
 * @param {string} filter "all", "live", "pinned", "unassigned", "group:<name>" or "file:<path>"
 * @param {string} query free text
 * @param {(id: string) => string} statusOf
 * @returns {T[]}
 */
export function filterTunnels(tunnels, filter, query, statusOf) {
  const needle = query.trim().toLowerCase();
  return tunnels.filter((t) => {
    if (filter === "live" && !isOn(statusOf(t.id))) return false;
    if (filter === "pinned" && !t.pinned) return false;
    if (filter === "unassigned" && t.group) return false;
    if (filter.startsWith("group:") && t.group !== filter.slice(6)) return false;
    if (filter.startsWith("file:") && t.sourceFile !== filter.slice(5)) return false;
    if (!needle) return true;
    return [t.name, t.host, t.user, t.group].some((value) => (value ?? "").toLowerCase().includes(needle));
  });
}
