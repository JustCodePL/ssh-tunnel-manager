<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from "svelte";
  import { GetAutostart, SetAutostart, GetCurrentVersion, CheckForUpdate, InstallUpdate } from "../../wailsjs/go/main/App";
  import { EventsOn } from "../../wailsjs/runtime/runtime";
  import { theme } from "../stores/theme";
  import type { Theme } from "../stores/theme";

  const dispatch = createEventDispatcher<{ close: void }>();

  let autostartEnabled = false;
  let autostartLoading = true;

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

  async function checkForUpdate() {
    checkingUpdate = true;
    updateError = "";
    try {
      const info = await CheckForUpdate();
      if (info) {
        updateInfo = info as any;
      } else {
        updateError = "You are up to date.";
      }
    } catch (e: any) {
      updateError = e?.toString() ?? "Update check failed.";
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
      updateError = e?.toString() ?? "Install failed.";
      installingUpdate = false;
    }
  }

  function setTheme(t: Theme) {
    theme.set(t);
  }
</script>

<div class="bg-white dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 rounded-lg p-5">
  <div class="flex items-center justify-between mb-4">
    <h2 class="text-base font-semibold">Settings</h2>
    <button
      class="text-xs text-zinc-500 hover:text-zinc-300"
      on:click={() => dispatch("close")}
    >
      Close
    </button>
  </div>

  <div class="space-y-4">
    <div>
      <h3 class="text-sm font-medium text-zinc-600 dark:text-zinc-400 mb-2">Theme</h3>
      <div class="flex gap-2">
        {#each [["dark", "Dark"], ["light", "Light"], ["system", "System"]] as [value, label]}
          <button
            class="px-3 py-1.5 text-xs rounded {$theme === value
              ? 'bg-blue-600 text-white'
              : 'bg-zinc-100 dark:bg-zinc-700 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-600'}"
            on:click={() => setTheme(value)}
          >
            {label}
          </button>
        {/each}
      </div>
    </div>

    <div>
      <h3 class="text-sm font-medium text-zinc-600 dark:text-zinc-400 mb-2">Startup</h3>
      {#if autostartLoading}
        <p class="text-xs text-zinc-500">Loading…</p>
      {:else}
        <label class="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={autostartEnabled}
            on:change={toggleAutostart}
            class="rounded"
          />
          <span class="text-sm text-zinc-700 dark:text-zinc-300">Start on login</span>
        </label>
      {/if}
    </div>

    <div>
      <h3 class="text-sm font-medium text-zinc-600 dark:text-zinc-400 mb-2">Updates</h3>
      {#if currentVersion}
        <p class="text-xs text-zinc-500 mb-2">
          Version: <span class="font-mono">{currentVersion}</span>
        </p>
      {/if}

      {#if updateInfo}
        <div class="rounded bg-blue-50 dark:bg-blue-900/30 border border-blue-200 dark:border-blue-700 p-3 mb-2">
          <p class="text-sm font-medium text-blue-800 dark:text-blue-200 mb-2">
            Version {updateInfo.latestVersion} is available
          </p>
          <button
            class="px-3 py-1.5 text-xs rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
            on:click={installUpdate}
            disabled={installingUpdate}
          >
            {installingUpdate ? "Installing…" : "Install & Restart"}
          </button>
        </div>
      {:else}
        <button
          class="px-3 py-1.5 text-xs rounded bg-zinc-100 dark:bg-zinc-700 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-600 disabled:opacity-50"
          on:click={checkForUpdate}
          disabled={checkingUpdate}
        >
          {checkingUpdate ? "Checking…" : "Check for updates"}
        </button>
      {/if}

      {#if updateError}
        <p class="text-xs text-zinc-500 mt-1">{updateError}</p>
      {/if}
    </div>
  </div>
</div>
