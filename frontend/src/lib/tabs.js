const NAMES = {
  terminal: "Terminal",
  rdp: "Remote desktop",
  vnc: "Screen sharing",
  files: "Files",
  editor: "Editor",
  docker: "Docker",
  htop: "htop",
  disk: "Disk",
  logs: "Logs",
  settings: "Settings",
  "tunnel-settings": "Settings",
};

/**
 * @param {{tool: string, tunnelId?: string, remoteHost?: string, remotePort?: number, path?: string}} spec
 * @returns {string} the title of a tab or window
 */
export function tabTitle(spec) {
  const name = NAMES[spec.tool] ?? spec.tool;
  if (spec.tool === "editor" && spec.path) {
    return spec.path.split("/").pop() || spec.path;
  }
  if (!spec.tunnelId) return name;
  const id = spec.tunnelId.startsWith("rd:") ? spec.tunnelId.slice(3) : spec.tunnelId;
  const target = spec.remoteHost ? `${id} to ${spec.remoteHost}:${spec.remotePort}` : id;
  return `${name}: ${target}`;
}
