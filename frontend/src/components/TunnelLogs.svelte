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

  onMount(async () => {
    try {
      const initial = await GetTunnelLogs(tunnelId);
      logs = initial || [];
      scrollToBottom();
    } catch (e) {
      console.error("GetTunnelLogs failed:", e);
    }
    EventsOn("tunnel:log", handleLogEvent);
  });

  onDestroy(() => {
    EventsOff("tunnel:log");
  });

  $: if (logs.length > 0) {
    scrollToBottom();
  }
</script>

<div
  class="logs-backdrop"
  on:click|self={() => dispatch("close")}
  on:keydown={(e) => e.key === "Escape" && dispatch("close")}
  role="dialog"
  aria-modal="true"
  tabindex="-1"
>
  <div class="logs-panel">
    <div class="logs-header">
      <div class="logs-title">
        <span class="logs-label">// logs</span>
        <span class="logs-tunnel-name">{tunnelName}</span>
      </div>
      <div class="logs-actions">
        <button class="log-btn" on:click={handleClear}>[ clear ]</button>
        <button class="log-btn close" on:click={() => dispatch("close")}>[ × ]</button>
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
          class="log-btn"
          on:click={() => { autoScroll = true; scrollToBottom(); }}
        >
          ↓ scroll to bottom
        </button>
      {/if}
    </div>
  </div>
</div>

<style>
  .logs-backdrop {
    position: fixed;
    inset: 0;
    z-index: 50;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.8);
  }

  .logs-panel {
    width: 100%;
    max-width: 700px;
    margin: 0 16px;
    background: #050505;
    border: 1px solid var(--border);
    border-radius: 2px;
    display: flex;
    flex-direction: column;
    max-height: 75vh;
  }

  .logs-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }

  .logs-title {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .logs-label {
    font-size: 10px;
    color: var(--accent);
    font-family: 'JetBrains Mono', monospace;
    letter-spacing: 0.05em;
  }

  .logs-tunnel-name {
    font-size: 10px;
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
  }

  .logs-actions {
    display: flex;
    gap: 4px;
  }

  .log-btn {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    font-size: 9px;
    padding: 2px 7px;
    cursor: pointer;
    border-radius: 2px;
    transition: color 0.15s, border-color 0.15s;
    letter-spacing: 0.05em;
  }

  .log-btn:hover {
    color: var(--text);
    border-color: var(--muted);
  }

  .log-btn.close:hover {
    color: #ff4444;
    border-color: #ff4444;
  }

  .logs-body {
    flex: 1;
    overflow-y: auto;
    padding: 10px 12px;
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    line-height: 1.7;
    min-height: 0;
  }

  .logs-empty {
    color: #333;
    text-align: center;
    padding: 32px 0;
    font-size: 11px;
  }

  .log-line {
    display: flex;
    gap: 10px;
    align-items: baseline;
    white-space: pre-wrap;
    word-break: break-all;
  }

  .log-time {
    color: #333333;
    flex-shrink: 0;
    font-size: 9px;
    tabular-nums: numeric;
  }

  .log-level {
    flex-shrink: 0;
    font-size: 9px;
    font-weight: 600;
    width: 38px;
    text-align: right;
  }

  .log-msg {
    flex: 1;
  }

  .logs-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 6px 12px;
    border-top: 1px solid var(--border);
    flex-shrink: 0;
  }

  .logs-count {
    font-size: 9px;
    color: #333;
    font-family: 'JetBrains Mono', monospace;
  }
</style>
