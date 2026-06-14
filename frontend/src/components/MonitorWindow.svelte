<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from "svelte";
  import { GetProcessStats } from "../../wailsjs/go/main/App";
  import type { sysstats } from "../../wailsjs/go/models";
  import type { TunnelConfig } from "../types";
  import { formatBytes } from "../format";

  export let tunnel: TunnelConfig;
  const dispatch = createEventDispatcher<{ close: void }>();

  const POLL_MS = 3000;

  let stats: sysstats.ProcessStats | null = null;
  let loading = true;
  let error = "";
  let minimized = false;
  let timer: ReturnType<typeof setInterval> | null = null;

  async function refresh() {
    try {
      stats = await GetProcessStats(tunnel.id);
      error = "";
    } catch (e: any) {
      error = e?.toString() ?? "failed to read process stats";
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

  function loadClass(value: number): string {
    if (value >= 80) return "crit";
    if (value >= 50) return "warn";
    return "";
  }

  function coreLabel(name: string): string {
    if (name === "cpu") return "all";
    return name.replace("cpu", "");
  }

  function formatUptime(seconds: number): string {
    const d = Math.floor(seconds / 86400);
    const h = Math.floor((seconds % 86400) / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    if (d > 0) return `${d}d ${h}h ${m}m`;
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m`;
  }

  $: memPct = stats && stats.memTotal > 0 ? (stats.memUsed / stats.memTotal) * 100 : 0;
  $: swapPct = stats && stats.swapTotal > 0 ? (stats.swapUsed / stats.swapTotal) * 100 : 0;
</script>

<div class="overlay" class:hidden={minimized}>
  <div class="window">
    <div class="titlebar">
      <span class="title">htop — {tunnel.name} ({tunnel.user}@{tunnel.host})</span>
      <div class="actions">
        <button class="title-btn" on:click={refresh} title="Refresh">⟳</button>
        <button class="title-btn" on:click={() => (minimized = true)} title="Minimize">−</button>
        <button class="close-btn" on:click={handleClose} title="Close">×</button>
      </div>
    </div>
    <div class="body">
      {#if loading}
        <div class="status-msg">reading process stats...</div>
      {:else if error}
        <div class="status-msg error">{error}</div>
      {:else if stats}
        <div class="summary">
          <div class="summary-item"><span class="lbl">load</span><span class="val">{stats.load1.toFixed(2)} {stats.load5.toFixed(2)} {stats.load15.toFixed(2)}</span></div>
          <div class="summary-item"><span class="lbl">uptime</span><span class="val">{formatUptime(stats.uptimeSeconds)}</span></div>
          <div class="summary-item"><span class="lbl">tasks</span><span class="val">{stats.processes.length}</span></div>
        </div>

        <div class="section-title">cpu</div>
        <div class="cores">
          {#each stats.cores as core}
            <div class="core" class:agg={core.name === "cpu"}>
              <span class="core-name">{coreLabel(core.name)}</span>
              <div class="bar">
                <div class="bar-fill {loadClass(core.percent)}" style={`width:${Math.min(core.percent, 100)}%`}></div>
              </div>
              <span class="core-pct">{core.percent.toFixed(0)}%</span>
            </div>
          {/each}
        </div>

        <div class="section-title">memory</div>
        <div class="mem-row">
          <div class="mem-metric">
            <span class="core-name">ram</span>
            <div class="bar tall">
              <div class="bar-fill {loadClass(memPct)}" style={`width:${memPct}%`}></div>
            </div>
            <span class="mem-val">{formatBytes(stats.memUsed)} / {formatBytes(stats.memTotal)}</span>
          </div>
          {#if stats.swapTotal > 0}
            <div class="mem-metric">
              <span class="core-name">swap</span>
              <div class="bar tall">
                <div class="bar-fill {loadClass(swapPct)}" style={`width:${swapPct}%`}></div>
              </div>
              <span class="mem-val">{formatBytes(stats.swapUsed)} / {formatBytes(stats.swapTotal)}</span>
            </div>
          {/if}
        </div>

        <div class="section-title">processes <span class="muted">(by cpu)</span></div>
        <div class="procs">
          {#each stats.processes as p (p.pid)}
            <div class="proc-card">
              <div class="proc-head">
                <span class="proc-cmd" title={p.command}>{p.command}</span>
                <span class="proc-pid">#{p.pid}</span>
              </div>
              <div class="proc-meta">
                <span class="proc-user">{p.user}</span>
              </div>
              <div class="proc-stats">
                <div class="proc-stat">
                  <span class="ps-lbl">cpu</span>
                  <div class="bar"><div class="bar-fill {loadClass(p.cpu)}" style={`width:${Math.min(p.cpu, 100)}%`}></div></div>
                  <span class="ps-val">{p.cpu.toFixed(1)}%</span>
                </div>
                <div class="proc-stat">
                  <span class="ps-lbl">mem</span>
                  <div class="bar"><div class="bar-fill mem" style={`width:${Math.min(p.mem, 100)}%`}></div></div>
                  <span class="ps-val">{p.mem.toFixed(1)}%</span>
                </div>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>

{#if minimized}
  <button class="restore-pill" on:click={() => (minimized = false)} title="Restore htop">
    <span class="pill-icon">▢</span>
    <span class="pill-text">htop — {tunnel.name}</span>
    <span class="pill-close" on:click|stopPropagation={handleClose} title="Close">×</span>
  </button>
{/if}

<style>
  .overlay { position: fixed; top: 36px; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.85); z-index: 1000; display: flex; align-items: center; justify-content: center; }
  .overlay.hidden { display: none; }
  .window { width: 88vw; max-width: 1100px; height: 80vh; background: #0a0a0a; border: 1px solid #00ff88; border-radius: 2px; display: flex; flex-direction: column; }
  .titlebar { display: flex; justify-content: space-between; align-items: center; padding: 6px 12px; border-bottom: 1px solid #1a1a1a; background: #111; }
  .title { font-family: "JetBrains Mono", monospace; font-size: 11px; color: #00ff88; }
  .actions { display: flex; gap: 4px; }
  .title-btn, .close-btn { background: none; border: 1px solid #333; color: #888; font-size: 14px; width: 24px; height: 24px; cursor: pointer; border-radius: 2px; line-height: 1; padding: 0; }
  .title-btn:hover { color: #00d4ff; border-color: #00d4ff; }
  .close-btn:hover { color: #ff4444; border-color: #ff4444; }
  .body { flex: 1; overflow: auto; padding: 10px 14px; font-family: "JetBrains Mono", monospace; }
  .status-msg { font-size: 12px; color: #00ff88; padding: 20px; }
  .status-msg.error { color: #ff4444; white-space: pre-wrap; }

  .summary { display: flex; flex-wrap: wrap; gap: 8px 20px; margin-bottom: 10px; }
  .summary-item { display: flex; align-items: baseline; gap: 6px; font-size: 11px; }
  .lbl { color: var(--muted); text-transform: uppercase; font-size: 9px; letter-spacing: 0.06em; }
  .val { color: #00ff88; }

  .section-title { font-size: 9px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--accent2); margin: 12px 0 6px; border-bottom: 1px solid #1a1a1a; padding-bottom: 3px; }
  .section-title .muted { color: var(--muted); }

  .cores { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 5px 12px; }
  .core { display: flex; align-items: center; gap: 6px; font-size: 10px; }
  .core.agg .core-name, .core.agg .core-pct { color: #00ff88; }
  .core-name { color: var(--muted); width: 26px; flex-shrink: 0; text-align: right; }
  .core-pct { color: var(--text); width: 32px; text-align: right; flex-shrink: 0; }

  .bar { flex: 1; height: 7px; background: var(--surface2); border: 1px solid var(--border); border-radius: 2px; overflow: hidden; min-width: 30px; }
  .bar.tall { height: 9px; }
  .bar-fill { height: 100%; background: #00ff88; transition: width 0.4s ease; }
  .bar-fill.warn { background: #ffb000; }
  .bar-fill.crit { background: #ff4444; }
  .bar-fill.mem { background: var(--accent); }

  .mem-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 6px 16px; }
  .mem-metric { display: flex; align-items: center; gap: 8px; font-size: 10px; }
  .mem-val { color: var(--text); white-space: nowrap; }

  .procs { display: grid; grid-template-columns: repeat(auto-fill, minmax(230px, 1fr)); gap: 6px; }
  .proc-card { background: #0d0d0d; border: 1px solid #1a1a1a; border-radius: 2px; padding: 7px 9px; display: flex; flex-direction: column; gap: 4px; }
  .proc-card:hover { border-color: #333; }
  .proc-head { display: flex; justify-content: space-between; align-items: baseline; gap: 6px; }
  .proc-cmd { color: #00ff88; font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .proc-pid { color: var(--muted); font-size: 9px; flex-shrink: 0; }
  .proc-meta { font-size: 9px; color: var(--muted); }
  .proc-stats { display: flex; flex-direction: column; gap: 3px; }
  .proc-stat { display: flex; align-items: center; gap: 6px; font-size: 9px; }
  .ps-lbl { color: var(--muted); width: 22px; flex-shrink: 0; text-transform: uppercase; }
  .ps-val { color: var(--text); width: 36px; text-align: right; flex-shrink: 0; }

  .restore-pill { position: fixed; bottom: 12px; right: 12px; z-index: 1000; display: flex; align-items: center; gap: 8px; padding: 6px 10px; background: #0a0a0a; border: 1px solid #00ff88; border-radius: 2px; color: #00ff88; font-family: "JetBrains Mono", monospace; font-size: 11px; cursor: pointer; box-shadow: 0 0 12px rgba(0, 255, 136, 0.3); }
  .restore-pill:hover { background: #111; }
  .pill-icon { font-size: 13px; }
  .pill-text { max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pill-close { color: #888; padding: 0 4px; font-size: 14px; line-height: 1; }
  .pill-close:hover { color: #ff4444; }
</style>
