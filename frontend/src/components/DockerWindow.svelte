<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from "svelte";
  import { ListDockerContainers } from "../../wailsjs/go/main/App";
  import type { sysstats } from "../../wailsjs/go/models";
  import type { TunnelConfig } from "../types";

  export let tunnel: TunnelConfig;
  const dispatch = createEventDispatcher<{ close: void }>();

  const POLL_MS = 5000;

  let containers: sysstats.DockerContainer[] = [];
  let loading = true;
  let error = "";
  let minimized = false;
  let timer: ReturnType<typeof setInterval> | null = null;

  async function refresh() {
    try {
      containers = await ListDockerContainers(tunnel.id);
      error = "";
    } catch (e: any) {
      error = e?.toString() ?? "failed to list containers";
    } finally {
      loading = false;
    }
  }

  function isRunning(c: sysstats.DockerContainer): boolean {
    return c.state === "running";
  }

  onMount(() => {
    refresh();
    timer = setInterval(refresh, POLL_MS);
  });

  onDestroy(() => {
    if (timer) clearInterval(timer);
  });

  function handleClose() {
    dispatch("close");
  }
</script>

<div class="overlay" class:hidden={minimized}>
  <div class="window">
    <div class="titlebar">
      <span class="title">docker — {tunnel.name} ({tunnel.user}@{tunnel.host})</span>
      <div class="actions">
        <button class="title-btn" on:click={refresh} title="Refresh">⟳</button>
        <button class="title-btn" on:click={() => (minimized = true)} title="Minimize">−</button>
        <button class="close-btn" on:click={handleClose} title="Close">×</button>
      </div>
    </div>
    <div class="body">
      {#if loading}
        <div class="status-msg">listing containers...</div>
      {:else if error}
        <div class="status-msg error">{error}</div>
      {:else if containers.length === 0}
        <div class="status-msg">no containers</div>
      {:else}
        <div class="cards">
          {#each containers as c}
            <div class="card" class:running={isRunning(c)}>
              <div class="card-head">
                <span class="name" title={c.name}>
                  <span class="dot" class:up={isRunning(c)}></span>{c.name}
                </span>
                <span class="state" class:up={isRunning(c)}>{c.state}</span>
              </div>
              <div class="image" title={c.image}>{c.image}</div>
              <div class="status">{c.status}</div>
              {#if c.ports}
                <div class="ports" title={c.ports}>{c.ports}</div>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>

{#if minimized}
  <button class="restore-pill" on:click={() => (minimized = false)} title="Restore docker">
    <span class="pill-icon">▢</span>
    <span class="pill-text">docker — {tunnel.name}</span>
    <span class="pill-close" on:click|stopPropagation={handleClose} title="Close">×</span>
  </button>
{/if}

<style>
  .overlay { position: fixed; top: 36px; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.85); z-index: 1000; display: flex; align-items: center; justify-content: center; }
  .overlay.hidden { display: none; }
  .window { width: 85vw; max-width: 1000px; height: 70vh; background: #0a0a0a; border: 1px solid #00ff88; border-radius: 2px; display: flex; flex-direction: column; }
  .titlebar { display: flex; justify-content: space-between; align-items: center; padding: 6px 12px; border-bottom: 1px solid #1a1a1a; background: #111; }
  .title { font-family: "JetBrains Mono", monospace; font-size: 11px; color: #00ff88; }
  .actions { display: flex; gap: 4px; }
  .title-btn, .close-btn { background: none; border: 1px solid #333; color: #888; font-size: 14px; width: 24px; height: 24px; cursor: pointer; border-radius: 2px; line-height: 1; padding: 0; }
  .title-btn:hover { color: #00d4ff; border-color: #00d4ff; }
  .close-btn:hover { color: #ff4444; border-color: #ff4444; }
  .body { flex: 1; overflow: auto; padding: 8px 12px; }
  .status-msg { font-family: "JetBrains Mono", monospace; font-size: 12px; color: #00ff88; padding: 20px; }
  .status-msg.error { color: #ff4444; white-space: pre-wrap; }

  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(250px, 1fr)); gap: 8px; font-family: "JetBrains Mono", monospace; }
  .card { background: #0d0d0d; border: 1px solid #1a1a1a; border-left: 3px solid #555; border-radius: 2px; padding: 9px 11px; display: flex; flex-direction: column; gap: 4px; }
  .card:hover { border-color: #333; }
  .card.running { border-left-color: #00ff88; }
  .card-head { display: flex; justify-content: space-between; align-items: baseline; gap: 8px; }
  .name { color: #00ff88; font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: flex; align-items: center; }
  .dot { display: inline-block; width: 7px; height: 7px; border-radius: 50%; margin-right: 6px; background: #555; flex-shrink: 0; }
  .dot.up { background: #00ff88; box-shadow: 0 0 5px #00ff88; }
  .state { font-size: 9px; text-transform: uppercase; letter-spacing: 0.05em; color: var(--muted); flex-shrink: 0; }
  .state.up { color: #00ff88; }
  .image { color: var(--accent2); font-size: 10px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .status { color: var(--muted); font-size: 10px; }
  .ports { color: var(--muted); font-size: 9px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .restore-pill { position: fixed; bottom: 12px; right: 12px; z-index: 1000; display: flex; align-items: center; gap: 8px; padding: 6px 10px; background: #0a0a0a; border: 1px solid #00ff88; border-radius: 2px; color: #00ff88; font-family: "JetBrains Mono", monospace; font-size: 11px; cursor: pointer; box-shadow: 0 0 12px rgba(0, 255, 136, 0.3); }
  .restore-pill:hover { background: #111; }
  .pill-icon { font-size: 13px; }
  .pill-text { max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pill-close { color: #888; padding: 0 4px; font-size: 14px; line-height: 1; }
  .pill-close:hover { color: #ff4444; }
</style>
