import { get, writable, derived } from "svelte/store";
import type { TunnelConfig, TunnelStatus, StatusEvent, ConfigFileInfo } from "../types";
import { AddGroup, GetGroups, RemoveGroup, GetTunnels, GetTunnelStatuses, AddTunnel, UpdateTunnel, DeleteTunnel, ConnectTunnel, DisconnectTunnel, SelectFile, ImportPreview, ImportTunnels, SelectSaveFile, ExportTunnels, SetTunnelPinned, ConnectGroup, DisconnectGroup, RenameGroup, GetIncludedConfigFiles } from "../../bindings/ssh-tunnel-manager/app";
import { EventsOn } from "../lib/runtime";
import { showToast } from "./toast";

export const tunnels = writable<TunnelConfig[]>([]);
export const statuses = writable<Record<string, StatusEvent>>({});
export const loading = writable(true);
/** Groups created by hand. Others exist because a tunnel names them. */
export const groups = writable<string[]>([]);
export const includedConfigFiles = writable<ConfigFileInfo[]>([]);

export async function loadTunnels() {
  loading.set(true);
  try {
    const [cfgs, sts, includes, saved] = await Promise.all([
      GetTunnels(),
      GetTunnelStatuses(),
      GetIncludedConfigFiles(),
      GetGroups(),
    ]);
    groups.set(saved ?? []);
    tunnels.set((cfgs || []) as unknown as TunnelConfig[]);
    statuses.set((sts || {}) as unknown as Record<string, StatusEvent>);
    includedConfigFiles.set((includes || []) as unknown as ConfigFileInfo[]);
  } finally {
    loading.set(false);
  }
}

export async function addTunnel(t: TunnelConfig): Promise<void> {
  await AddTunnel(t as any);
  await loadTunnels();
}

export async function updateTunnel(t: TunnelConfig): Promise<void> {
  await UpdateTunnel(t as any);
  await loadTunnels();
}

export async function deleteTunnel(id: string): Promise<void> {
  await DeleteTunnel(id);
  await loadTunnels();
}

export async function connectTunnel(id: string): Promise<void> {
  await ConnectTunnel(id);
}

export async function disconnectTunnel(id: string): Promise<void> {
  await DisconnectTunnel(id);
}

export async function setTunnelPinned(id: string, pinned: boolean): Promise<void> {
  await SetTunnelPinned(id, pinned);
  await loadTunnels();
}

export async function connectGroup(group: string): Promise<void> {
  await ConnectGroup(group);
}

export async function disconnectGroup(group: string): Promise<void> {
  await DisconnectGroup(group);
}

export async function addGroup(name: string): Promise<void> {
  await AddGroup(name);
  await loadTunnels();
}

export async function removeGroup(name: string): Promise<void> {
  await RemoveGroup(name);
  await loadTunnels();
}

export function disconnectAll(): void {
  const current = get(statuses);
  for (const t of get(tunnels)) {
    const status = current[t.id]?.status;
    if (status === "connected" || status === "connecting" || status === "reconnecting") void DisconnectTunnel(t.id);
  }
}

export async function renameGroup(oldName: string, newName: string): Promise<void> {
  await RenameGroup(oldName, newName);
}

export function getStatus(
  statusMap: Record<string, StatusEvent>,
  tunnelId: string
): TunnelStatus {
  return statusMap[tunnelId]?.status ?? "disconnected";
}

export async function selectImportFile(): Promise<string> {
  return await SelectFile();
}

export async function previewImport(path: string): Promise<TunnelConfig[]> {
  return ((await ImportPreview(path)) || []) as unknown as TunnelConfig[];
}

export async function importTunnels(path: string, names: string[]): Promise<number> {
  const count = await ImportTunnels(path, names);
  await loadTunnels();
  return count;
}

export async function exportTunnels(ids: string[]): Promise<void> {
  const path = await SelectSaveFile("tunnels-export");
  if (!path) return;
  await ExportTunnels(path, ids);
}

/** Tool windows pass notify=false: the main window already reports errors. */
export function initEventListeners(notify = true) {
  EventsOn("tunnel:status-changed", (event: StatusEvent) => {
    statuses.update((s) => ({
      ...s,
      [event.tunnelId]: event,
    }));
  });
  EventsOn("tunnel:error", (event: StatusEvent) => {
    if (!notify) return;
    const name = get(tunnels).find((t) => t.id === event.tunnelId)?.name ?? event.tunnelId;
    showToast(`${name}: ${event.error ?? "connection failed"}`, "error", 6000);
  });
  EventsOn("tunnels:changed", () => {
    loadTunnels();
  });
}
