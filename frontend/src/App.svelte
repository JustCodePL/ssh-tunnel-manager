<script lang="ts">
  import { onMount } from "svelte";
  import Titlebar from "./components/Titlebar.svelte";
  import { isMac } from "./lib/platform";
  import TunnelList from "./components/TunnelList.svelte";
  import TunnelForm from "./components/TunnelForm.svelte";
  import ImportDialog from "./components/ImportDialog.svelte";
  import PassphraseDialog from "./components/PassphraseDialog.svelte";
  import CommandPalette from "./components/CommandPalette.svelte";
  import ToastContainer from "./components/ToastContainer.svelte";
  import ConfirmHost from "./components/ConfirmHost.svelte";
  import PortlessBindBanner from "./components/PortlessBindBanner.svelte";
  import PortForwardErrorBanner from "./components/PortForwardErrorBanner.svelte";
  import Icon from "./components/Icon.svelte";
  import {
    loadTunnels,
    initEventListeners,
    tunnels,
    exportTunnels,
    disconnectAll,
    disconnectTunnel,
    connectTunnel,
    connectGroup,
    disconnectGroup,
  } from "./stores/tunnels";
  import { initSettings } from "./stores/settings";
  import { loadSavedCapabilities } from "./stores/capabilities";
  import { CopyToClipboard, ReportWindowReady } from "../bindings/ssh-tunnel-manager/app";
  import { EventsOn } from "./lib/runtime";
  import { openTool } from "./lib/openTool";
  import { showToast } from "./stores/toast";

  let showImport = false;
  let showNew = false;
  let newGroup = "";
  let creatingGroup = false;
  let showMenu = false;
  let showPalette = false;
  let passphraseKeyPath: string | null = null;
  let updateAvailable = false;
  let query = "";
  let filter = "all";

  onMount(() => {
    void ReportWindowReady("main");
    initEventListeners();
    void loadTunnels();
    void initSettings();
    void loadSavedCapabilities();

    EventsOn("passphrase:request", (keyPath: string) => {
      passphraseKeyPath = keyPath;
    });
    EventsOn("updater:update-available", () => {
      updateAvailable = true;
    });
    EventsOn("updater:update-cleared", () => {
      updateAvailable = false;
    });
  });

  async function handleExport() {
    const ids = $tunnels.map((t) => t.id);
    if (ids.length === 0) return;
    try {
      await exportTunnels(ids);
    } catch (e) {
      console.error("export failed:", e);
    }
  }

  function openNew(group: string) {
    showMenu = false;
    newGroup = group;
    showNew = true;
  }

  async function runAction(run: any) {
    showPalette = false;
    switch (run.kind) {
      case "tool":
        return openTool(run.spec);
      case "new-tunnel":
        openNew("");
        return;
      case "new-group":
        creatingGroup = true;
        return;
      case "import":
        showImport = true;
        return;
      case "export":
        return handleExport();
      case "disconnect-all":
        return disconnectAll();
      case "connect":
        return connectTunnel(run.id);
      case "disconnect":
        return disconnectTunnel(run.id);
      case "connect-group":
        return connectGroup(run.group);
      case "disconnect-group":
        return disconnectGroup(run.group);
      case "copy":
        await CopyToClipboard(run.text);
        showToast(`Copied ${run.text}`, "success");
        return;
    }
  }

  function onKey(event: KeyboardEvent) {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
      event.preventDefault();
      showPalette = !showPalette;
    }
  }
</script>

<svelte:window on:keydown={onKey} />

<div class="app-root">
  {#if !isMac}<Titlebar />{/if}

  <div class="top">
    <input
      class="field-control search"
      type="search"
      placeholder="Find a tunnel"
      aria-label="Find a tunnel"
      data-testid="search"
      bind:value={query}
    />
    <button type="button" class="btn palette-hint" data-testid="open-palette" on:click={() => (showPalette = true)}>
      Actions <kbd>{navigator.userAgent.includes("Mac") ? "⌘K" : "Ctrl+K"}</kbd>
    </button>
    <span class="more">
      <button
        type="button"
        class="icon-btn"
        title="New"
        aria-label="New"
        aria-haspopup="menu"
        aria-expanded={showMenu}
        data-testid="new-menu"
        on:click|stopPropagation={() => (showMenu = !showMenu)}
      >
        <Icon name="plus" />
      </button>
      {#if showMenu}
        <div class="menu" role="menu">
          <button type="button" class="menu-item" role="menuitem" data-testid="new-tunnel" on:click={() => openNew("")}>Tunnel</button>
          <button type="button" class="menu-item" role="menuitem" data-testid="new-group" on:click={() => { showMenu = false; creatingGroup = true; }}>Group</button>
          <div class="menu-sep"></div>
          <button type="button" class="menu-item" role="menuitem" data-testid="import-tunnels" on:click={() => { showMenu = false; showImport = true; }}>Import from SSH config</button>
        </div>
      {/if}
    </span>
    <button
      type="button"
      class="icon-btn gear"
      title={updateAvailable ? "Settings, an update is available" : "Settings"}
      aria-label="Settings"
      data-testid="open-settings"
      on:click={() => void openTool({ tool: "settings", tunnelId: "" })}
    >
      <Icon name="settings" />
      {#if updateAvailable}<span class="update-dot"></span>{/if}
    </button>
  </div>

  <main class="app-main">
    {#if showImport}
      <ImportDialog on:close={() => (showImport = false)} />
    {:else}
      <TunnelList bind:filter bind:creatingGroup {query} paused={showPalette || showNew} on:new={(e) => openNew(e.detail)} />
    {/if}
  </main>

  {#if showNew}
    <TunnelForm tunnel={null} initialGroup={newGroup} on:close={() => (showNew = false)} />
  {/if}
  {#if showPalette}
    <CommandPalette on:close={() => (showPalette = false)} on:run={(e) => runAction(e.detail)} />
  {/if}

  <ToastContainer />
  <ConfirmHost />
  <PortlessBindBanner />
  <PortForwardErrorBanner />

  {#if passphraseKeyPath}
    <PassphraseDialog keyPath={passphraseKeyPath} on:close={() => (passphraseKeyPath = null)} />
  {/if}
</div>

<svelte:body on:click={() => (showMenu = false)} />

<style>
  .app-root {
    display: flex;
    flex-direction: column;
    height: 100vh;
    background: var(--bg);
    overflow: hidden;
  }
  .top {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 7px 10px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }
  .search {
    flex: 1;
    padding: 5px 9px;
  }
  .palette-hint kbd {
    font-size: 11px;
    color: var(--muted);
  }
  .more {
    position: relative;
  }
  .more .menu {
    top: calc(100% + 4px);
    right: 0;
  }
  .gear {
    position: relative;
  }
  [data-testid="new-menu"] {
    color: var(--accent);
  }
  .update-dot {
    position: absolute;
    top: 2px;
    right: 2px;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent2);
  }
  .app-main {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
</style>
