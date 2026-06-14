import { writable } from "svelte/store";
import { GetShowResourceStats, SetShowResourceStats } from "../../wailsjs/go/main/App";

// showResourceStats mirrors the backend pref for the inline CPU/RAM widget.
// Cards subscribe to it so the settings toggle takes effect live.
export const showResourceStats = writable<boolean>(true);

let loaded = false;

/** loadShowResourceStats fetches the persisted value once. */
export async function loadShowResourceStats(): Promise<void> {
  if (loaded) return;
  loaded = true;
  try {
    showResourceStats.set(await GetShowResourceStats());
  } catch (e) {
    console.error("failed to load showResourceStats:", e);
  }
}

/** setShowResourceStats persists the new value and updates the store. */
export async function setShowResourceStats(value: boolean): Promise<void> {
  await SetShowResourceStats(value);
  showResourceStats.set(value);
}
