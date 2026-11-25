<script lang="ts">
  import { get } from "svelte/store";
  import { tunnels, statuses, getStatus, connectTunnel } from "../stores/tunnels";
  import { webRows, filterRows, groupRows } from "../lib/clients.js";
  import { BrowserOpenURL, errorText } from "../lib/runtime";
  import { showToast } from "../stores/toast";
  import Icon from "./Icon.svelte";

  export let query = "";

  const CONNECT_TIMEOUT = 30_000;
  let opening = "";

  $: rows = filterRows(webRows($tunnels), query);
  $: sections = groupRows(rows);

  /**
   * Resolves once the tunnel is connected, or rejects when this attempt fails.
   * The status the tunnel had before, such as an earlier error, is ignored.
   */
  function connected(tunnelId: string): Promise<void> {
    return new Promise((resolve, reject) => {
      // An error counts only after this attempt has left the old state.
      let started = false;
      let stop = () => {};
      const timer = setTimeout(() => {
        stop();
        reject(new Error("The tunnel did not connect in time."));
      }, CONNECT_TIMEOUT);
      stop = statuses.subscribe((map) => {
        const status = getStatus(map, tunnelId);
        if (status === "connecting" || status === "reconnecting") started = true;
        if (status === "connected" || (status === "error" && started)) {
          clearTimeout(timer);
          queueMicrotask(() => stop());
          if (status === "connected") resolve();
          else reject(new Error(map[tunnelId]?.error || "The tunnel could not connect."));
        }
      });
    });
  }

  async function open(row: any) {
    if (opening) return;
    opening = row.key;
    try {
      const status = getStatus(get(statuses), row.tunnelId);
      if (status !== "connected") {
        const ready = connected(row.tunnelId);
        // A failed connect call ends the wait below; this keeps its later
        // timeout from going unhandled.
        ready.catch(() => {});
        // A tunnel already connecting or reconnecting is only waited for.
        if (status === "disconnected" || status === "error") await connectTunnel(row.tunnelId);
        await ready;
      }
      await BrowserOpenURL(row.url);
    } catch (e) {
      showToast(`${row.name}: ${errorText(e)}`, "error");
    } finally {
      opening = "";
    }
  }
</script>

<div class="list" data-testid="web-list">
  {#if !sections.length}
    <p class="empty">
      {query.trim() ? "No pages match this search." : "No web pages yet. A portless forward on a host shows up here, under the name the browser opens."}
    </p>
  {/if}
  {#each sections as section (section.key)}
    <section data-testid="web-group" data-group={section.label}>
      <div class="band"><strong>{section.label}</strong><span class="count">{section.rows.length}</span></div>
      {#each section.rows as row (row.key)}
        <button
          type="button"
          class="row"
          class:tinted={row.color}
          style={row.color ? `--tunnel-color: ${row.color}` : undefined}
          data-testid="web-page"
          data-name={row.name}
          disabled={opening === row.key}
          title={`Open ${row.url}`}
          on:click={() => open(row)}
        >
          <span class="dot {getStatus($statuses, row.tunnelId)}"></span>
          <span class="icon"><Icon name="globe" /></span>
          <span class="name">{row.name}</span>
          <span class="route">via {row.route}</span>
          <span class="detail">{opening === row.key ? "connecting…" : row.url}</span>
        </button>
      {/each}
    </section>
  {/each}
</div>

<style>
  .list {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
  }
  .empty {
    padding: 24px;
    text-align: center;
    color: var(--muted);
  }
  .band {
    position: sticky;
    top: 0;
    z-index: 2;
    display: flex;
    align-items: center;
    gap: 8px;
    height: 30px;
    padding: 0 12px;
    background: var(--surface2);
    border-bottom: 1px solid var(--border);
    font-size: 12.5px;
    color: var(--muted);
  }
  .band strong {
    color: var(--text);
    font-size: 13px;
    font-weight: 650;
  }
  :global([data-theme="terminal"]) .band strong::before {
    content: "// ";
    color: var(--live);
  }
  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    min-height: 36px;
    padding: 0 12px;
    text-align: left;
    border-left: 3px solid var(--tunnel-color, transparent);
    border-bottom: 1px solid var(--border-soft);
    font-size: 12.5px;
  }
  .row:hover {
    background: var(--surface);
  }
  .row.tinted {
    background: color-mix(in srgb, var(--tunnel-color) 10%, transparent);
  }
  .dot {
    width: 8px;
    height: 8px;
    flex: none;
    border-radius: 50%;
    border: 1.5px solid var(--muted);
  }
  .dot.connected {
    background: var(--live);
    border-color: var(--live);
    box-shadow: var(--glow);
  }
  .dot.connecting,
  .dot.reconnecting {
    background: var(--warning);
    border-color: var(--warning);
  }
  .dot.error {
    background: var(--error);
    border-color: var(--error);
  }
  .icon {
    display: inline-flex;
    color: var(--muted);
  }
  .name {
    font-weight: 600;
    white-space: nowrap;
  }
  .route {
    color: var(--accent);
    font-size: 12px;
    white-space: nowrap;
  }
  .detail {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--muted);
    font-size: 12px;
  }
</style>
