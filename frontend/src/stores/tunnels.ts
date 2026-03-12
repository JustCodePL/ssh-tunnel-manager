import { writable, derived } from "svelte/store";
import type { TunnelConfig, TunnelStatus, StatusEvent } from "../types";
import { GetTunnels, GetTunnelStatuses, AddTunnel, UpdateTunnel, DeleteTunnel, ConnectTunnel, DisconnectTunnel, SelectFile, ImportPreview, ImportTunnels, SelectSaveFile, ExportTunnels, SetTunnelPinned, ConnectGroup, DisconnectGroup, RenameGroup } from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";

export const tunnels = writable<TunnelConfig[]>([]);
export const statuses = writable<Record<string, StatusEvent>>({});
export const loading = writable(true);

export async function loadTunnels() {
  loading.set(true);
  try {
    const [cfgs, sts] = await Promise.all([GetTunnels(), GetTunnelStatuses()]);
    tunnels.set(cfgs || []);
    statuses.set(sts || {});
  } finally {
    loading.set(false);
  }
}

export async function addTunnel(t: TunnelConfig): Promise<void> {
  await AddTunnel(t);
  await loadTunnels();
}

export async function updateTunnel(t: TunnelConfig): Promise<void> {
  await UpdateTunnel(t);
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
  return (await ImportPreview(path)) || [];
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

export function initEventListeners() {
  EventsOn("tunnel:status-changed", (event: StatusEvent) => {
    statuses.update((s) => ({
      ...s,
      [event.tunnelId]: event,
    }));
  });
  EventsOn("tunnels:changed", () => {
    loadTunnels();
  });
}
