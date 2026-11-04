<script lang="ts">
  import { get } from "svelte/store";
  import { createEventDispatcher, onDestroy, onMount, tick } from "svelte";
  import { tunnels, statuses, loading, groups, getStatus, connectGroup, disconnectGroup, renameGroup, includedConfigFiles, connectTunnel, disconnectTunnel, disconnectAll, addGroup, removeGroup } from "../stores/tunnels";
  import { capabilities, refreshCapabilities } from "../stores/capabilities";
  import { effective, settingsFor, attachOptionsFor } from "../stores/settings";
  import { filterTunnels, groupTunnels, isOn, toolsIn, ROW_TOOLS } from "../lib/rowLayout.js";
  import { openTool } from "../lib/openTool";
  import TunnelRow from "./TunnelRow.svelte";
  import Icon from "./Icon.svelte";

  export let filter = "all";
  export let query = "";
  export let paused = false;
  export let creatingGroup = false;

  const dispatch = createEventDispatcher<{ new: string }>();

  const STORAGE_KEY = "stm-collapsed-groups";
  let collapsed: Record<string, boolean> = {};
  let expanded: Record<string, boolean> = {};
  let selectedId = "";
  let renaming = "";
  let renameValue = "";
  let attachOptions: Record<string, { protocol: string; label: string }[]> = {};

  onMount(() => {
    try {
      collapsed = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "{}");
    } catch {
      collapsed = {};
    }
    if (typeof ResizeObserver !== "undefined") {
      chipObserver = new ResizeObserver(measureChips);
      chipObserver.observe(filtersEl);
    }
    measureChips();
  });

  function toggleGroup(key: string) {
    collapsed = { ...collapsed, [key]: !collapsed[key] };
    localStorage.setItem(STORAGE_KEY, JSON.stringify(collapsed));
  }

  async function loadAttachOptions(id: string) {
    attachOptions = { ...attachOptions, [id]: await attachOptionsFor(id) };
  }

  // Detected tools decide what Attach offers. Connecting re-probes the host, and
  // the options follow once the capabilities store has the result.
  const connectedBefore = new Set<string>();
  // A tunnel's settings can turn remote desktop on or off.
  let optionsFor: unknown = null;
  $: if ($effective !== optionsFor) {
    optionsFor = $effective;
    for (const t of $tunnels) void loadAttachOptions(t.id);
  }
  $: for (const t of $tunnels) {
    const connected = getStatus($statuses, t.id) === "connected";
    if (connected && !connectedBefore.has(t.id)) {
      connectedBefore.add(t.id);
      void refreshCapabilities(t.id).then(() => loadAttachOptions(t.id));
    } else if (!connected) {
      connectedBefore.delete(t.id);
    }
    if (!attachOptions[t.id]) {
      attachOptions[t.id] = [{ protocol: "ssh", label: "Terminal" }];
      void loadAttachOptions(t.id);
    }
  }

  $: visible = filterTunnels($tunnels, filter, query, (id: string) => getStatus($statuses, id));
  $: emptyGroups = query.trim() || (filter !== "all" && !filter.startsWith("group:")) ? [] : $groups.filter((name) => filter === "all" || filter === `group:${name}`);
  $: sections = groupTunnels(visible, emptyGroups);
  $: flatVisible = sections.flatMap((s) => (collapsed[s.key] ? [] : s.tunnels));
  $: liveCount = $tunnels.filter((t) => isOn(getStatus($statuses, t.id))).length;
  $: pinnedCount = $tunnels.filter((t) => t.pinned).length;
  $: unassignedCount = $tunnels.filter((t) => !t.group).length;
  $: groupNames = [...new Set([...$tunnels.map((t) => t.group), ...$groups].filter(Boolean))].sort((a, b) => a.localeCompare(b));
  $: sourceFiles = $includedConfigFiles ?? [];
  $: chips = [
    { id: "all", label: "All", count: $tunnels.length, title: "" },
    { id: "live", label: "Live", count: liveCount, title: "" },
    ...(pinnedCount ? [{ id: "pinned", label: "Pinned", count: pinnedCount, title: "" }] : []),
    ...groupNames.map((name) => ({ id: `group:${name}`, label: name, count: $tunnels.filter((t) => t.group === name).length, title: "" })),
    ...(unassignedCount ? [{ id: "unassigned", label: "Unassigned", count: unassignedCount, title: "" }] : []),
    ...(sourceFiles.length > 1 ? sourceFiles.map((file) => ({ id: `file:${file.path}`, label: file.label, count: $tunnels.filter((t) => t.sourceFile === file.path).length, title: file.path })) : []),
  ];
  $: shownChips = chips.slice(0, fitCount);
  $: hiddenChips = chips.slice(fitCount);
  $: if (chips && filtersEl) void tick().then(measureChips);

  // The chips stay on one line. Those that do not fit go into a dropdown.
  let filtersEl: HTMLDivElement;
  let measureEl: HTMLDivElement;
  let fitCount = Infinity;
  let showHidden = false;
  let chipObserver: ResizeObserver | null = null;

  function measureChips() {
    if (!filtersEl || !measureEl) return;
    const gap = 2;
    const widths = [...measureEl.children].map((child) => (child as HTMLElement).offsetWidth + gap);
    const action = filtersEl.querySelector(".disconnect-all") as HTMLElement | null;
    const available = filtersEl.clientWidth - 16 - (action ? action.offsetWidth + 8 : 0);
    const total = widths.reduce((sum, width) => sum + width, 0);
    if (total <= available) {
      fitCount = widths.length;
      return;
    }
    let used = 34; // the dropdown's button
    let count = 0;
    for (const width of widths) {
      if (used + width > available) break;
      used += width;
      count += 1;
    }
    fitCount = Math.max(count, 1);
  }

  onDestroy(() => chipObserver?.disconnect());

  let newGroupName = "";

  async function finishNewGroup() {
    const name = newGroupName.trim();
    creatingGroup = false;
    newGroupName = "";
    if (!name) return;
    filter = "all";
    await addGroup(name);
  }

  function liveIn(list: { id: string }[]): number {
    return list.filter((t) => isOn(getStatus($statuses, t.id))).length;
  }

  function startRename(group: string) {
    renaming = group;
    renameValue = group;
  }

  async function finishRename() {
    const from = renaming;
    const to = renameValue.trim();
    renaming = "";
    if (from && to && from !== to) {
      await renameGroup(from, to);
      if (filter === `group:${from}`) filter = `group:${to}`;
    }
  }

  function moveSelection(step: number) {
    if (!flatVisible.length) return;
    const index = flatVisible.findIndex((t) => t.id === selectedId);
    const next = index < 0 ? (step > 0 ? 0 : flatVisible.length - 1) : (index + step + flatVisible.length) % flatVisible.length;
    selectedId = flatVisible[next].id;
    document.querySelector(`[data-name="${CSS.escape(flatVisible[next].name)}"]`)?.scrollIntoView({ block: "nearest" });
  }

  function onKey(event: KeyboardEvent) {
    if (paused || event.metaKey || event.ctrlKey || event.altKey) return;
    const target = event.target as HTMLElement;
    if (["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)) {
      if (event.key === "Escape") target.blur();
      return;
    }
    if (event.key === "ArrowDown" || event.key === "j") {
      event.preventDefault();
      moveSelection(1);
      return;
    }
    if (event.key === "ArrowUp" || event.key === "k") {
      event.preventDefault();
      moveSelection(-1);
      return;
    }
    if (event.key === "/") {
      event.preventDefault();
      (document.querySelector('[data-testid="search"]') as HTMLElement | null)?.focus();
      return;
    }
    const tunnel = flatVisible.find((t) => t.id === selectedId);
    if (!tunnel) return;
    const key = event.key.toLowerCase();
    if (event.key === "Enter") {
      expanded = { ...expanded, [tunnel.id]: !expanded[tunnel.id] };
    } else if (event.key === " ") {
      event.preventDefault();
      void (isOn(getStatus($statuses, tunnel.id)) ? disconnectTunnel(tunnel.id) : connectTunnel(tunnel.id));
    } else if (key === "t") {
      void openTool({ tool: "terminal", tunnelId: tunnel.id });
    } else if (key === "r" && attachOptions[tunnel.id]?.some((o) => o.protocol === "rdp")) {
      void openTool({ tool: "rdp", tunnelId: tunnel.id });
    } else if (key === "s") {
      void openTool({ tool: "tunnel-settings", tunnelId: tunnel.id });
    } else {
      const tool = ROW_TOOLS.find((item) => item.shortcut === key);
      const row = settingsFor(get(effective), tunnel.id).row;
      if (tool && toolsIn(row, "panel", $capabilities[tunnel.id]).some((item) => item.key === tool.key)) {
        void openTool({ tool: tool.tool, tunnelId: tunnel.id });
      }
    }
  }
</script>

<svelte:window on:keydown={onKey} />

<svelte:body on:click={() => (showHidden = false)} />

<div class="filters" role="tablist" aria-label="Filter tunnels" bind:this={filtersEl}>
  <div class="chip-measure" bind:this={measureEl} aria-hidden="true">
    {#each chips as chip}<span class="chip">{chip.label}<b>{chip.count}</b></span>{/each}
  </div>
  {#each shownChips as chip (chip.id)}
    <button
      type="button"
      role="tab"
      class="chip"
      aria-selected={filter === chip.id}
      class:on={filter === chip.id}
      title={chip.title || undefined}
      data-testid={chip.id === "live" ? "filter-live" : undefined}
      on:click={() => (filter = chip.id)}
    >{chip.label}<b>{chip.count}</b></button>
  {/each}
  {#if hiddenChips.length}
    <span class="chip-more">
      <button
        type="button"
        class="chip chip-toggle"
        class:on={hiddenChips.some((chip) => chip.id === filter)}
        aria-label="More groups"
        aria-haspopup="menu"
        aria-expanded={showHidden}
        data-testid="more-groups"
        on:click|stopPropagation={() => (showHidden = !showHidden)}
      ><span class="toggle-chevron"><Icon name="chevron" size={12} /></span></button>
      {#if showHidden}
        <div class="menu" role="menu">
          {#each hiddenChips as chip (chip.id)}
            <button type="button" class="menu-item" role="menuitem" class:current={filter === chip.id} on:click={() => { filter = chip.id; showHidden = false; }}>
              {chip.label}<kbd>{chip.count}</kbd>
            </button>
          {/each}
        </div>
      {/if}
    </span>
  {/if}
  {#if liveCount > 0}
    <button type="button" class="disconnect-all" data-testid="disconnect-all" on:click={disconnectAll}>Disconnect all</button>
  {/if}
</div>

<div class="list" data-testid="tunnel-list">
  {#if creatingGroup}
    <div class="band new-group-band">
      <!-- svelte-ignore a11y-autofocus -->
      <input
        class="field-control"
        autofocus
        placeholder="Group name"
        aria-label="Group name"
        data-testid="new-group-name"
        bind:value={newGroupName}
        on:blur={finishNewGroup}
        on:keydown={(e) => {
          if (e.key === "Enter") void finishNewGroup();
          if (e.key === "Escape") { newGroupName = ""; creatingGroup = false; }
        }}
      />
    </div>
  {/if}
  {#if $loading && !$tunnels.length}
    <p class="empty">Loading tunnels</p>
  {:else if !$tunnels.length && !$groups.length}
    <p class="empty">No tunnels yet. Add one with the + button, or import your SSH config from there.</p>
  {:else if !sections.length}
    <p class="empty">No tunnels match this filter.</p>
  {/if}

  {#each sections as section (section.key)}
    {@const live = liveIn(section.tunnels)}
    <section data-testid="group" data-group={section.label}>
      <!-- svelte-ignore a11y-click-events-have-key-events -->
      <div class="band" role="button" tabindex="-1" on:click={() => toggleGroup(section.key)}>
        <span class="chevron" class:down={!collapsed[section.key]}><Icon name="chevron" size={12} /></span>
        {#if renaming === section.group && renaming}
          <!-- svelte-ignore a11y-autofocus -->
          <input
            class="field-control"
            autofocus
            bind:value={renameValue}
            on:click|stopPropagation
            on:blur={finishRename}
            on:keydown={(e) => {
              if (e.key === "Enter") void finishRename();
              if (e.key === "Escape") renaming = "";
            }}
          />
        {:else}
          <!-- svelte-ignore a11y-no-static-element-interactions -->
          <strong title={section.group ? "Double-click to rename" : undefined} on:dblclick|stopPropagation={() => section.group && startRename(section.group)}>{section.label}</strong>
        {/if}
        <span class="count">{section.tunnels.length && (section.group || section.key === "__pinned__") ? `${live} of ${section.tunnels.length} live` : `${section.tunnels.length} tunnel${section.tunnels.length === 1 ? "" : "s"}`}</span>
        {#if section.group && !section.tunnels.length}
          <button type="button" class="band-action" data-testid="delete-group" on:click|stopPropagation={() => removeGroup(section.group)}>Delete group</button>
        {:else if section.group}
          {#if live < section.tunnels.length}
            <button type="button" class="band-action" on:click|stopPropagation={() => connectGroup(section.group)}>Connect all</button>
          {/if}
          {#if live > 0}
            <button type="button" class="band-action" class:first={live === section.tunnels.length} on:click|stopPropagation={() => disconnectGroup(section.group)}>Disconnect all</button>
          {/if}
        {/if}
      </div>
      {#if !collapsed[section.key]}
        <div class="members">
          {#each section.tunnels as tunnel (tunnel.id)}
            <TunnelRow
              {tunnel}
              event={$statuses[tunnel.id]}
              settings={settingsFor($effective, tunnel.id)}
              caps={$capabilities[tunnel.id]}
              attachOptions={attachOptions[tunnel.id] ?? []}
              expanded={!!expanded[tunnel.id]}
              selected={selectedId === tunnel.id}
              on:toggle={() => {
                selectedId = tunnel.id;
                expanded = { ...expanded, [tunnel.id]: !expanded[tunnel.id] };
              }}
            />
          {/each}
          {#if section.key !== "__pinned__"}
            <button type="button" class="add-row" data-testid="add-to-group" on:click={() => dispatch("new", section.group)}>
              <Icon name="plus" size={13} />New tunnel
            </button>
          {/if}
        </div>
      {/if}
    </section>
  {/each}
</div>

<style>
  .filters {
    position: relative;
    display: flex;
    align-items: center;
    gap: 2px;
    padding: 5px 8px;
    font-size: 12.5px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }
  .chip {
    flex: none;
    padding: 3px 9px;
    border-radius: var(--radius);
    color: var(--muted);
    font-size: 12.5px;
    white-space: nowrap;
  }
  .chip.on {
    background: var(--raise);
    color: var(--text);
  }
  .chip b {
    font-weight: 400;
    margin-left: 5px;
    opacity: 0.75;
  }
  .chip-measure {
    position: absolute;
    left: 0;
    top: 0;
    display: flex;
    gap: 2px;
    height: 0;
    overflow: hidden;
    visibility: hidden;
    pointer-events: none;
  }
  .chip-more {
    position: relative;
    flex: none;
  }
  .chip-more .menu {
    top: calc(100% + 4px);
    left: 0;
  }
  .chip-toggle {
    display: inline-flex;
    align-items: center;
    color: var(--accent);
  }
  .toggle-chevron {
    display: inline-flex;
    transform: rotate(90deg);
  }
  .menu-item.current {
    color: var(--accent);
  }
  .disconnect-all {
    flex: none;
    margin-left: auto;
    padding: 3px 6px;
    color: var(--muted);
    font-size: 12px;
    white-space: nowrap;
  }
  .disconnect-all:hover {
    color: var(--text);
  }
  .new-group-band input {
    flex: 1;
  }
  .add-row {
    display: flex;
    align-items: center;
    gap: 7px;
    width: 100%;
    height: 32px;
    padding: 0 14px;
    color: var(--muted);
    font-size: 12.5px;
    text-align: left;
  }
  .add-row:hover {
    color: var(--accent);
    background: var(--surface);
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
    height: 33px;
    padding: 0 12px 0 11px;
    background: var(--surface2);
    border-bottom: 1px solid var(--border);
    font-size: 12.5px;
    color: var(--muted);
    cursor: pointer;
    user-select: none;
  }
  section + section .band {
    border-top: 1px solid var(--border);
  }
  .band strong {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text);
    font-size: 13px;
    font-weight: 650;
  }
  .band .count {
    flex: 0 100 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: clip;
    white-space: nowrap;
  }
  :global([data-theme="terminal"]) .band strong::before {
    content: "// ";
    color: var(--live);
  }
  .chevron {
    display: inline-flex;
  }
  .chevron.down {
    transform: rotate(90deg);
  }
  .band-action {
    flex: none;
    white-space: nowrap;
    font-size: 12px;
    color: var(--muted);
    margin-left: 8px;
  }
  .band-action:first-of-type {
    margin-left: auto;
  }
  .band-action:hover {
    color: var(--text);
  }
  .members {
    margin-left: 16px;
    border-left: 1px solid var(--border);
  }
</style>
