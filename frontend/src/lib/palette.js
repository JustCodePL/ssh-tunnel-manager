import { ROW_TOOLS, toolAvailable, isOn } from "./rowLayout.js";
import { forwardAddress, forwardAttach } from "./forwards.js";

/**
 * Builds every palette action from the current state. Actions carry plain data
 * and the app decides how to run them.
 * @param {object} state
 * @param {any[]} state.tunnels
 * @param {(id: string) => string} state.statusOf
 * @param {Record<string, any>} state.capabilities
 * @param {(id: string) => {protocol: string, label: string}[]} state.attachOptions
 * @returns {{id: string, label: string, hint?: string, run: any}[]}
 */
export function buildActions({ tunnels, statusOf, capabilities, attachOptions }) {
  const actions = [
    { id: "app:new", label: "New tunnel", run: { kind: "new-tunnel" } },
    { id: "app:new-group", label: "New group", run: { kind: "new-group" } },
    { id: "app:settings", label: "Open settings", run: { kind: "tool", spec: { tool: "settings", tunnelId: "" } } },
    { id: "app:import", label: "Import from SSH config", run: { kind: "import" } },
    { id: "app:export", label: "Export tunnels", run: { kind: "export" } },
    { id: "app:disconnect-all", label: "Disconnect all tunnels", run: { kind: "disconnect-all" } },
  ];
  const groups = new Set();
  for (const t of tunnels) {
    if (t.group) groups.add(t.group);
    const on = isOn(statusOf(t.id));
    actions.push({
      id: `toggle:${t.id}`,
      label: `${on ? "Disconnect" : "Connect"} ${t.name}`,
      run: { kind: on ? "disconnect" : "connect", id: t.id },
    });
    for (const option of attachOptions(t.id)) {
      const rdp = option.protocol === "rdp";
      actions.push({
        id: `attach:${t.id}:${option.protocol}`,
        label: `${rdp ? "Remote desktop on" : "Terminal on"} ${t.name}`,
        hint: "term attach",
        run: { kind: "tool", spec: { tool: rdp ? "rdp" : "terminal", tunnelId: t.id } },
      });
    }
    for (const tool of ROW_TOOLS) {
      if (!toolAvailable(tool.key, capabilities[t.id])) continue;
      actions.push({
        id: `tool:${t.id}:${tool.key}`,
        label: `${tool.label} on ${t.name}`,
        run: { kind: "tool", spec: { tool: tool.tool, tunnelId: t.id } },
      });
    }
    actions.push({
      id: `settings:${t.id}`,
      label: `Settings of ${t.name}`,
      run: { kind: "tool", spec: { tool: "tunnel-settings", tunnelId: t.id } },
    });
    for (const pf of t.portForwards ?? []) {
      const address = forwardAddress(pf);
      actions.push({
        id: `copy:${t.id}:${address}`,
        label: `Copy ${address}`,
        hint: t.name,
        run: { kind: "copy", text: address },
      });
      const attach = forwardAttach(pf);
      if (attach) {
        actions.push({
          id: `attach:${t.id}:${pf.remoteHost}:${pf.remotePort}`,
          label: `${attach.label} on ${pf.remoteHost}:${pf.remotePort}`,
          hint: `via ${t.name}`,
          run: {
            kind: "tool",
            spec: { tool: attach.tool, tunnelId: t.id, remoteHost: pf.remoteHost, remotePort: pf.remotePort },
          },
        });
      }
    }
  }
  for (const group of groups) {
    actions.push({ id: `group-on:${group}`, label: `Connect group ${group}`, run: { kind: "connect-group", group } });
    actions.push({ id: `group-off:${group}`, label: `Disconnect group ${group}`, run: { kind: "disconnect-group", group } });
  }
  return actions;
}

/**
 * Every word of the query must appear in the action's label or hint. Earlier
 * matches rank first, so "term g146" finds the terminal before the disk.
 * @template {{label: string, hint?: string}} A
 * @param {A[]} actions
 * @param {string} query
 * @param {number} [limit]
 * @returns {A[]}
 */
export function searchActions(actions, query, limit = 12) {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return actions.slice(0, limit);
  const scored = [];
  for (const action of actions) {
    const text = `${action.label} ${action.hint ?? ""}`.toLowerCase();
    let score = 0;
    let matched = true;
    for (const word of words) {
      const at = text.indexOf(word);
      if (at < 0) {
        matched = false;
        break;
      }
      score += at;
    }
    if (matched) scored.push({ action, score });
  }
  scored.sort((a, b) => a.score - b.score || a.action.label.localeCompare(b.action.label));
  return scored.slice(0, limit).map((entry) => entry.action);
}
