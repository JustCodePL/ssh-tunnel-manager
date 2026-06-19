<script lang="ts">
  import { onMount, createEventDispatcher } from "svelte";
  import { ListDockerContainers, GetDockerContainerDetails } from "../../wailsjs/go/main/App";
  import type { sysstats } from "../../wailsjs/go/models";
  import type { TunnelConfig } from "../types";

  export let tunnel: TunnelConfig;
  const dispatch = createEventDispatcher<{ close: void }>();

  let containers: sysstats.DockerContainer[] = [];
  let loading = true;
  let refreshing = false;
  let error = "";
  let minimized = false;
  let stoppedExpanded = false;

  type SortKey = "name" | "cpu" | "mem";
  let sortBy: SortKey = "name";

  // CPU/RAM sorting needs `docker stats`, which is slow; only request it then.
  $: needsStats = sortBy === "cpu" || sortBy === "mem";

  // Detail view state.
  let selected: sysstats.DockerContainer | null = null;
  let details: sysstats.DockerContainerDetails | null = null;
  let detailLoading = false;
  let detailError = "";

  function pct(s: string): number {
    const v = parseFloat((s ?? "").replace("%", ""));
    return isNaN(v) ? 0 : v;
  }

  function sortContainers(list: sysstats.DockerContainer[]): sysstats.DockerContainer[] {
    const arr = [...list];
    if (sortBy === "cpu") {
      arr.sort((a, b) => pct(b.cpuPercent) - pct(a.cpuPercent) || a.name.localeCompare(b.name));
    } else if (sortBy === "mem") {
      arr.sort((a, b) => pct(b.memPercent) - pct(a.memPercent) || a.name.localeCompare(b.name));
    } else {
      arr.sort((a, b) => a.name.localeCompare(b.name));
    }
    return arr;
  }

  $: running = sortContainers(containers.filter(isRunning));
  $: stopped = sortContainers(containers.filter((c) => !isRunning(c)));

  async function refresh() {
    // Docker commands can be slow on small hosts; guard against overlapping
    // requests so a click while one is in flight doesn't stack them.
    if (refreshing) return;
    refreshing = true;
    try {
      containers = await ListDockerContainers(tunnel.id, needsStats);
      error = "";
    } catch (e: any) {
      error = e?.toString() ?? "failed to list containers";
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
    try {
      details = await GetDockerContainerDetails(tunnel.id, c.id);
    } catch (e: any) {
      detailError = e?.toString() ?? "failed to load details";
    } finally {
      detailLoading = false;
    }
  }

  function closeDetails() {
    selected = null;
    details = null;
    detailError = "";
  }

  // Changing the sort re-checks live usage so cpu/ram ordering reflects current
  // numbers; the centered spinner makes the in-flight check visible.
  function setSort(key: SortKey) {
    sortBy = key;
    refresh();
  }

  onMount(() => {
    // No auto-refresh: docker listing is slow on small hosts, so refresh only
    // on first open and when the user clicks ⟳ (or changes the sort).
    refresh();
  });

  function handleClose() {
    dispatch("close");
  }
</script>

<div class="overlay" class:hidden={minimized}>
  <div class="window">
    <div class="titlebar">
      <span class="title">docker — {tunnel.name} ({tunnel.user}@{tunnel.host})</span>
      <div class="actions">
        {#if selected}
          <button class="title-btn" on:click={closeDetails} title="Back to list">←</button>
        {:else}
          <button class="title-btn" on:click={refresh} disabled={refreshing} title="Refresh">
            {#if refreshing}<span class="spinner"></span>{:else}⟳{/if}
          </button>
        {/if}
        <button class="title-btn" on:click={() => (minimized = true)} title="Minimize">−</button>
        <button class="close-btn" on:click={handleClose} title="Close">×</button>
      </div>
    </div>
    {#if refreshing && !loading && !selected && containers.length > 0}
      <div class="refresh-overlay">
        <span class="spinner big"></span>
        <span class="refresh-text">checking…</span>
      </div>
    {/if}
    <div class="body">
      {#if selected}
        <!-- Detail view -->
        <div class="detail-head">
          <span class="name" title={selected.name}>
            <span class="dot" class:up={isRunning(selected)}></span>{selected.name}
          </span>
          <span class="state" class:up={isRunning(selected)}>{selected.state}</span>
        </div>
        {#if detailLoading}
          <div class="status-msg">loading details...</div>
        {:else if detailError}
          <div class="status-msg error">{detailError}</div>
        {:else if details}
          {#if details.hasStats}
            <div class="section-label">usage</div>
            <div class="usage-grid">
              <div class="metric"><span class="m-label">cpu</span><span class="m-val">{details.cpuPercent}</span></div>
              <div class="metric"><span class="m-label">mem</span><span class="m-val">{details.memPercent}</span></div>
              <div class="metric"><span class="m-label">mem usage</span><span class="m-val">{details.memUsage}</span></div>
              <div class="metric"><span class="m-label">net i/o</span><span class="m-val">{details.netIO}</span></div>
              <div class="metric"><span class="m-label">block i/o</span><span class="m-val">{details.blockIO}</span></div>
              <div class="metric"><span class="m-label">pids</span><span class="m-val">{details.pids}</span></div>
            </div>
          {/if}

          <div class="section-label">info</div>
          <div class="kv-list">
            <div class="kv"><span class="k">status</span><span class="v">{selected.status}</span></div>
            <div class="kv"><span class="k">image</span><span class="v" title={details.image}>{details.image}</span></div>
            {#if details.created}
              <div class="kv"><span class="k">created</span><span class="v">{details.created}</span></div>
            {/if}
            {#if details.command}
              <div class="kv"><span class="k">command</span><span class="v" title={details.command}>{details.command}</span></div>
            {/if}
            <div class="kv"><span class="k">restarts</span><span class="v">{details.restartCount}</span></div>
            {#if details.networks && details.networks.length}
              <div class="kv"><span class="k">networks</span><span class="v">{details.networks.join(", ")}</span></div>
            {/if}
          </div>

          <div class="section-label">ports</div>
          {#if details.ports && details.ports.length}
            <div class="kv-list">
              {#each details.ports as p}
                <div class="kv">
                  <span class="k">{p.containerPort}</span>
                  <span class="v">
                    {#if p.hostPort}
                      → {p.hostIp || "0.0.0.0"}:{p.hostPort}
                    {:else}
                      <span class="muted">exposed (not published)</span>
                    {/if}
                  </span>
                </div>
              {/each}
            </div>
          {:else}
            <div class="empty">no published ports</div>
          {/if}

          <div class="section-label">mounts</div>
          {#if details.mounts && details.mounts.length}
            <div class="kv-list">
              {#each details.mounts as m}
                <div class="kv">
                  <span class="k">{m.destination}</span>
                  <span class="v" title={m.source}>{m.source}{#if m.mode} <span class="muted">({m.mode})</span>{/if}</span>
                </div>
              {/each}
            </div>
          {:else}
            <div class="empty">no mounts</div>
          {/if}

          {#if details.env && details.env.length}
            <div class="section-label">env ({details.env.length})</div>
            <div class="env-list">
              {#each details.env as e}
                <div class="env-row" title={e}>{e}</div>
              {/each}
            </div>
          {/if}
        {/if}
      {:else if loading}
        <div class="status-msg">listing containers...</div>
      {:else if error && containers.length === 0}
        <div class="status-msg error">{error}</div>
        <button class="retry-btn" on:click={refresh} disabled={refreshing}>
          {refreshing ? "retrying..." : "retry"}
        </button>
      {:else if containers.length === 0}
        <div class="status-msg">no containers</div>
      {:else}
        <!-- List view -->
        <div class="sortbar">
          <span class="sort-label">sort</span>
          <button class="sort-btn" class:active={sortBy === "name"} on:click={() => setSort("name")}>name</button>
          <button class="sort-btn" class:active={sortBy === "cpu"} on:click={() => setSort("cpu")}>cpu</button>
          <button class="sort-btn" class:active={sortBy === "mem"} on:click={() => setSort("mem")}>ram</button>
        </div>
        {#if running.length}
          <div class="group-label">active ({running.length})</div>
          <div class="cards">
            {#each running as c}
              <button class="card running" on:click={() => openDetails(c)} title="View details">
                <div class="card-head">
                  <span class="name" title={c.name}>
                    <span class="dot up"></span>{c.name}
                  </span>
                  <span class="state up">{c.state}</span>
                </div>
                <div class="image" title={c.image}>{c.image}</div>
                <div class="status">{c.status}</div>
                {#if c.cpuPercent || c.memPercent}
                  <div class="usage">
                    <span>cpu {c.cpuPercent || "—"}</span>
                    <span>ram {c.memPercent || "—"}</span>
                  </div>
                {/if}
                {#if c.ports}
                  <div class="ports" title={c.ports}>{c.ports}</div>
                {/if}
              </button>
            {/each}
          </div>
        {/if}

        {#if stopped.length}
          <button class="group-label toggle" on:click={() => (stoppedExpanded = !stoppedExpanded)}>
            <span class="chevron" class:open={stoppedExpanded}>▶</span>
            stopped ({stopped.length})
          </button>
          {#if stoppedExpanded}
            <div class="cards">
              {#each stopped as c}
                <button class="card" on:click={() => openDetails(c)} title="View details">
                  <div class="card-head">
                    <span class="name" title={c.name}>
                      <span class="dot"></span>{c.name}
                    </span>
                    <span class="state">{c.state}</span>
                  </div>
                  <div class="image" title={c.image}>{c.image}</div>
                  <div class="status">{c.status}</div>
                  {#if c.ports}
                    <div class="ports" title={c.ports}>{c.ports}</div>
                  {/if}
                </button>
              {/each}
            </div>
          {/if}
        {/if}
      {/if}
    </div>
  </div>
</div>

{#if minimized}
  <button class="restore-pill" on:click={() => (minimized = false)} title="Restore docker">
    <span class="pill-icon">▢</span>
    <span class="pill-text">docker — {tunnel.name}</span>
    <span class="pill-close" on:click|stopPropagation={handleClose} title="Close">×</span>
  </button>
{/if}

<style>
  .overlay { position: fixed; top: 36px; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.85); z-index: 1000; display: flex; align-items: center; justify-content: center; }
  .overlay.hidden { display: none; }
  .window { position: relative; width: 85vw; max-width: 1000px; height: 70vh; background: #0a0a0a; border: 1px solid #00ff88; border-radius: 2px; display: flex; flex-direction: column; }
  .refresh-overlay { position: absolute; top: 37px; left: 0; right: 0; bottom: 0; z-index: 5; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 12px; background: rgba(10, 10, 10, 0.55); backdrop-filter: blur(1px); }
  .refresh-text { font-family: "JetBrains Mono", monospace; font-size: 11px; color: #00d4ff; letter-spacing: 0.05em; }
  .spinner.big { width: 28px; height: 28px; border-width: 3px; }
  .titlebar { display: flex; justify-content: space-between; align-items: center; padding: 6px 12px; border-bottom: 1px solid #1a1a1a; background: #111; }
  .title { font-family: "JetBrains Mono", monospace; font-size: 11px; color: #00ff88; }
  .actions { display: flex; gap: 4px; }
  .title-btn, .close-btn { background: none; border: 1px solid #333; color: #888; font-size: 14px; width: 24px; height: 24px; cursor: pointer; border-radius: 2px; line-height: 1; padding: 0; }
  .title-btn:hover { color: #00d4ff; border-color: #00d4ff; }
  .title-btn:disabled { opacity: 0.6; cursor: default; }
  .spinner { display: inline-block; width: 11px; height: 11px; border: 2px solid #333; border-top-color: #00d4ff; border-radius: 50%; animation: spin 0.7s linear infinite; vertical-align: middle; box-sizing: border-box; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .close-btn:hover { color: #ff4444; border-color: #ff4444; }
  .retry-btn { margin: 0 20px; padding: 5px 14px; background: none; border: 1px solid #333; color: #00d4ff; font-family: "JetBrains Mono", monospace; font-size: 11px; cursor: pointer; border-radius: 2px; }
  .retry-btn:hover:not(:disabled) { border-color: #00d4ff; }
  .retry-btn:disabled { opacity: 0.6; cursor: default; }
  .body { flex: 1; overflow: auto; padding: 8px 12px; font-family: "JetBrains Mono", monospace; }
  .status-msg { font-family: "JetBrains Mono", monospace; font-size: 12px; color: #00ff88; padding: 20px; }
  .status-msg.error { color: #ff4444; white-space: pre-wrap; }

  .sortbar { display: flex; align-items: center; gap: 6px; padding: 2px 2px 8px; border-bottom: 1px solid #131313; margin-bottom: 4px; }
  .sort-label { font-size: 9px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--muted); margin-right: 2px; }
  .sort-btn { background: none; border: 1px solid #1a1a1a; color: var(--muted); font-family: "JetBrains Mono", monospace; font-size: 10px; padding: 3px 9px; border-radius: 2px; cursor: pointer; }
  .sort-btn:hover { color: #00d4ff; border-color: #333; }
  .sort-btn.active { color: #00ff88; border-color: #00ff88; }

  .group-label { display: flex; align-items: center; gap: 6px; font-size: 10px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--muted); margin: 10px 2px 6px; }
  .group-label:first-child { margin-top: 2px; }
  .group-label.toggle { background: none; border: none; cursor: pointer; padding: 6px 2px; width: 100%; font-family: inherit; }
  .group-label.toggle:hover { color: #00d4ff; }
  .chevron { display: inline-block; font-size: 8px; transition: transform 0.1s; }
  .chevron.open { transform: rotate(90deg); }

  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(250px, 1fr)); gap: 8px; }
  .card { background: #0d0d0d; border: 1px solid #1a1a1a; border-left: 3px solid #555; border-radius: 2px; padding: 9px 11px; display: flex; flex-direction: column; gap: 4px; text-align: left; cursor: pointer; font-family: inherit; }
  .card:hover { border-color: #333; }
  .card.running { border-left-color: #00ff88; }
  .card.running:hover { border-color: #00ff88; }
  .card-head { display: flex; justify-content: space-between; align-items: baseline; gap: 8px; }
  .name { color: #00ff88; font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: flex; align-items: center; }
  .dot { display: inline-block; width: 7px; height: 7px; border-radius: 50%; margin-right: 6px; background: #555; flex-shrink: 0; }
  .dot.up { background: #00ff88; box-shadow: 0 0 5px #00ff88; }
  .state { font-size: 9px; text-transform: uppercase; letter-spacing: 0.05em; color: var(--muted); flex-shrink: 0; }
  .state.up { color: #00ff88; }
  .image { color: var(--accent2); font-size: 10px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .status { color: var(--muted); font-size: 10px; }
  .usage { display: flex; gap: 12px; font-size: 10px; color: var(--accent2); }
  .ports { color: var(--muted); font-size: 9px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  /* Detail view */
  .detail-head { display: flex; justify-content: space-between; align-items: baseline; gap: 8px; padding: 2px 2px 10px; border-bottom: 1px solid #1a1a1a; margin-bottom: 4px; }
  .detail-head .name { font-size: 14px; }
  .section-label { font-size: 10px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--accent2); margin: 14px 2px 6px; }
  .usage-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 6px; }
  .metric { background: #0d0d0d; border: 1px solid #1a1a1a; border-radius: 2px; padding: 7px 9px; display: flex; flex-direction: column; gap: 3px; }
  .m-label { font-size: 9px; text-transform: uppercase; letter-spacing: 0.05em; color: var(--muted); }
  .m-val { font-size: 13px; color: #00ff88; }
  .kv-list { display: flex; flex-direction: column; gap: 3px; }
  .kv { display: grid; grid-template-columns: 120px 1fr; gap: 10px; font-size: 11px; padding: 3px 2px; border-bottom: 1px solid #131313; }
  .kv .k { color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .kv .v { color: #ccc; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .muted { color: var(--muted); }
  .empty { font-size: 11px; color: var(--muted); padding: 2px; }
  .env-list { display: flex; flex-direction: column; gap: 2px; }
  .env-row { font-size: 10px; color: #aaa; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 2px; background: #0d0d0d; border-radius: 2px; }

  .restore-pill { position: fixed; bottom: 12px; right: 12px; z-index: 1000; display: flex; align-items: center; gap: 8px; padding: 6px 10px; background: #0a0a0a; border: 1px solid #00ff88; border-radius: 2px; color: #00ff88; font-family: "JetBrains Mono", monospace; font-size: 11px; cursor: pointer; box-shadow: 0 0 12px rgba(0, 255, 136, 0.3); }
  .restore-pill:hover { background: #111; }
  .pill-icon { font-size: 13px; }
  .pill-text { max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pill-close { color: #888; padding: 0 4px; font-size: 14px; line-height: 1; }
  .pill-close:hover { color: #ff4444; }
</style>
