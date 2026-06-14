<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from "svelte";
  import { GetDiskUsage } from "../../wailsjs/go/main/App";
  import type { sysstats } from "../../wailsjs/go/models";
  import type { TunnelConfig } from "../types";
  import { formatBytes } from "../format";

  export let tunnel: TunnelConfig;
  const dispatch = createEventDispatcher<{ close: void }>();

  const POLL_MS = 5000;

  let mounts: sysstats.DiskMount[] = [];
  let loading = true;
  let error = "";
  let minimized = false;
  let timer: ReturnType<typeof setInterval> | null = null;

  async function refresh() {
    try {
      mounts = await GetDiskUsage(tunnel.id);
      error = "";
    } catch (e: any) {
      error = e?.toString() ?? "failed to read disk usage";
    } finally {
      loading = false;
    }
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
      <span class="title">disk — {tunnel.name} ({tunnel.user}@{tunnel.host})</span>
      <div class="actions">
        <button class="title-btn" on:click={refresh} title="Refresh">⟳</button>
        <button class="title-btn" on:click={() => (minimized = true)} title="Minimize">−</button>
        <button class="close-btn" on:click={handleClose} title="Close">×</button>
      </div>
    </div>
    <div class="body">
      {#if loading}
        <div class="status-msg">reading disk usage...</div>
      {:else if error}
        <div class="status-msg error">{error}</div>
      {:else if mounts.length === 0}
        <div class="status-msg">no filesystems found</div>
      {:else}
        <div class="cards">
          {#each mounts as m}
            <div class="card">
              <div class="card-head">
                <span class="mount" title={m.mountPoint}>{m.mountPoint}</span>
                <span class="pct">{m.usePercent.toFixed(0)}%</span>
              </div>
              <div class="fs" title={m.filesystem}>{m.filesystem}</div>
              <div class="bar">
                <div
                  class="bar-fill"
                  class:warn={m.usePercent >= 80 && m.usePercent < 90}
                  class:crit={m.usePercent >= 90}
                  style={`width:${Math.min(m.usePercent, 100)}%`}
                ></div>
              </div>
              <div class="meta">
                <span><span class="meta-lbl">used</span> {formatBytes(m.used)}</span>
                <span><span class="meta-lbl">free</span> {formatBytes(m.avail)}</span>
                <span><span class="meta-lbl">size</span> {formatBytes(m.total)}</span>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>

{#if minimized}
  <button class="restore-pill" on:click={() => (minimized = false)} title="Restore disk">
    <span class="pill-icon">▢</span>
    <span class="pill-text">disk — {tunnel.name}</span>
    <span class="pill-close" on:click|stopPropagation={handleClose} title="Close">×</span>
  </button>
{/if}

<style>
  .overlay { position: fixed; top: 36px; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.85); z-index: 1000; display: flex; align-items: center; justify-content: center; }
  .overlay.hidden { display: none; }
  .window { width: 80vw; max-width: 900px; height: 70vh; background: #0a0a0a; border: 1px solid #00ff88; border-radius: 2px; display: flex; flex-direction: column; }
  .titlebar { display: flex; justify-content: space-between; align-items: center; padding: 6px 12px; border-bottom: 1px solid #1a1a1a; background: #111; }
  .title { font-family: "JetBrains Mono", monospace; font-size: 11px; color: #00ff88; }
  .actions { display: flex; gap: 4px; }
  .title-btn, .close-btn { background: none; border: 1px solid #333; color: #888; font-size: 14px; width: 24px; height: 24px; cursor: pointer; border-radius: 2px; line-height: 1; padding: 0; }
  .title-btn:hover { color: #00d4ff; border-color: #00d4ff; }
  .close-btn:hover { color: #ff4444; border-color: #ff4444; }
  .body { flex: 1; overflow: auto; padding: 8px 12px; }
  .status-msg { font-family: "JetBrains Mono", monospace; font-size: 12px; color: #00ff88; padding: 20px; }
  .status-msg.error { color: #ff4444; white-space: pre-wrap; }

  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 8px; font-family: "JetBrains Mono", monospace; }
  .card { background: #0d0d0d; border: 1px solid #1a1a1a; border-radius: 2px; padding: 10px 12px; display: flex; flex-direction: column; gap: 6px; }
  .card:hover { border-color: #333; }
  .card-head { display: flex; justify-content: space-between; align-items: baseline; gap: 8px; }
  .mount { color: #00ff88; font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pct { color: var(--text); font-size: 12px; flex-shrink: 0; }
  .fs { color: var(--muted); font-size: 9px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .bar { height: 9px; background: var(--surface2); border: 1px solid var(--border); border-radius: 2px; overflow: hidden; }
  .bar-fill { height: 100%; background: #00ff88; transition: width 0.4s ease; }
  .bar-fill.warn { background: #ffb000; }
  .bar-fill.crit { background: #ff4444; }
  .meta { display: flex; justify-content: space-between; gap: 6px; font-size: 9px; color: var(--text); }
  .meta-lbl { color: var(--muted); text-transform: uppercase; letter-spacing: 0.04em; }

  .restore-pill { position: fixed; bottom: 12px; right: 12px; z-index: 1000; display: flex; align-items: center; gap: 8px; padding: 6px 10px; background: #0a0a0a; border: 1px solid #00ff88; border-radius: 2px; color: #00ff88; font-family: "JetBrains Mono", monospace; font-size: 11px; cursor: pointer; box-shadow: 0 0 12px rgba(0, 255, 136, 0.3); }
  .restore-pill:hover { background: #111; }
  .pill-icon { font-size: 13px; }
  .pill-text { max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pill-close { color: #888; padding: 0 4px; font-size: 14px; line-height: 1; }
  .pill-close:hover { color: #ff4444; }
</style>
