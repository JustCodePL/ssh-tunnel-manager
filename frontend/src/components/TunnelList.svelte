<script lang="ts">
  import type { TunnelConfig } from "../types";
  import { tunnels, statuses, loading, getStatus } from "../stores/tunnels";
  import TunnelCard from "./TunnelCard.svelte";
  import TunnelForm from "./TunnelForm.svelte";
  import TunnelLogs from "./TunnelLogs.svelte";
  import TerminalWindow from "./TerminalWindow.svelte";

  export let filterGroup: string | null = null;

  let showForm = false;
  let editingTunnel: TunnelConfig | null = null;
  let loggingTunnel: TunnelConfig | null = null;
  let terminalTunnel: TunnelConfig | null = null;
  let collapsedGroups: Record<string, boolean> = {};

  function handleAdd() {
    editingTunnel = null;
    showForm = true;
  }

  function handleEdit(event: CustomEvent<TunnelConfig>) {
    editingTunnel = event.detail;
    showForm = true;
  }

  function handleFormClose() {
    showForm = false;
    editingTunnel = null;
  }

  function handleLogs(event: CustomEvent<TunnelConfig>) {
    loggingTunnel = event.detail;
  }

  function handleTerminal(event: CustomEvent<TunnelConfig>) {
    terminalTunnel = event.detail;
  }

  function handleLogsClose() {
    loggingTunnel = null;
  }

  function toggleGroup(group: string) {
    collapsedGroups[group] = !collapsedGroups[group];
    collapsedGroups = collapsedGroups;
  }

  interface GroupedTunnels {
    ungrouped: TunnelConfig[];
    groups: { name: string; tunnels: TunnelConfig[] }[];
  }

  $: filteredTunnels = (() => {
    if (filterGroup === null) return $tunnels;
    if (filterGroup === "__ungrouped__") return $tunnels.filter(t => !t.group);
    return $tunnels.filter(t => t.group === filterGroup);
  })();

  $: grouped = (() => {
    const result: GroupedTunnels = { ungrouped: [], groups: [] };
    const groupMap = new Map<string, TunnelConfig[]>();
    for (const t of filteredTunnels) {
      if (t.group) {
        const list = groupMap.get(t.group);
        if (list) {
          list.push(t);
        } else {
          groupMap.set(t.group, [t]);
        }
      } else {
        result.ungrouped.push(t);
      }
    }
    for (const [name, tunnels] of groupMap) {
      result.groups.push({ name, tunnels });
    }
    result.groups.sort((a, b) => a.name.localeCompare(b.name));
    return result;
  })();
</script>

{#if loggingTunnel}
  <TunnelLogs
    tunnelName={loggingTunnel.name}
    tunnelId={loggingTunnel.id}
    on:close={handleLogsClose}
  />
{/if}

{#if terminalTunnel}
  <TerminalWindow tunnel={terminalTunnel} on:close={() => terminalTunnel = null} />
{/if}

<div class="tunnel-list">
  {#if showForm}
    <TunnelForm tunnel={editingTunnel} on:close={handleFormClose} />
  {:else}
    <div class="list-header">
      <button class="add-btn" on:click={handleAdd}>+ new tunnel</button>
    </div>
  {/if}

  {#if $loading}
    <div class="loading-msg">// loading...</div>
  {:else if !showForm}
    {#if $tunnels.length === 0}
      <div class="empty-state">
        <div class="empty-art">
          <div>┌─────────────────────┐</div>
          <div>│  no tunnels found   │</div>
          <div>└─────────────────────┘</div>
        </div>
        <button class="add-btn" on:click={handleAdd}>+ add first tunnel</button>
      </div>
    {:else if filteredTunnels.length === 0}
      <div class="empty-state">
        <div class="empty-art">
          <div>// no tunnels in this group</div>
        </div>
      </div>
    {:else}
      <div class="cards">
        {#each grouped.ungrouped as tunnel (tunnel.id)}
          <TunnelCard
            {tunnel}
            status={getStatus($statuses, tunnel.id)}
            on:edit={handleEdit}
            on:logs={handleLogs}
            on:terminal={handleTerminal}
          />
        {/each}

        {#each grouped.groups as group (group.name)}
          <div class="group-section">
            <button
              class="group-header"
              on:click={() => toggleGroup(group.name)}
            >
              <span class="group-arrow">{collapsedGroups[group.name] ? '▸' : '▾'}</span>
              <span class="group-name">{group.name.toUpperCase()}</span>
              <span class="group-count">({group.tunnels.length})</span>
            </button>
            {#if !collapsedGroups[group.name]}
              <div class="group-tunnels">
                {#each group.tunnels as tunnel (tunnel.id)}
                  <TunnelCard
                    {tunnel}
                    status={getStatus($statuses, tunnel.id)}
                    on:edit={handleEdit}
                    on:logs={handleLogs}
                    on:terminal={handleTerminal}
                  />
                {/each}
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>

<style>
  .tunnel-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .list-header {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 4px;
  }

  .add-btn {
    background: transparent;
    border: 1px solid var(--accent);
    color: var(--accent);
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    padding: 4px 10px;
    cursor: pointer;
    border-radius: 2px;
    transition: background 0.15s;
    letter-spacing: 0.05em;
  }

  .add-btn:hover {
    background: rgba(0, 255, 136, 0.1);
  }

  .loading-msg {
    font-size: 11px;
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    padding: 24px 0;
    text-align: center;
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 16px;
    padding: 40px 0;
  }

  .empty-art {
    font-size: 11px;
    color: var(--border);
    font-family: 'JetBrains Mono', monospace;
    line-height: 1.6;
    text-align: center;
  }

  .cards {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .group-section {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .group-header {
    display: flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: none;
    cursor: pointer;
    padding: 4px 0;
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    font-size: 9px;
    text-align: left;
  }

  .group-arrow {
    font-size: 8px;
    color: var(--accent);
  }

  .group-name {
    letter-spacing: 0.1em;
    color: #444;
  }

  .group-header:hover .group-name {
    color: var(--muted);
  }

  .group-count {
    color: #333;
  }

  .group-tunnels {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding-left: 14px;
    border-left: 1px solid var(--border);
    margin-left: 4px;
  }
</style>
