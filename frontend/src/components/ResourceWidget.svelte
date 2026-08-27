<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { GetServerStats } from "../../wailsjs/go/main/App";
  import { formatBytes } from "../format";
  import { createVisibilityPoller } from "../lib/visibilityPoller.js";

  // The tunnel must be connected; the parent only mounts this when so.
  export let tunnelId: string;

  const POLL_MS = 3000;

  let cpu = 0;
  let hasCpu = false;
  let memTotal = 0;
  let memUsed = 0;
  let ready = false;
  let unsupported = false;
  let poller: ReturnType<typeof createVisibilityPoller> | null = null;

  async function poll() {
    try {
      const s = await GetServerStats(tunnelId);
      // No /proc on the remote (non-Linux) — total comes back 0.
      if (!s.memTotal && !s.hasCPU) {
        unsupported = true;
        return;
      }
      cpu = s.cpuPercent;
      hasCpu = s.hasCPU;
      memTotal = s.memTotal;
      memUsed = s.memUsed;
      ready = true;
      unsupported = false;
    } catch (e) {
      // Transient (reconnecting, etc.) — keep last values, don't spam console.
    }
  }

  onMount(() => {
    poller = createVisibilityPoller(poll, POLL_MS);
    poller.start();
  });

  onDestroy(() => {
    poller?.stop();
  });

  $: memPct = memTotal > 0 ? (memUsed / memTotal) * 100 : 0;
</script>

{#if !unsupported && (ready || hasCpu)}
  <div class="resource-widget">
    <div class="metric" title="CPU usage">
      <span class="metric-label">cpu</span>
      <div class="bar">
        <div class="bar-fill cpu" style={`width:${hasCpu ? cpu : 0}%`}></div>
      </div>
      <span class="metric-value">{hasCpu ? cpu.toFixed(0) + "%" : "··"}</span>
    </div>
    <div class="metric" title="RAM usage">
      <span class="metric-label">ram</span>
      <div class="bar">
        <div class="bar-fill mem" style={`width:${memPct}%`}></div>
      </div>
      <span class="metric-value">{formatBytes(memUsed)}/{formatBytes(memTotal)}</span>
    </div>
  </div>
{/if}

<style>
  .resource-widget {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 14px;
    margin: 4px 0 2px;
    font-family: "JetBrains Mono", monospace;
    font-size: 9px;
  }

  .metric {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }

  .metric-label {
    color: var(--muted);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    width: 22px;
    flex-shrink: 0;
  }

  .bar {
    width: 64px;
    height: 6px;
    background: var(--surface2);
    border: 1px solid var(--border);
    border-radius: 2px;
    overflow: hidden;
    flex-shrink: 0;
  }

  .bar-fill {
    height: 100%;
    transition: width 0.4s ease;
  }

  .bar-fill.cpu {
    background: var(--accent2);
  }

  .bar-fill.mem {
    background: var(--accent);
  }

  .metric-value {
    color: var(--text);
    white-space: nowrap;
  }
</style>
