<script lang="ts">
  import { tunnels, statuses, getStatus, connectGroup, disconnectGroup, renameGroup } from "../stores/tunnels";
  import type { TunnelStatus } from "../types";

  export let selectedGroup: string | null = null;
  let renamingGroup: string | null = null;
  let renameValue = "";

  interface GroupInfo {
    name: string;
    count: number;
    connectedCount: number;
  }

  $: groups = (() => {
    const groupMap = new Map<string, { total: number; connected: number }>();
    let ungroupedTotal = 0;
    let ungroupedConnected = 0;
    let pinnedTotal = 0;
    let pinnedConnected = 0;

    for (const t of $tunnels) {
      const status = getStatus($statuses, t.id);
      const isConnected = status === "connected" || status === "connecting" || status === "reconnecting";

      if (t.pinned) {
        pinnedTotal++;
        if (isConnected) pinnedConnected++;
      }

      if (t.group) {
        const existing = groupMap.get(t.group);
        if (existing) {
          existing.total++;
          if (isConnected) existing.connected++;
        } else {
          groupMap.set(t.group, { total: 1, connected: isConnected ? 1 : 0 });
        }
      } else {
        ungroupedTotal++;
        if (isConnected) ungroupedConnected++;
      }
    }

    const result: GroupInfo[] = [];
    for (const [name, info] of groupMap) {
      result.push({ name, count: info.total, connectedCount: info.connected });
    }
    result.sort((a, b) => a.name.localeCompare(b.name));
    return { groups: result, ungroupedTotal, ungroupedConnected, pinnedTotal, pinnedConnected };
  })();

  $: totalConnected = (() => {
    let count = 0;
    for (const t of $tunnels) {
      const status = getStatus($statuses, t.id);
      if (status === "connected" || status === "connecting" || status === "reconnecting") count++;
    }
    return count;
  })();

  function selectGroup(group: string | null) {
    selectedGroup = group;
    renamingGroup = null;
  }

  function startRename(groupName: string) {
    renamingGroup = groupName;
    renameValue = groupName;
  }

  async function confirmRename() {
    if (!renamingGroup || !renameValue.trim()) {
      renamingGroup = null;
      return;
    }
    const oldName = renamingGroup;
    const newName = renameValue.trim();
    renamingGroup = null;
    if (oldName === newName) return;
    await renameGroup(oldName, newName);
    if (selectedGroup === oldName) {
      selectedGroup = newName;
    }
  }

  function handleRenameKeydown(e: KeyboardEvent) {
    if (e.key === "Enter") confirmRename();
    if (e.key === "Escape") renamingGroup = null;
  }

  function isGroupAllConnected(groupName: string): boolean {
    const groupTunnels = $tunnels.filter(t => t.group === groupName);
    return groupTunnels.length > 0 && groupTunnels.every(t => {
      const s = getStatus($statuses, t.id);
      return s === "connected" || s === "connecting" || s === "reconnecting";
    });
  }
</script>

<div class="sidebar">
  <div class="sidebar-header">// groups</div>

  <div class="sidebar-items">
    <button
      class="sidebar-item"
      class:active={selectedGroup === null}
      on:click={() => selectGroup(null)}
    >
      <span class="item-icon">*</span>
      <span class="item-name">all tunnels</span>
      <span class="item-count">{$tunnels.length}</span>
      {#if totalConnected > 0}
        <span class="item-dot active-dot"></span>
      {/if}
    </button>

    {#if groups.pinnedTotal > 0}
      <button
        class="sidebar-item"
        class:active={selectedGroup === "__pinned__"}
        on:click={() => selectGroup("__pinned__")}
      >
        <span class="item-icon">^</span>
        <span class="item-name">pinned</span>
        <span class="item-count">{groups.pinnedTotal}</span>
        {#if groups.pinnedConnected > 0}
          <span class="item-dot active-dot"></span>
        {/if}
      </button>
    {/if}

    {#each groups.groups as group (group.name)}
      <div class="sidebar-group-wrapper">
        {#if renamingGroup === group.name}
          <div class="rename-row">
            <input
              class="rename-input"
              bind:value={renameValue}
              on:keydown={handleRenameKeydown}
              on:blur={confirmRename}
              autofocus
            />
          </div>
        {:else}
          <button
            class="sidebar-item"
            class:active={selectedGroup === group.name}
            on:click={() => selectGroup(group.name)}
          >
            <span class="item-icon">▸</span>
            <span class="item-name">{group.name}</span>
            <span class="item-count">{group.count}</span>
            {#if group.connectedCount > 0}
              <span class="item-dot active-dot"></span>
            {/if}
          </button>

        {/if}
      </div>
    {/each}

    {#if groups.ungroupedTotal > 0}
      <button
        class="sidebar-item"
        class:active={selectedGroup === "__ungrouped__"}
        on:click={() => selectGroup("__ungrouped__")}
      >
        <span class="item-icon">~</span>
        <span class="item-name">unassigned</span>
        <span class="item-count">{groups.ungroupedTotal}</span>
        {#if groups.ungroupedConnected > 0}
          <span class="item-dot active-dot"></span>
        {/if}
      </button>
    {/if}
  </div>
</div>

<style>
  .sidebar {
    width: 180px;
    min-width: 180px;
    background: var(--surface);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    overflow-y: auto;
    flex-shrink: 0;
  }

  .sidebar-header {
    font-size: 9px;
    color: var(--accent);
    font-family: 'JetBrains Mono', monospace;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    padding: 10px 12px 6px;
    border-bottom: 1px solid var(--border);
  }

  .sidebar-items {
    display: flex;
    flex-direction: column;
    padding: 4px 0;
  }

  .sidebar-item {
    display: flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: none;
    cursor: pointer;
    padding: 6px 12px;
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    color: var(--muted);
    text-align: left;
    transition: color 0.15s, background 0.15s;
    position: relative;
  }

  .sidebar-item:hover {
    color: var(--text);
    background: rgba(255, 255, 255, 0.02);
  }

  .sidebar-item.active {
    color: var(--accent);
    background: rgba(0, 255, 136, 0.05);
  }

  .sidebar-item.active::before {
    content: '';
    position: absolute;
    left: 0;
    top: 2px;
    bottom: 2px;
    width: 2px;
    background: var(--accent);
    box-shadow: 0 0 6px var(--accent);
  }

  .item-icon {
    font-size: 9px;
    width: 10px;
    text-align: center;
    flex-shrink: 0;
  }

  .item-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .item-count {
    font-size: 9px;
    color: var(--muted);
    flex-shrink: 0;
  }

  .sidebar-item.active .item-count {
    color: rgba(0, 255, 136, 0.5);
  }

  .item-dot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .active-dot {
    background: var(--accent);
    box-shadow: 0 0 4px var(--accent);
  }

  .sidebar-group-wrapper {
    display: flex;
    flex-direction: column;
  }


  .rename-row {
    padding: 4px 12px;
  }

  .rename-input {
    width: 100%;
    background: var(--surface2);
    border: 1px solid var(--accent);
    border-radius: 2px;
    color: var(--text);
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    padding: 3px 6px;
    outline: none;
  }
</style>
