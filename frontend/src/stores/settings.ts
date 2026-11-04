import { writable, get } from "svelte/store";
import {
  GetAppSettings,
  GetEffectiveSettings,
  GetAttachOptions,
  SetAppSettings,
} from "../../bindings/ssh-tunnel-manager/app";
import { EventsOn } from "../lib/runtime";

export interface AppSettings {
  theme: string;
  windowGrouping: string;
  afterDisconnect: string;
  hideToolsWithMain: boolean;
  refresh: Record<string, number>;
  row: Record<string, string>;
}

export interface EffectiveSettings {
  afterDisconnect: string;
  refresh: Record<string, number>;
  row: Record<string, string>;
  clipboard: string;
}

const fallback: EffectiveSettings = {
  afterDisconnect: "resume",
  refresh: { stats: 3, htop: 3, disk: 5, docker: 0 },
  row: { attach: "row", forwards: "row" },
  clipboard: "both",
};

export const appSettings = writable<AppSettings | null>(null);
export const effective = writable<Record<string, EffectiveSettings>>({});

export function applyTheme(theme: string): void {
  document.documentElement.dataset.theme = theme === "graphite" ? "graphite" : "terminal";
}

export async function loadSettings(): Promise<void> {
  const [app, all] = await Promise.all([GetAppSettings(), GetEffectiveSettings()]);
  appSettings.set(app as unknown as AppSettings);
  effective.set(all as unknown as Record<string, EffectiveSettings>);
  applyTheme((app as any).theme);
}

export function settingsFor(map: Record<string, EffectiveSettings>, id: string): EffectiveSettings {
  return map[id] ?? fallback;
}

export async function saveAppSettings(change: Partial<AppSettings>): Promise<void> {
  const current = get(appSettings);
  if (!current) return;
  await SetAppSettings({ ...current, ...change } as any);
  await loadSettings();
}

let listening = false;

/** initSettings loads settings and keeps them current in this window. */
export async function initSettings(): Promise<void> {
  if (!listening) {
    listening = true;
    EventsOn("settings:changed", () => {
      void loadSettings();
    });
  }
  await loadSettings();
}

export async function attachOptionsFor(id: string): Promise<{ protocol: string; label: string }[]> {
  return ((await GetAttachOptions(id)) ?? []) as any;
}
