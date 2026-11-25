<script lang="ts">
  import { createEventDispatcher, onMount } from "svelte";
  import { tunnels, statuses, remoteDesktops, reachability, getStatus, checkRemoteDesktops, deleteRemoteDesktop, duplicateRemoteDesktop } from "../stores/tunnels";
  import { capabilities } from "../stores/capabilities";
  import { effective, attachOptionsFor } from "../stores/settings";
  import { clientRows, clientIcon, filterRows, groupRows } from "../lib/clients.js";
  import { openTool } from "../lib/openTool";
  import { askConfirm } from "../stores/confirm";
  import { showToast } from "../stores/toast";
  import { errorText } from "../lib/runtime";
  import type { RemoteDesktop } from "../types";
  import Icon from "./Icon.svelte";

  export let query = "";

  const dispatch = createEventDispatcher<{ add: string; edit: RemoteDesktop }>();

  let attachOptions: Record<string, { protocol: string; label: string }[]> = {};
  let menuFor = "";
  let menuPosition = "";
  const checked = new Set<string>();

  async function loadAttachOptions() {
    try {
      const entries = await Promise.all($tunnels.map(async (t) => [t.id, await attachOptionsFor(t.id)] as const));
      attachOptions = Object.fromEntries(entries);
    } catch (e) {
      console.warn("reading attach options failed:", e);
    }
  }

  // Detection and per-host settings decide what a host offers.
  $: void ($tunnels, $effective, $capabilities, loadAttachOptions());

  // Every desktop is checked once after the tab opens, and a new one when it
  // appears. Nothing polls.
  // An edited address or port is checked again.
  $: {
    const unchecked = $remoteDesktops.filter((d) => !checked.has(`${d.id}|${d.address}|${d.port}`));
    for (const d of unchecked) checked.add(`${d.id}|${d.address}|${d.port}`);
    if (unchecked.length) void checkRemoteDesktops(unchecked.map((d) => d.id));
  }

  function checkAll() {
    void checkRemoteDesktops($remoteDesktops.map((d) => d.id));
  }

  $: rows = filterRows(clientRows($tunnels, $remoteDesktops, attachOptions), query);
  $: sections = groupRows(rows);

  function dotClass(row: any): string {
    if (row.kind === "desktop") return `reach-${$reachability[row.key] ?? "checking"}`;
    return getStatus($statuses, row.tunnel.id);
  }

  function dotTitle(row: any): string {
    if (row.kind === "desktop") {
      const state = $reachability[row.key];
      if (state === "up") return "Answers";
      if (state === "down") return "Does not answer";
      return "Checking";
    }
    return `${row.tunnel.name}: ${getStatus($statuses, row.tunnel.id)}`;
  }

  function attach(row: any) {
    menuFor = "";
    void openTool(row.spec);
  }

  function openMenu(row: any, event: MouseEvent) {
    event.preventDefault();
    event.stopPropagation();
    menuFor = menuFor === row.key ? "" : row.key;
    const width = 170;
    const left = Math.min(event.clientX, window.innerWidth - width - 8);
    const top = Math.min(event.clientY + 4, window.innerHeight - 120);
    menuPosition = `left:${left}px;top:${top}px;`;
  }

  function edit(row: any) {
    menuFor = "";
    if (row.kind === "desktop") dispatch("edit", row.desktop);
    else void openTool({ tool: "tunnel-settings", tunnelId: row.tunnel.id, path: row.kind === "forward" ? "forwards" : "" });
  }

  async function duplicate(row: any) {
    menuFor = "";
    try {
      const copy = await duplicateRemoteDesktop(row.desktop.name);
      showToast(`Added ${copy.name}`, "success");
    } catch (e) {
      showToast(errorText(e), "error");
    }
  }

  async function remove(row: any) {
    menuFor = "";
    const ok = await askConfirm({
      title: `Delete ${row.name}?`,
      message: "The desktop is removed from your SSH config. A saved login it used stays in Saved logins.",
      confirmLabel: "Delete",
      danger: true,
    });
    if (!ok) return;
    try {
      await deleteRemoteDesktop(row.desktop.name);
    } catch (e) {
      showToast(errorText(e), "error");
    }
  }

  onMount(() => {
    const close = () => (menuFor = "");
    window.addEventListener("click", close);
    return () => window.removeEventListener("click", close);
  });
</script>

<div class="bar">
  <button type="button" class="btn small" data-testid="add-desktop" on:click={() => dispatch("add", "")}>
    <Icon name="plus" size={13} />Add desktop
  </button>
  <span class="grow"></span>
  {#if $remoteDesktops.length}
    <button type="button" class="btn small" data-testid="check-connections" title="Try a connection to every desktop reached without SSH" on:click={checkAll}>
      <Icon name="refresh" size={13} />Check connections
    </button>
  {/if}
</div>

<div class="list" data-testid="client-list">
  {#if !sections.length}
    <p class="empty">
      {query.trim() ? "No clients match this search." : "Nothing to attach to yet. Add a desktop reached without SSH, or an RDP, VNC or SSH forward on a host."}
    </p>
  {/if}
  {#each sections as section (section.key)}
    <section data-testid="client-group" data-group={section.label}>
      <div class="band"><strong>{section.label}</strong><span class="count">{section.rows.length}</span></div>
      {#each section.rows as row (row.key)}
        <!-- svelte-ignore a11y-click-events-have-key-events -->
        <div
          class="row"
          class:tinted={row.color}
          style={row.color ? `--tunnel-color: ${row.color}` : undefined}
          role="button"
          tabindex="0"
          data-testid="client"
          data-name={row.name}
          data-kind={row.kind}
          data-state={dotClass(row)}
          title={`Open ${row.detail}`}
          on:click={() => attach(row)}
          on:keydown={(e) => e.key === "Enter" && attach(row)}
          on:contextmenu={(e) => openMenu(row, e)}
        >
          <span class="dot {dotClass(row)}" title={dotTitle(row)}></span>
          <span class="proto" title={row.protocol.toUpperCase()}><Icon name={clientIcon(row.protocol)} /></span>
          <span class="name">{row.name}</span>
          {#if row.pinned}<span class="pin" title="Pinned"><Icon name="pin" size={12} /></span>{/if}
          <span class="route">{row.kind === "desktop" ? "direct" : `via ${row.route}`}</span>
          <span class="detail">{row.detail}</span>
          <button type="button" class="icon-btn" aria-label={`Actions for ${row.name}`} data-testid="client-menu" on:click={(e) => openMenu(row, e)} on:keydown|stopPropagation>
            <Icon name="more" />
          </button>
        </div>
        {#if menuFor === row.key}
          <div class="menu" role="menu" style={menuPosition} data-testid="client-actions">
            <button type="button" class="menu-item" role="menuitem" on:click|stopPropagation={() => edit(row)}><Icon name="pen" />Edit</button>
            {#if row.kind === "desktop"}
              <button type="button" class="menu-item" role="menuitem" on:click|stopPropagation={() => duplicate(row)}><Icon name="plus" />Duplicate</button>
              <button type="button" class="menu-item danger" role="menuitem" on:click|stopPropagation={() => remove(row)}><Icon name="trash" />Delete</button>
            {/if}
          </div>
        {/if}
      {/each}
    </section>
  {/each}
</div>

<style>
  .bar {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 5px 8px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }
  .grow {
    flex: 1;
  }
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
    min-height: 36px;
    padding: 0 8px 0 12px;
    cursor: pointer;
    border-left: 3px solid var(--tunnel-color, transparent);
    border-bottom: 1px solid var(--border-soft);
    font-size: 12.5px;
  }
  .row:hover,
  .row:focus-visible {
    background: var(--surface);
    outline: none;
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
  .dot.connected,
  .dot.reach-up {
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
  .dot.reach-down {
    background: var(--muted);
    opacity: 0.6;
  }
  .dot.reach-checking {
    border-style: dashed;
    animation: pulse 1s ease-in-out infinite;
  }
  @keyframes pulse {
    50% {
      opacity: 0.3;
    }
  }
  .proto {
    display: inline-flex;
    color: var(--muted);
  }
  .name {
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }
  .pin {
    display: inline-flex;
    color: var(--muted);
  }
  .route {
    flex: none;
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
  .menu {
    position: fixed;
    min-width: 170px;
    z-index: 20;
  }
  .menu-item.danger {
    color: var(--error);
  }
</style>
