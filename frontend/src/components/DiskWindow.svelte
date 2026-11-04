<script lang="ts">
  import { errorText } from "../lib/runtime";
  import { onDestroy } from "svelte";
  import { GetDiskUsage } from "../../bindings/ssh-tunnel-manager/app";
  import type * as sysstats from "../../bindings/ssh-tunnel-manager/internal/sysstats/models";
  import { formatBytes } from "../format";
  import { createVisibilityPoller } from "../lib/visibilityPoller.js";
  import type { TunnelConfig } from "../types";
  import Icon from "./Icon.svelte";

  export let tunnel: TunnelConfig;
  export let active = true;
  export let intervalSeconds = 5;

  let mounts: sysstats.DiskMount[] = [];
  let loading = true;
  let error = "";
  let poller: ReturnType<typeof createVisibilityPoller> | null = null;

  async function refresh() {
    try {
      mounts = (await GetDiskUsage(tunnel.id)) ?? [];
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
</script>

<div class="tool-root">
  <div class="tool-bar">
    <span class="grow">{mounts.length} filesystem{mounts.length === 1 ? "" : "s"}</span>
    <button type="button" class="icon-btn" aria-label="Refresh" on:click={refresh}><Icon name="refresh" /></button>
  </div>
  <div class="tool-body">
    {#if loading}
      <p class="tool-message">Reading disk usage</p>
    {:else if error}
      <p class="tool-message error">{error}</p>
    {:else if !mounts.length}
      <p class="tool-message">No filesystems found.</p>
    {:else}
      <div class="cards" data-testid="disk-list">
        {#each mounts as m}
          <div class="card" data-testid="disk-card">
            <span class="card-head">
              <strong title={m.mountPoint}>{m.mountPoint}</strong>
              <span class="percent">{m.usePercent.toFixed(0)}%</span>
            </span>
            <span class="card-line" title={m.filesystem}>{m.filesystem}</span>
            <i class="meter-track"><i class="meter-fill" class:warn={m.usePercent >= 80 && m.usePercent < 90} class:crit={m.usePercent >= 90} style={`width:${Math.min(100, m.usePercent)}%`}></i></i>
            <span class="card-line">{formatBytes(m.used)} used · {formatBytes(m.avail)} free · {formatBytes(m.total)}</span>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>

<style>
  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: 8px;
    padding: 10px;
  }
  .card {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 0;
    padding: 9px 11px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .card-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 10px;
    min-width: 0;
  }
  .card-head strong {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 600;
  }
  .percent {
    flex: none;
    font: 12px var(--mono);
  }
  .card-line {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font: 11.5px var(--mono);
    color: var(--muted);
  }
</style>
