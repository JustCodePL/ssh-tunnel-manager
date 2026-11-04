<script lang="ts">
  import { onMount } from "svelte";
  import { GetToolWindow, ActivateTab, CloseTab, ReportWindowReady } from "../bindings/ssh-tunnel-manager/app";
  import { EventsOn, WindowClose } from "./lib/runtime";
  import { initSettings } from "./stores/settings";
  import { loadTunnels, initEventListeners } from "./stores/tunnels";
  import { loadSavedCapabilities } from "./stores/capabilities";
  import { openTool } from "./lib/openTool";
  import ToolFrame from "./tools/ToolFrame.svelte";
  import ToastContainer from "./components/ToastContainer.svelte";
  import ConfirmHost from "./components/ConfirmHost.svelte";
  import PassphraseDialog from "./components/PassphraseDialog.svelte";
  import Icon from "./components/Icon.svelte";
  import { tabTitle } from "./lib/tabs.js";

  export let windowName: string;

  interface Tab {
    id: string;
    spec: { tool: string; tunnelId: string; remoteHost?: string; remotePort?: number; path?: string };
  }

  let tabs: Tab[] = [];
  let active = "";
  let error = "";
  let ready = false;
  let passphraseKeyPath: string | null = null;

  async function reload() {
    try {
      const state = await GetToolWindow(windowName);
      tabs = (state.tabs ?? []) as any;
      active = state.active;
      error = "";
    } catch (e: any) {
      // The window was forgotten, for example when its last tab closed.
      tabs = [];
      if (!ready) error = String(e?.message ?? e);
    }
  }

  onMount(async () => {
    void ReportWindowReady(windowName);
    initEventListeners(false);
    EventsOn("windows:changed", (name: string) => {
      if (name === windowName) void reload();
    });
    EventsOn("passphrase:request", (keyPath: string) => {
      // The window that asked for the session is the one in front of the user.
      if (document.hasFocus()) passphraseKeyPath = keyPath;
    });
    await Promise.all([initSettings(), loadTunnels(), loadSavedCapabilities()]);
    await reload();
    ready = true;
    // Browser tabs of the server build have no native close to report, so the
    // end-to-end tests close the window's tabs through this.
    if (new URLSearchParams(location.search).has("external")) {
      (window as any).stmCloseWindow = async () => {
        for (const tab of [...tabs]) await CloseTab(windowName, tab.id);
      };
    }
  });

  function activate(tab: Tab) {
    active = tab.id;
    void ActivateTab(windowName, tab.id);
  }

  async function closeTab(tab: Tab) {
    const last = tabs.length === 1;
    await CloseTab(windowName, tab.id);
    // A window without native close, in the server build, just stays empty.
    if (last) void WindowClose().catch(() => window.close());
  }

  function onKey(event: KeyboardEvent) {
    if (!(event.metaKey || event.ctrlKey) || tabs.length < 2) return;
    const number = Number(event.key);
    if (number >= 1 && number <= tabs.length) {
      event.preventDefault();
      activate(tabs[number - 1]);
    }
  }

  $: current = tabs.find((t) => t.id === active);
  $: canAddTab = current?.spec.tool === "terminal";
  $: if (current) document.title = tabTitle(current.spec);
</script>

<svelte:window on:keydown={onKey} />

<div class="shell" data-testid="tool-window">
  {#if tabs.length > 1 || canAddTab}
    <div class="tabs" role="tablist">
      {#each tabs as tab (tab.id)}
        <div class="tab" class:on={tab.id === active} data-testid="tab">
          <button type="button" role="tab" aria-selected={tab.id === active} on:click={() => activate(tab)}>
            {tabTitle(tab.spec)}
          </button>
          <button type="button" class="tab-close" aria-label={`Close ${tabTitle(tab.spec)}`} on:click={() => closeTab(tab)}>
            <Icon name="close" size={11} />
          </button>
        </div>
      {/each}
      {#if canAddTab && current}
        <button type="button" class="icon-btn" aria-label="New terminal tab" data-testid="new-tab" on:click={() => current && openTool(current.spec)}>
          <Icon name="plus" size={13} />
        </button>
      {/if}
    </div>
  {/if}

  <div class="body">
    {#if error}
      <p class="message">{error}</p>
    {:else if ready && !tabs.length}
      <p class="message">This window has no open tabs.</p>
    {/if}
    {#each tabs as tab (tab.id)}
      <div class="pane" class:hidden={tab.id !== active}>
        <ToolFrame spec={tab.spec} tabId={tab.id} visible={tab.id === active} on:close={() => closeTab(tab)} />
      </div>
    {/each}
  </div>

  <ToastContainer />
  <ConfirmHost />
</div>
{#if passphraseKeyPath}
  <PassphraseDialog keyPath={passphraseKeyPath} on:close={() => (passphraseKeyPath = null)} />
{/if}

<style>
  .shell {
    display: flex;
    flex-direction: column;
    height: 100vh;
    background: var(--bg);
    overflow: hidden;
  }
  .tabs {
    display: flex;
    align-items: center;
    gap: 2px;
    padding: 4px 6px 0;
    background: var(--surface);
    border-bottom: 1px solid var(--border);
    overflow-x: auto;
    flex-shrink: 0;
  }
  .tab {
    display: flex;
    align-items: center;
    gap: 2px;
    padding: 3px 6px 4px 10px;
    font-size: 12px;
    color: var(--muted);
    border-radius: var(--radius) var(--radius) 0 0;
    white-space: nowrap;
  }
  .tab.on {
    background: var(--bg);
    color: var(--text);
  }
  .tab-close {
    display: grid;
    place-items: center;
    width: 18px;
    height: 18px;
    color: var(--muted);
    border-radius: var(--radius);
  }
  .tab-close:hover {
    background: var(--raise);
    color: var(--text);
  }
  .body {
    position: relative;
    flex: 1;
    min-height: 0;
  }
  .pane {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
  }
  .pane.hidden {
    visibility: hidden;
    pointer-events: none;
  }
  .message {
    padding: 24px;
    color: var(--muted);
    text-align: center;
  }
</style>
