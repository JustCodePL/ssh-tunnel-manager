<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from "svelte";
  import { GetAutostart, SetAutostart, GetCloseToTray, SetCloseToTray, GetCurrentVersion, CheckForUpdate, InstallUpdate } from "../../wailsjs/go/main/App";
  import { EventsOn } from "../../wailsjs/runtime/runtime";
  import { showResourceStats, setShowResourceStats } from "../stores/prefs";

  const dispatch = createEventDispatcher<{ close: void }>();

  let autostartEnabled = false;
  let autostartLoading = true;
  let closeToTray = true;
  let closeToTrayLoading = true;

  let currentVersion = "";
  let updateInfo: { latestVersion: string; releaseUrl: string; assetUrl: string; releaseNotes: string } | null = null;
  let checkingUpdate = false;
  let installingUpdate = false;
  let updateError = "";
  let unsubUpdate: (() => void) | undefined;

  onMount(async () => {
    try {
      autostartEnabled = await GetAutostart();
    } catch (e) {
      console.error("Failed to get autostart status:", e);
    } finally {
      autostartLoading = false;
    }

    try {
      closeToTray = await GetCloseToTray();
    } catch (e) {
      console.error("Failed to get close-to-tray setting:", e);
    } finally {
      closeToTrayLoading = false;
    }

    try {
      currentVersion = await GetCurrentVersion();
    } catch (e) {
      console.error("Failed to get current version:", e);
    }

    unsubUpdate = EventsOn("updater:update-available", (info: any) => {
      updateInfo = info;
    });
  });

  onDestroy(() => {
    unsubUpdate?.();
  });

  async function toggleAutostart() {
    const newValue = !autostartEnabled;
    try {
      await SetAutostart(newValue);
      autostartEnabled = newValue;
    } catch (e: any) {
      console.error("Failed to set autostart:", e);
    }
  }

  async function toggleCloseToTray() {
    const newValue = !closeToTray;
    try {
      await SetCloseToTray(newValue);
      closeToTray = newValue;
    } catch (e: any) {
      console.error("Failed to set close-to-tray:", e);
    }
  }

  async function toggleResourceStats() {
    const newValue = !$showResourceStats;
    try {
      await setShowResourceStats(newValue);
    } catch (e: any) {
      console.error("Failed to set resource stats:", e);
    }
  }

  async function checkForUpdate() {
    checkingUpdate = true;
    updateError = "";
    try {
      const info = await CheckForUpdate();
      if (info) {
        updateInfo = info as any;
      } else {
        updateError = "// you are up to date";
      }
    } catch (e: any) {
      updateError = e?.toString() ?? "update check failed";
    } finally {
      checkingUpdate = false;
    }
  }

  async function installUpdate() {
    installingUpdate = true;
    updateError = "";
    try {
      await InstallUpdate();
    } catch (e: any) {
      updateError = e?.toString() ?? "install failed";
      installingUpdate = false;
    }
  }
</script>

<div class="settings-panel">
  <div class="settings-header">
    <span class="settings-title">// config</span>
    <button class="close-btn" on:click={() => dispatch("close")}>[ × ]</button>
  </div>

  <div class="settings-body">
    <div class="settings-section">
      <div class="section-label">startup</div>
      {#if autostartLoading}
        <div class="loading-text">// loading...</div>
      {:else}
        <label class="checkbox-label" on:click|preventDefault={toggleAutostart}>
          <span class="hacker-check" class:checked={autostartEnabled}>
            {#if autostartEnabled}<span class="check-mark">■</span>{:else}<span class="check-empty">□</span>{/if}
          </span>
          <span class="checkbox-text" class:active={autostartEnabled}>start on login</span>
        </label>
      {/if}
    </div>

    <div class="settings-section">
      <div class="section-label">window</div>
      {#if closeToTrayLoading}
        <div class="loading-text">// loading...</div>
      {:else}
        <label class="checkbox-label" on:click|preventDefault={toggleCloseToTray}>
          <span class="hacker-check" class:checked={closeToTray}>
            {#if closeToTray}<span class="check-mark">■</span>{:else}<span class="check-empty">□</span>{/if}
          </span>
          <span class="checkbox-text" class:active={closeToTray}>hide to tray on close</span>
        </label>
      {/if}
    </div>

    <div class="settings-section">
      <div class="section-label">monitoring</div>
      <label class="checkbox-label" on:click|preventDefault={toggleResourceStats}>
        <span class="hacker-check" class:checked={$showResourceStats}>
          {#if $showResourceStats}<span class="check-mark">■</span>{:else}<span class="check-empty">□</span>{/if}
        </span>
        <span class="checkbox-text" class:active={$showResourceStats}>show cpu/ram widget on active tunnels</span>
      </label>
    </div>

    <div class="settings-section">
      <div class="section-label">updates</div>
      {#if currentVersion}
        <div class="version-line">
          <span class="version-label">version</span>
          <span class="version-value">{currentVersion}</span>
        </div>
      {/if}

      {#if updateInfo}
        <div class="update-available">
          <span class="update-text">! v{updateInfo.latestVersion} available</span>
          <button
            class="action-btn accent"
            on:click={installUpdate}
            disabled={installingUpdate}
          >
            {installingUpdate ? "// installing..." : "[ install & restart ]"}
          </button>
        </div>
      {:else}
        <button
          class="action-btn"
          on:click={checkForUpdate}
          disabled={checkingUpdate}
        >
          {checkingUpdate ? "// checking..." : "[ check for updates ]"}
        </button>
      {/if}

      {#if updateError}
        <div class="status-text">{updateError}</div>
      {/if}
    </div>
  </div>
</div>

<style>
  .settings-panel {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 2px;
  }

  .settings-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 14px;
    border-bottom: 1px solid var(--border);
  }

  .settings-title {
    font-size: 11px;
    color: var(--accent);
    font-family: 'JetBrains Mono', monospace;
    letter-spacing: 0.05em;
  }

  .close-btn {
    background: transparent;
    border: none;
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    cursor: pointer;
    padding: 2px 6px;
    transition: color 0.15s;
  }

  .close-btn:hover {
    color: #ff4444;
  }

  .settings-body {
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  .settings-section {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .section-label {
    font-size: 9px;
    color: var(--accent);
    font-family: 'JetBrains Mono', monospace;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    padding-bottom: 4px;
    border-bottom: 1px solid var(--border);
  }

  .loading-text {
    font-size: 10px;
    color: #333;
    font-family: 'JetBrains Mono', monospace;
  }

  .checkbox-label {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    font-size: 11px;
    color: var(--text);
    font-family: 'JetBrains Mono', monospace;
    user-select: none;
    padding: 4px 0;
  }

  .hacker-check {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 16px;
    height: 16px;
    font-size: 14px;
    line-height: 1;
    transition: all 0.15s;
  }

  .check-empty {
    color: var(--muted);
  }

  .check-mark {
    color: var(--accent);
    text-shadow: 0 0 6px var(--accent);
  }

  .checkbox-text {
    color: var(--muted);
    transition: color 0.15s;
  }

  .checkbox-text.active {
    color: var(--accent);
  }

  .checkbox-label:hover .check-empty {
    color: var(--text);
  }

  .checkbox-label:hover .checkbox-text:not(.active) {
    color: var(--text);
  }

  .version-line {
    display: flex;
    align-items: center;
    gap: 10px;
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
  }

  .version-label {
    color: var(--muted);
    text-transform: uppercase;
    font-size: 9px;
    letter-spacing: 0.08em;
  }

  .version-value {
    color: var(--text);
  }

  .update-available {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 8px 10px;
    border: 1px solid rgba(0, 255, 136, 0.2);
    border-radius: 2px;
    background: rgba(0, 255, 136, 0.03);
  }

  .update-text {
    font-size: 10px;
    color: var(--accent);
    font-family: 'JetBrains Mono', monospace;
  }

  .action-btn {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    padding: 5px 10px;
    cursor: pointer;
    border-radius: 2px;
    align-self: flex-start;
    transition: color 0.15s, border-color 0.15s, background 0.15s;
    letter-spacing: 0.05em;
  }

  .action-btn:hover:not(:disabled) {
    color: var(--text);
    border-color: var(--muted);
  }

  .action-btn.accent {
    color: var(--accent);
    border-color: var(--accent);
  }

  .action-btn.accent:hover:not(:disabled) {
    background: rgba(0, 255, 136, 0.1);
  }

  .action-btn:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .status-text {
    font-size: 10px;
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
  }
</style>
