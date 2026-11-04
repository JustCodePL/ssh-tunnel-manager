<script lang="ts">
  import { errorText } from "../lib/runtime";
  import { onDestroy } from "svelte";
  import { GetProcessStats } from "../../bindings/ssh-tunnel-manager/app";
  import type * as sysstats from "../../bindings/ssh-tunnel-manager/internal/sysstats/models";
  import { formatBytes } from "../format";
  import { createVisibilityPoller } from "../lib/visibilityPoller.js";
  import { sortProcesses, scaleBar } from "../lib/toolTables.js";
  import type { TunnelConfig } from "../types";
  import Icon from "./Icon.svelte";

  export let tunnel: TunnelConfig;
  export let active = true;
  export let intervalSeconds = 3;

  let stats: sysstats.ProcessStats | null = null;
  let loading = true;
  let error = "";
  let poller: ReturnType<typeof createVisibilityPoller> | null = null;

  type SortKey = "cpu" | "mem" | "name" | "pid";
  let sortBy: SortKey = "cpu";

  $: processes = stats ? sortProcesses(stats.processes, sortBy) : [];
  $: maxCpu = Math.max(1, ...processes.map((p) => p.cpu));
  $: maxMem = Math.max(1, ...processes.map((p) => p.mem));
  $: memPct = stats && stats.memTotal > 0 ? (stats.memUsed / stats.memTotal) * 100 : 0;
  $: swapPct = stats && stats.swapTotal > 0 ? (stats.swapUsed / stats.swapTotal) * 100 : 0;

  async function refresh() {
    try {
      stats = await GetProcessStats(tunnel.id);
      error = "";
    } catch (e: any) {
      error = errorText(e);
    } finally {
      loading = false;
    }
  }

  function restartPoller(enabled: boolean, seconds: number) {
    poller?.stop();
    poller = null;
    if (!enabled) return;
    if (seconds > 0) {
      poller = createVisibilityPoller(refresh, seconds * 1000);
      poller.start();
    } else if (loading) {
      void refresh();
    }
  }

  $: restartPoller(active, intervalSeconds);
  onDestroy(() => poller?.stop());

  function loadClass(value: number): string {
    if (value >= 80) return "crit";
    if (value >= 50) return "warn";
    return "";
  }

  function coreLabel(name: string): string {
    return name === "cpu" ? "all" : name.replace("cpu", "");
  }

  function formatUptime(seconds: number): string {
    const d = Math.floor(seconds / 86400);
    const h = Math.floor((seconds % 86400) / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    return d > 0 ? `${d}d ${h}h ${m}m` : h > 0 ? `${h}h ${m}m` : `${m}m`;
  }

  const sortable: { key: SortKey; label: string; numeric?: boolean }[] = [
    { key: "name", label: "Process" },
  ];
</script>

<div class="tool-root">
  {#if loading}
    <p class="tool-message">Reading process stats</p>
  {:else if error && !stats}
    <p class="tool-message error">{error}</p>
  {:else if stats}
    <div class="summary">
      <div class="facts">
        <span>Load<b>{stats.load1.toFixed(2)} {stats.load5.toFixed(2)} {stats.load15.toFixed(2)}</b></span>
        <span>Uptime<b>{formatUptime(stats.uptimeSeconds)}</b></span>
        <span>Tasks<b>{stats.processes.length}</b></span>
        <button type="button" class="icon-btn" aria-label="Refresh" on:click={refresh}><Icon name="refresh" /></button>
      </div>
      <div class="meters">
        <div class="meter-col">
          {#each stats.cores as core}
            <div class="meter-line">
              <span>{coreLabel(core.name)}</span>
              <i class="meter-track"><i class="meter-fill {loadClass(core.percent)}" style={`width:${Math.min(core.percent, 100)}%`}></i></i>
              <em>{core.percent.toFixed(0)}%</em>
            </div>
          {/each}
        </div>
        <div class="meter-col">
          <div class="meter-line">
            <span>ram</span>
            <i class="meter-track"><i class="meter-fill mem {loadClass(memPct)}" style={`width:${memPct}%`}></i></i>
            <em>{formatBytes(stats.memUsed)}/{formatBytes(stats.memTotal)}</em>
          </div>
          {#if stats.swapTotal > 0}
            <div class="meter-line">
              <span>swap</span>
              <i class="meter-track"><i class="meter-fill mem {loadClass(swapPct)}" style={`width:${swapPct}%`}></i></i>
              <em>{formatBytes(stats.swapUsed)}/{formatBytes(stats.swapTotal)}</em>
            </div>
          {/if}
        </div>
      </div>
    </div>
    <div class="tool-body">
      <table class="data-table" data-testid="process-table">
        <thead>
          <tr>
            <th class="sortable" class:sorted={sortBy === "name"} on:click={() => (sortBy = "name")}>Process</th>
            <th>User</th>
            <th class="num sortable" class:sorted={sortBy === "pid"} on:click={() => (sortBy = "pid")}>PID</th>
            <th class="num sortable" class:sorted={sortBy === "cpu"} on:click={() => (sortBy = "cpu")}>CPU</th>
            <th class="num sortable" class:sorted={sortBy === "mem"} on:click={() => (sortBy = "mem")}>Mem</th>
          </tr>
        </thead>
        <tbody>
          {#each processes as p (p.pid)}
            <tr>
              <td class="command" title={p.command}>{p.command}</td>
              <td class="dim">{p.user}</td>
              <td class="num">{p.pid}</td>
              <td>
                <div class="cell">
                  <i class="meter-track"><i class="meter-fill {loadClass(p.cpu)}" style={`width:${scaleBar(p.cpu, maxCpu)}%`}></i></i>
                  <span class="num">{p.cpu.toFixed(1)}%</span>
                </div>
              </td>
              <td>
                <div class="cell">
                  <i class="meter-track"><i class="meter-fill mem" style={`width:${scaleBar(p.mem, maxMem)}%`}></i></i>
                  <span class="num">{p.mem.toFixed(1)}%</span>
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<style>
  .summary {
    padding: 10px 12px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
    font-size: 12px;
  }
  .facts {
    display: flex;
    align-items: center;
    gap: 14px;
    color: var(--muted);
    margin-bottom: 6px;
  }
  .facts b {
    margin-left: 5px;
    font: 500 12px var(--mono);
    color: var(--text);
  }
  .facts button {
    margin-left: auto;
  }
  .meters {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 22px;
  }
  .meter-col {
    display: grid;
    gap: 4px;
    align-content: start;
  }
  .meter-line {
    display: grid;
    grid-template-columns: 32px 1fr 92px;
    align-items: center;
    gap: 7px;
    font: 11.5px var(--mono);
    color: var(--muted);
  }
  .meter-line em {
    font-style: normal;
    text-align: right;
    color: var(--text);
  }
  .command {
    max-width: 280px;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .cell {
    display: grid;
    grid-template-columns: 56px 42px;
    align-items: center;
    justify-content: end;
    gap: 7px;
  }
  .cell .num {
    text-align: right;
    font: 11.5px var(--mono);
  }
  th.num {
    text-align: right;
  }
</style>
