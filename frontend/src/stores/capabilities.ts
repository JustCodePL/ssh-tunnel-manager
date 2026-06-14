import { writable, get } from "svelte/store";
import { GetSavedCapabilities, GetServerCapabilities, VerifyTool } from "../../wailsjs/go/main/App";

export type ToolName = "docker" | "htop";
export type ToolCaps = { docker: boolean; htop: boolean };

// capabilities maps a tunnel ID to the optional tools known on its host. It is
// seeded from persisted prefs at startup, filled on first connect, and updated
// whenever a tool is re-verified on click.
export const capabilities = writable<Record<string, ToolCaps>>({});

/** loadSavedCapabilities seeds the store from persisted prefs (call once). */
export async function loadSavedCapabilities(): Promise<void> {
  try {
    const saved = await GetSavedCapabilities();
    const m: Record<string, ToolCaps> = {};
    for (const [id, c] of Object.entries(saved ?? {})) {
      m[id] = { docker: !!(c as any).docker, htop: !!(c as any).htop };
    }
    capabilities.set(m);
  } catch (e) {
    console.error("failed to load saved capabilities:", e);
  }
}

/**
 * ensureCapabilities probes + persists a host's tools the first time we connect
 * to it. If we already have a record (persisted or freshly probed) it does
 * nothing, so the buttons stay stable across reconnects.
 */
export async function ensureCapabilities(tunnelId: string): Promise<void> {
  if (get(capabilities)[tunnelId]) return;
  try {
    const c = await GetServerCapabilities(tunnelId);
    capabilities.update((m) => ({ ...m, [tunnelId]: { docker: c.docker, htop: c.htop } }));
  } catch (e) {
    // Connection still settling — leave it unprobed; we'll try next connect.
  }
}

export type VerifyResult = "present" | "absent" | "error";

/**
 * verifyTool re-checks a tool with `command -v` when its button is clicked and
 * syncs the store. "absent" means the probe ran and the tool is gone (host
 * reinstalled); "error" means the probe couldn't run (connection issue) and
 * must NOT be treated as the tool being missing.
 */
export async function verifyTool(tunnelId: string, tool: ToolName): Promise<VerifyResult> {
  try {
    const present = await VerifyTool(tunnelId, tool);
    capabilities.update((m) => {
      const rec: ToolCaps = { docker: false, htop: false, ...(m[tunnelId] ?? {}) };
      rec[tool] = present;
      return { ...m, [tunnelId]: rec };
    });
    return present ? "present" : "absent";
  } catch (e) {
    return "error";
  }
}
