<script lang="ts">
  import { onMount } from "svelte";
  import TunnelList from "./components/TunnelList.svelte";
  import ImportDialog from "./components/ImportDialog.svelte";
  import PassphraseDialog from "./components/PassphraseDialog.svelte";
  import SettingsPanel from "./components/SettingsPanel.svelte";
  import { loadTunnels, initEventListeners, tunnels, exportTunnels } from "./stores/tunnels";
  import { theme } from "./stores/theme";
  import { EventsOn } from "../wailsjs/runtime/runtime";

  let showImport = false;
  let showSettings = false;
  let passphraseKeyPath: string | null = null;
  let updateAvailable = false;

  onMount(() => {
    initEventListeners();
    loadTunnels();

    EventsOn("passphrase:request", (keyPath: string) => {
      passphraseKeyPath = keyPath;
    });

    EventsOn("updater:update-available", () => {
      updateAvailable = true;
    });
  });

  async function handleExport() {
    const ids = $tunnels.map((t) => t.id);
    if (ids.length === 0) return;
    try {
      await exportTunnels(ids);
    } catch (e: any) {
      console.error("export failed:", e);
    }
  }

  function cycleTheme() {
    const order = ["dark", "light", "system"] as const;
    const idx = order.indexOf($theme);
    theme.set(order[(idx + 1) % order.length]);
  }

  const themeIcons: Record<string, string> = {
    dark: "\u263E",
    light: "\u2600",
    system: "\u2699",
  };
</script>

<div class="min-h-screen flex flex-col bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100">
  <header class="border-b border-zinc-200 dark:border-zinc-800 px-5 py-3 flex items-center justify-between shrink-0">
    <h1 class="text-sm font-semibold tracking-tight text-zinc-800 dark:text-zinc-200">
      SSH Tunnel Manager
    </h1>
    <div class="flex items-center gap-2">
      <button
        class="px-2.5 py-1 text-xs rounded bg-zinc-200 dark:bg-zinc-700 hover:bg-zinc-300 dark:hover:bg-zinc-600 text-zinc-700 dark:text-zinc-300"
        on:click={() => (showImport = true)}
      >
        Import
      </button>
      <button
        class="px-2.5 py-1 text-xs rounded bg-zinc-200 dark:bg-zinc-700 hover:bg-zinc-300 dark:hover:bg-zinc-600 text-zinc-700 dark:text-zinc-300"
        on:click={handleExport}
        disabled={$tunnels.length === 0}
      >
        Export
      </button>
      <button
        class="px-2 py-1 text-xs rounded bg-zinc-200 dark:bg-zinc-700 hover:bg-zinc-300 dark:hover:bg-zinc-600 text-zinc-700 dark:text-zinc-300"
        on:click={cycleTheme}
        title="Theme: {$theme}"
      >
        {themeIcons[$theme] || themeIcons.dark}
      </button>
      <button
        class="relative px-2 py-1 text-xs rounded bg-zinc-200 dark:bg-zinc-700 hover:bg-zinc-300 dark:hover:bg-zinc-600 text-zinc-700 dark:text-zinc-300"
        on:click={() => (showSettings = !showSettings)}
        title="Settings"
      >
        &#x2699;
        {#if updateAvailable}
          <span class="absolute -top-0.5 -right-0.5 w-2 h-2 bg-blue-500 rounded-full"></span>
        {/if}
      </button>
    </div>
  </header>
  <main class="flex-1 overflow-y-auto p-5">
    {#if showSettings}
      <SettingsPanel on:close={() => (showSettings = false)} />
    {:else if showImport}
      <ImportDialog on:close={() => (showImport = false)} />
    {:else}
      <TunnelList />
    {/if}
  </main>

  {#if passphraseKeyPath}
    <PassphraseDialog
      keyPath={passphraseKeyPath}
      on:close={() => (passphraseKeyPath = null)}
    />
  {/if}
</div>
