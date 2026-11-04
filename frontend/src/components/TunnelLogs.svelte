<script lang="ts">
  import { onMount, onDestroy, tick } from "svelte";
  import { createEventDispatcher } from "svelte";
  import type { LogEntry } from "../types";
  import { GetTunnelLogs, ClearTunnelLogs } from "../../bindings/ssh-tunnel-manager/app";
  import { createEventScope } from "../lib/runtime";
  const events = createEventScope();

  export let tunnelName: string;
  export let tunnelId: string;


  let logs: LogEntry[] = [];
  let logContainer: HTMLDivElement;
  let autoScroll = true;
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
      case "info":  return "var(--accent2)";
      case "warn":  return "var(--warning)";
      case "error": return "var(--error)";
      default:      return "var(--muted)";
    }
  }

  function msgColor(level: string): string {
    switch (level) {
      case "error": return "var(--error)";
      case "warn":  return "var(--warning)";
      default:      return "var(--muted)";
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
      logs = (initial || []) as unknown as LogEntry[];
      scrollToBottom();
    } catch (e) {
      console.error("GetTunnelLogs failed:", e);
    }
    events.on("tunnel:log", handleLogEvent);

    // Keep view pinned to bottom when window/container is resized.
    if (logContainer && typeof ResizeObserver !== "undefined") {
      resizeObserver = new ResizeObserver(() => scrollToBottom());
      resizeObserver.observe(logContainer);
    }
  });

  onDestroy(() => {
    events.off();
    resizeObserver?.disconnect();
  });

  $: if (logs.length > 0) {
    scrollToBottom();
  }
</script>

<div class="tool-root" data-testid="logs">
  <div class="tool-bar">
    <span class="grow">{logs.length} entr{logs.length !== 1 ? "ies" : "y"} on {tunnelName}</span>
    {#if !autoScroll}
      <button type="button" class="btn small" on:click={() => { autoScroll = true; scrollToBottom(); }}>Scroll to bottom</button>
    {/if}
    <button type="button" class="btn small" title="Clear all log entries" on:click={handleClear}>Clear</button>
  </div>
  <div class="tool-body logs" bind:this={logContainer} on:scroll={handleScroll}>
    {#if logs.length === 0}
      <p class="tool-message">No log entries yet.</p>
    {:else}
      {#each logs as entry (entry.timestamp + entry.message)}
        <div class="line">
          <span class="time">{formatTime(entry.timestamp)}</span>
          <span class="level" style="color: {levelColor(entry.level)};">{entry.level}</span>
          <span style="color: {msgColor(entry.level)};">{entry.message}</span>
        </div>
      {/each}
    {/if}
  </div>
</div>

<style>
  .logs {
    padding: 6px 10px;
    font: 12px/1.5 var(--mono);
  }
  .line {
    display: flex;
    gap: 8px;
    word-break: break-word;
  }
  .time {
    color: var(--muted);
    flex: none;
  }
  .level {
    flex: none;
    width: 40px;
  }
</style>
