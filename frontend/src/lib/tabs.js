const NAMES = {
  terminal: "Terminal",
  rdp: "Remote desktop",
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
  const target = spec.remoteHost ? `${spec.tunnelId} to ${spec.remoteHost}:${spec.remotePort}` : spec.tunnelId;
  return `${name}: ${target}`;
}
