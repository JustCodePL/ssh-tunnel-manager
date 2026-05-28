<script lang="ts">
  import { onMount } from "svelte";
  import Titlebar from "./components/Titlebar.svelte";
  import GroupSidebar from "./components/GroupSidebar.svelte";
  import TunnelList from "./components/TunnelList.svelte";
  import ImportDialog from "./components/ImportDialog.svelte";
  import PassphraseDialog from "./components/PassphraseDialog.svelte";
  import SettingsPanel from "./components/SettingsPanel.svelte";
  import ToastContainer from "./components/ToastContainer.svelte";
  import { loadTunnels, initEventListeners, tunnels, exportTunnels } from "./stores/tunnels";
  import { EventsOn } from "../wailsjs/runtime/runtime";

  let showImport = false;
  let showSettings = false;
  let passphraseKeyPath: string | null = null;
  let updateAvailable = false;
  let selectedGroup: string | null = null;

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
</script>

<div class="app-root">
  <Titlebar />

  <div class="app-toolbar">
    <div class="toolbar-left">
      <span class="tunnel-count">{$tunnels.length} tunnel{$tunnels.length !== 1 ? "s" : ""}</span>
    </div>
    <div class="toolbar-right">
      <button class="toolbar-btn" on:click={() => (showImport = !showImport)}>
        {showImport ? "[ CANCEL ]" : "[ IMPORT ]"}
      </button>
      <button
        class="toolbar-btn"
        on:click={handleExport}
        disabled={$tunnels.length === 0}
      >
        [ EXPORT ]
      </button>
      <button
        class="toolbar-btn settings-btn"
        class:active={showSettings}
        on:click={() => (showSettings = !showSettings)}
        title="Settings"
      >
        [ CFG ]
        {#if updateAvailable}
          <span class="update-dot"></span>
        {/if}
      </button>
    </div>
  </div>

  <div class="app-content">
    <GroupSidebar bind:selectedGroup />
    <main class="app-main">
      {#if showImport}
        <ImportDialog on:close={() => (showImport = false)} />
      {:else}
        <TunnelList filterGroup={selectedGroup} on:groupRenamed={(e) => selectedGroup = e.detail} />
      {/if}
    </main>
  </div>

  {#if showSettings}
    <div class="modal-backdrop" on:click|self={() => (showSettings = false)} role="presentation">
      <div class="modal-panel">
        <SettingsPanel on:close={() => (showSettings = false)} />
      </div>
    </div>
  {/if}

  <ToastContainer />

  {#if passphraseKeyPath}
    <PassphraseDialog
      keyPath={passphraseKeyPath}
      on:close={() => (passphraseKeyPath = null)}
    />
  {/if}
</div>

<style>
  .app-root {
    display: flex;
    flex-direction: column;
    height: 100vh;
    background: var(--bg);
    overflow: hidden;
  }

  .app-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 6px 12px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
    flex-shrink: 0;
  }

  .tunnel-count {
    font-size: 10px;
    color: var(--muted);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  .toolbar-right {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .toolbar-btn {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    padding: 3px 8px;
    cursor: pointer;
    border-radius: 2px;
    transition: color 0.15s, border-color 0.15s;
    position: relative;
    letter-spacing: 0.05em;
  }

  .toolbar-btn:hover:not(:disabled) {
    color: var(--accent);
    border-color: var(--accent);
  }

  .toolbar-btn:disabled {
    opacity: 0.3;
    cursor: default;
  }

  .toolbar-btn.active {
    color: var(--accent);
    border-color: var(--accent);
  }

  .update-dot {
    position: absolute;
    top: -3px;
    right: -3px;
    width: 6px;
    height: 6px;
    background: var(--accent2);
    border-radius: 50%;
    box-shadow: 0 0 4px var(--accent2);
  }

  .app-content {
    flex: 1;
    display: flex;
    overflow: hidden;
  }

  .app-main {
    flex: 1;
    overflow-y: auto;
    padding: 12px;
  }

  .modal-backdrop {
    position: fixed;
    inset: 0;
    z-index: 50;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.7);
  }

  .modal-panel {
    position: relative;
    z-index: 51;
    width: 100%;
    max-width: 440px;
    margin: 0 16px;
  }
</style>
