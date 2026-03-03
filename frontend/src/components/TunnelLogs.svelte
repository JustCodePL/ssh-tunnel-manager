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

  function levelClass(level: string): string {
    switch (level) {
      case "info":  return "bg-blue-500/20 text-blue-300 border border-blue-500/30";
      case "warn":  return "bg-yellow-500/20 text-yellow-300 border border-yellow-500/30";
      case "error": return "bg-red-500/20 text-red-300 border border-red-500/30";
      default:      return "bg-zinc-500/20 text-zinc-300";
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

<!-- Backdrop -->
<div
  class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
  on:click|self={() => dispatch("close")}
  on:keydown={(e) => e.key === "Escape" && dispatch("close")}
  role="dialog"
  aria-modal="true"
  tabindex="-1"
>
  <!-- Panel -->
  <div class="relative w-full max-w-2xl mx-4 bg-zinc-900 border border-zinc-700 rounded-xl shadow-2xl flex flex-col max-h-[80vh]">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-zinc-700 shrink-0">
      <div>
        <h2 class="text-sm font-semibold text-zinc-100">Connection Logs</h2>
        <p class="text-xs text-zinc-400">{tunnelName}</p>
      </div>
      <div class="flex items-center gap-2">
        <button
          class="px-2.5 py-1 text-xs rounded bg-zinc-700 hover:bg-zinc-600 text-zinc-300"
          on:click={handleClear}
        >
          Clear
        </button>
        <button
          class="p-1 rounded hover:bg-zinc-700 text-zinc-400 hover:text-zinc-200"
          on:click={() => dispatch("close")}
          aria-label="Close"
        >
          <svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
          </svg>
        </button>
      </div>
    </div>

    <!-- Log entries -->
    <div
      bind:this={logContainer}
      on:scroll={handleScroll}
      class="flex-1 overflow-y-auto p-3 font-mono text-xs space-y-0.5 min-h-0"
    >
      {#if logs.length === 0}
        <p class="text-zinc-500 text-center py-8">No log entries yet.</p>
      {:else}
        {#each logs as entry (entry.timestamp + entry.message)}
          <div class="flex items-start gap-2 py-0.5">
            <span class="text-zinc-500 shrink-0 tabular-nums">[{formatTime(entry.timestamp)}]</span>
            <span class="shrink-0 px-1 py-0.5 rounded text-[10px] uppercase font-semibold leading-none {levelClass(entry.level)}">
              {entry.level}
            </span>
            <span class="text-zinc-300 break-all">{entry.message}</span>
          </div>
        {/each}
      {/if}
    </div>

    <!-- Footer -->
    <div class="px-4 py-2 border-t border-zinc-700 shrink-0 flex items-center justify-between">
      <span class="text-xs text-zinc-500">{logs.length} entr{logs.length !== 1 ? "ies" : "y"}</span>
      {#if !autoScroll}
        <button
          class="text-xs text-blue-400 hover:text-blue-300"
          on:click={() => { autoScroll = true; scrollToBottom(); }}
        >
          ↓ Scroll to bottom
        </button>
      {/if}
    </div>
  </div>
</div>
