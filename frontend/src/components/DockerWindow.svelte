<script lang="ts">
  import { errorText } from "../lib/runtime";
  import { onDestroy } from "svelte";
  import { GetDockerContainerDetails, ListDockerContainers } from "../../bindings/ssh-tunnel-manager/app";
  import type * as sysstats from "../../bindings/ssh-tunnel-manager/internal/sysstats/models";
  import { createVisibilityPoller } from "../lib/visibilityPoller.js";
  import { sortContainers } from "../lib/toolTables.js";
  import type { TunnelConfig } from "../types";
  import Icon from "./Icon.svelte";

  export let tunnel: TunnelConfig;
  export let active = true;
  export let intervalSeconds = 0;

  let containers: sysstats.DockerContainer[] = [];
  let loading = true;
  let refreshing = false;
  let error = "";
  let filter = "";
  let stoppedExpanded = false;
  let envExpanded = false;
  let poller: ReturnType<typeof createVisibilityPoller> | null = null;

  type SortKey = "name" | "cpu" | "mem";
  let sortBy: SortKey = "name";

  // CPU and RAM come from `docker stats`, which is slow, so ask only when shown
  // by a sort or by an interval the user chose.
  $: needsStats = sortBy !== "name" || intervalSeconds > 0;

  let selected: sysstats.DockerContainer | null = null;
  let details: sysstats.DockerContainerDetails | null = null;
  let detailLoading = false;
  let detailError = "";

  $: filtered = containers.filter((c) => {
    const needle = filter.trim().toLowerCase();
    return !needle || c.name.toLowerCase().includes(needle) || c.image.toLowerCase().includes(needle);
  });
  $: running = sortContainers(filtered.filter(isRunning), sortBy);
  $: stopped = sortContainers(filtered.filter((c) => !isRunning(c)), sortBy);
  $: showUsage = containers.some((c) => c.cpuPercent || c.memPercent);

  async function refresh() {
    // Docker commands can be slow on small hosts, so requests never overlap.
    if (refreshing) return;
    refreshing = true;
    try {
      containers = (await ListDockerContainers(tunnel.id, needsStats)) ?? [];
      error = "";
    } catch (e: any) {
      error = errorText(e);
    } finally {
      loading = false;
      refreshing = false;
    }
  }

  function isRunning(c: sysstats.DockerContainer): boolean {
    return c.state === "running";
  }

  async function openDetails(c: sysstats.DockerContainer) {
    selected = c;
    details = null;
    detailError = "";
    detailLoading = true;
    envExpanded = false;
    try {
      details = await GetDockerContainerDetails(tunnel.id, c.id);
    } catch (e: any) {
      detailError = errorText(e);
    } finally {
      detailLoading = false;
    }
  }

  function setSort(key: string) {
    sortBy = key as SortKey;
    void refresh();
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
  <div class="split" class:with-detail={selected}>
    <div class="left">
      <div class="tool-bar">
        <input class="field-control grow" placeholder="Filter containers" aria-label="Filter containers" bind:value={filter} />
        <span class="segmented" data-testid="docker-sort" aria-label="Sort containers">
          {#each [["name", "Name"], ["cpu", "CPU"], ["mem", "RAM"]] as [key, label]}
            <button type="button" class:on={sortBy === key} on:click={() => setSort(key)}>{label}</button>
          {/each}
        </span>
        <button type="button" class="icon-btn" aria-label="Refresh" disabled={refreshing} data-testid="docker-refresh" on:click={refresh}><Icon name="refresh" /></button>
      </div>
      <div class="tool-body">
        {#if loading}
          <p class="tool-message">Listing containers</p>
        {:else if error && !containers.length}
          <p class="tool-message error">{error}</p>
          <p class="tool-message"><button type="button" class="btn" on:click={refresh}>Retry</button></p>
        {:else if !containers.length}
          <p class="tool-message">No containers on this host.</p>
        {:else}
          <div class="cards" data-testid="docker-list">
            {#each running as c (c.id)}
              <button type="button" class="card" class:selected={selected?.id === c.id} data-testid="docker-card" on:click={() => openDetails(c)}>
                <span class="card-name" title={c.name}><i class="dot up"></i><strong>{c.name}</strong></span>
                <span class="card-line" title={c.image}>{c.image}</span>
                <span class="card-foot">
                  <span class="card-line" title={c.status}>{c.status}</span>
                  {#if showUsage && (c.cpuPercent || c.memPercent)}<span class="usage">{c.cpuPercent || "0%"} CPU · {c.memPercent || "0%"} RAM</span>{/if}
                </span>
              </button>
            {/each}
            {#if stopped.length}
              <button type="button" class="stopped-toggle" on:click={() => (stoppedExpanded = !stoppedExpanded)}>
                <span class:down={stoppedExpanded} class="chevron"><Icon name="chevron" size={12} /></span>Stopped {stopped.length}
              </button>
              {#if stoppedExpanded}
                {#each stopped as c (c.id)}
                  <button type="button" class="card" class:selected={selected?.id === c.id} data-testid="docker-card" on:click={() => openDetails(c)}>
                    <span class="card-name" title={c.name}><i class="dot"></i><strong>{c.name}</strong></span>
                    <span class="card-line" title={c.image}>{c.image}</span>
                    <span class="card-foot"><span class="card-line" title={c.status}>{c.status}</span></span>
                  </button>
                {/each}
              {/if}
            {/if}
          </div>
        {/if}
      </div>
    </div>

    {#if selected}
      <aside class="detail" data-testid="docker-detail">
        <div class="detail-head">
          <i class="dot" class:up={isRunning(selected)}></i><strong title={selected.name}>{selected.name}</strong>
          <small>{selected.state}</small>
          <button type="button" class="icon-btn" aria-label="Close details" on:click={() => (selected = null)}><Icon name="close" size={12} /></button>
        </div>
        {#if detailLoading}
          <p class="note">Loading details</p>
        {:else if detailError}
          <p class="note error">{detailError}</p>
        {:else if details}
          {#if details.hasStats}
            <dl>
              <dt>CPU</dt><dd>{details.cpuPercent}</dd>
              <dt>Memory</dt><dd>{details.memPercent}, {details.memUsage}</dd>
              <dt>Net I/O</dt><dd>{details.netIO}</dd>
              <dt>Block I/O</dt><dd>{details.blockIO}</dd>
              <dt>PIDs</dt><dd>{details.pids}</dd>
            </dl>
          {/if}
          <dl>
            <dt>Status</dt><dd title={selected.status}>{selected.status}</dd>
            <dt>Image</dt><dd title={details.image}>{details.image}</dd>
            {#if details.created}<dt>Created</dt><dd>{details.created}</dd>{/if}
            {#if details.command}<dt>Command</dt><dd title={details.command}>{details.command}</dd>{/if}
            <dt>Restarts</dt><dd>{details.restartCount}</dd>
            {#if details.networks?.length}<dt>Networks</dt><dd>{details.networks.join(", ")}</dd>{/if}
          </dl>
          <p class="section">Ports</p>
          {#if details.ports?.length}
            <dl>
              {#each details.ports as p}
                <dt>{p.containerPort}</dt>
                <dd>{p.hostPort ? `to ${p.hostIp || "0.0.0.0"}:${p.hostPort}` : "not published"}</dd>
              {/each}
            </dl>
          {:else}
            <p class="note">No published ports</p>
          {/if}
          <p class="section">Mounts</p>
          {#if details.mounts?.length}
            <dl>
              {#each details.mounts as m}
                <dt title={m.destination}>{m.destination}</dt>
                <dd title={m.source}>{m.source}{m.mode ? ` (${m.mode})` : ""}</dd>
              {/each}
            </dl>
          {:else}
            <p class="note">No mounts</p>
          {/if}
          {#if details.env?.length}
            <button type="button" class="env-toggle" data-testid="docker-env" on:click={() => (envExpanded = !envExpanded)}>
              <span class:down={envExpanded} class="chevron"><Icon name="chevron" size={12} /></span>Env ({details.env.length})
            </button>
            {#if envExpanded}
              <div class="env">
                {#each details.env as e}<div title={e}>{e}</div>{/each}
              </div>
            {/if}
          {/if}
        {/if}
      </aside>
    {/if}
  </div>
</div>

<style>
  .split {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: 1fr;
  }
  .split.with-detail {
    grid-template-columns: minmax(0, 1fr) 320px;
  }
  /* In a narrow window the details take the whole window. */
  @media (max-width: 720px) {
    .split.with-detail {
      grid-template-columns: minmax(0, 1fr);
    }
    .split.with-detail .left {
      display: none;
    }
  }
  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: 8px;
    padding: 10px;
  }
  .card {
    display: flex;
    flex-direction: column;
    gap: 5px;
    min-width: 0;
    padding: 9px 11px;
    text-align: left;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .card:hover {
    border-color: var(--accent2);
  }
  .card.selected {
    border-color: var(--accent);
  }
  .card-name {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .card-name strong {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 600;
  }
  .card-line {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font: 11.5px var(--mono);
    color: var(--muted);
  }
  .card-foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    min-width: 0;
  }
  .usage {
    flex: none;
    font: 11.5px var(--mono);
  }
  .stopped-toggle {
    grid-column: 1 / -1;
    padding: 6px 2px;
    text-align: left;
    color: var(--muted);
  }
  .left {
    min-width: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .dot {
    display: inline-block;
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    border: 1.5px solid var(--muted);
  }
  .dot.up {
    background: var(--live);
    border-color: var(--live);
    box-shadow: var(--glow);
  }
  .chevron {
    display: inline-flex;
    margin-right: 6px;
    vertical-align: -2px;
  }
  .chevron.down {
    transform: rotate(90deg);
  }
  .detail {
    overflow-y: auto;
    padding: 10px 12px;
    font-size: 12.5px;
    background: var(--surface);
    border-left: 1px solid var(--border);
  }
  .detail-head {
    display: flex;
    align-items: center;
    gap: 7px;
    margin-bottom: 8px;
    font-size: 13.5px;
  }
  .detail-head strong {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .detail-head small {
    flex: none;
    color: var(--muted);
    font-size: 12px;
  }
  .detail-head .icon-btn {
    flex: none;
  }
  dl {
    display: grid;
    grid-template-columns: 76px 1fr;
    gap: 3px 8px;
    margin: 0 0 10px;
  }
  dt {
    color: var(--muted);
    font-size: 12px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  dd {
    margin: 0;
    font: 11.5px var(--mono);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .section {
    margin: 10px 0 4px;
    color: var(--muted);
    font-size: 12px;
  }
  .note {
    margin: 0 0 8px;
    color: var(--muted);
  }
  .note.error {
    color: var(--error);
  }
  .env-toggle {
    width: 100%;
    padding-top: 8px;
    text-align: left;
    color: var(--muted);
    border-top: 1px solid var(--border);
  }
  .env div {
    margin-top: 4px;
    font: 11.5px var(--mono);
    word-break: break-all;
  }
</style>
