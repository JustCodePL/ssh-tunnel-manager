<script lang="ts">
  import { onMount, onDestroy, tick } from "svelte";
  import { createEventDispatcher } from "svelte";
  import type { LogEntry } from "../types";
  import { GetTunnelLogs, ClearTunnelLogs } from "../../wailsjs/go/main/App";
  import { EventsOn, EventsOff } from "../../wailsjs/runtime/runtime";

  export let tunnelName: string;
  export let tunnelId: string;

  const dispatch = createEventDispatcher<{ close: void }>();

  let logs: LogEntry[] = [];
  let logContainer: HTMLDivElement;
  let autoScroll = true;
  let minimized = false;
  let resizeObserver: ResizeObserver | null = null;

  function formatTime(iso: string): string {
    const d = new Date(iso);
    const hh = String(d.getHours()).padStart(2, "0");
    const mm = String(d.getMinutes()).padStart(2, "0");
    const ss = String(d.getSeconds()).padStart(2, "0");
    return `${hh}:${mm}:${ss}`;
  }

  function levelColor(level: string): string {
    switch (level) {
      case "info":  return "#00d4ff";
      case "warn":  return "#ffaa00";
      case "error": return "#ff4444";
      default:      return "#555555";
    }
  }

  function msgColor(level: string): string {
    switch (level) {
      case "error": return "#ff4444";
      case "warn":  return "#ffaa00";
      default:      return "#aaaaaa";
    }
  }

  async function scrollToBottom() {
    if (autoScroll && logContainer) {
      await tick();
      logContainer.scrollTop = logContainer.scrollHeight;
    }
  }

  function handleScroll() {
    if (!logContainer) return;
    const { scrollTop, scrollHeight, clientHeight } = logContainer;
    autoScroll = scrollHeight - scrollTop - clientHeight < 40;
  }

  async function handleClear() {
    await ClearTunnelLogs(tunnelId);
    logs = [];
  }

  function handleLogEvent(data: any) {
    if (data && data.tunnelId === tunnelId && data.entry) {
      logs = [...logs, data.entry];
      scrollToBottom();
    }
  }

  function minimize() { minimized = true; }
  function restore() {
    minimized = false;
    setTimeout(scrollToBottom, 50);
  }
  function handleClose() { dispatch("close"); }

  onMount(async () => {
    try {
      const initial = await GetTunnelLogs(tunnelId);
      logs = initial || [];
      scrollToBottom();
    } catch (e) {
      console.error("GetTunnelLogs failed:", e);
    }
    EventsOn("tunnel:log", handleLogEvent);

    // Keep view pinned to bottom when window/container is resized.
    if (logContainer && typeof ResizeObserver !== "undefined") {
      resizeObserver = new ResizeObserver(() => scrollToBottom());
      resizeObserver.observe(logContainer);
    }
  });

  onDestroy(() => {
    EventsOff("tunnel:log");
    resizeObserver?.disconnect();
  });

  $: if (logs.length > 0) {
    scrollToBottom();
  }
</script>

<div class="logs-overlay" class:hidden={minimized}>
  <div class="logs-window">
    <div class="logs-titlebar">
      <span class="logs-title">logs — {tunnelName}</span>
      <div class="titlebar-actions">
        <button class="tb-btn" on:click={handleClear} title="Clear all log entries">[ clear ]</button>
        <button class="title-btn minimize-btn" on:click={minimize} title="Minimize">−</button>
        <button class="close-btn" on:click={handleClose} title="Close">×</button>
      </div>
    </div>

    <div
      class="logs-body"
      bind:this={logContainer}
      on:scroll={handleScroll}
    >
      {#if logs.length === 0}
        <div class="logs-empty">// no log entries yet</div>
      {:else}
        {#each logs as entry (entry.timestamp + entry.message)}
          <div class="log-line">
            <span class="log-time">[{formatTime(entry.timestamp)}]</span>
            <span class="log-level" style="color: {levelColor(entry.level)};">{entry.level.toUpperCase()}</span>
            <span class="log-msg" style="color: {msgColor(entry.level)};">{entry.message}</span>
          </div>
        {/each}
      {/if}
    </div>

    <div class="logs-footer">
      <span class="logs-count">{logs.length} entr{logs.length !== 1 ? "ies" : "y"}</span>
      {#if !autoScroll}
        <button
          class="tb-btn"
          on:click={() => { autoScroll = true; scrollToBottom(); }}
        >
          ↓ scroll to bottom
        </button>
      {/if}
    </div>
  </div>
</div>

{#if minimized}
  <button class="restore-pill" on:click={restore} title="Restore logs">
    <span class="pill-icon">▢</span>
    <span class="pill-text">logs — {tunnelName}</span>
    <span class="pill-close" on:click|stopPropagation={handleClose} title="Close">×</span>
  </button>
{/if}

<style>
  .logs-overlay { position: fixed; top: 36px; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.85); z-index: 998; display: flex; align-items: center; justify-content: center; }
  .logs-overlay.hidden { display: none; }
  .logs-window { width: 90vw; height: 80vh; background: #0a0a0a; border: 1px solid #ffaa00; border-radius: 2px; display: flex; flex-direction: column; min-height: 0; }
  .logs-titlebar { display: flex; justify-content: space-between; align-items: center; padding: 6px 12px; border-bottom: 1px solid #1a1a1a; background: #111; flex-shrink: 0; }
  .logs-title { font-family: "JetBrains Mono", monospace; font-size: 11px; color: #ffaa00; }
  .titlebar-actions { display: flex; gap: 4px; align-items: center; }
  .title-btn, .close-btn { background: none; border: 1px solid #333; color: #888; font-size: 16px; width: 24px; height: 24px; cursor: pointer; border-radius: 2px; line-height: 1; padding: 0; }
  .minimize-btn:hover { color: #ffaa00; border-color: #ffaa00; }
  .close-btn:hover { color: #ff4444; border-color: #ff4444; }

  .tb-btn { background: transparent; border: 1px solid #333; color: #888; font-family: "JetBrains Mono", monospace; font-size: 10px; padding: 3px 8px; cursor: pointer; border-radius: 2px; }
  .tb-btn:hover { color: #ffaa00; border-color: #ffaa00; }

  .logs-body { flex: 1; overflow-y: auto; padding: 10px 12px; font-family: "JetBrains Mono", monospace; font-size: 10px; line-height: 1.7; min-height: 0; }
  .logs-empty { color: #333; text-align: center; padding: 32px 0; font-size: 11px; }

  .log-line { display: flex; gap: 10px; align-items: baseline; white-space: pre-wrap; word-break: break-all; }
  .log-time { color: #333; flex-shrink: 0; font-size: 9px; }
  .log-level { flex-shrink: 0; font-size: 9px; font-weight: 600; width: 38px; text-align: right; }
  .log-msg { flex: 1; }

  .logs-footer { display: flex; align-items: center; justify-content: space-between; padding: 6px 12px; border-top: 1px solid #1a1a1a; background: #0d0d0d; flex-shrink: 0; }
  .logs-count { font-size: 9px; color: #444; font-family: "JetBrains Mono", monospace; }

  .restore-pill { position: fixed; bottom: 12px; right: 428px; z-index: 1000; display: flex; align-items: center; gap: 8px; padding: 6px 10px; background: #0a0a0a; border: 1px solid #ffaa00; border-radius: 2px; color: #ffaa00; font-family: "JetBrains Mono", monospace; font-size: 11px; cursor: pointer; box-shadow: 0 0 12px rgba(255, 170, 0, 0.3); }
  .restore-pill:hover { background: #111; box-shadow: 0 0 16px rgba(255, 170, 0, 0.5); }
  .pill-icon { font-size: 13px; }
  .pill-text { max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pill-close { color: #888; padding: 0 4px; font-size: 14px; line-height: 1; }
  .pill-close:hover { color: #ff4444; }
</style>
