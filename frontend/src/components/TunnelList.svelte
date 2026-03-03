<script lang="ts">
  import type { TunnelConfig } from "../types";
  import { tunnels, statuses, loading, getStatus } from "../stores/tunnels";
  import TunnelCard from "./TunnelCard.svelte";
  import TunnelForm from "./TunnelForm.svelte";
  import TunnelLogs from "./TunnelLogs.svelte";

  let showForm = false;
  let editingTunnel: TunnelConfig | null = null;
  let loggingTunnel: TunnelConfig | null = null;
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

  $: grouped = (() => {
    const result: GroupedTunnels = { ungrouped: [], groups: [] };
    const groupMap = new Map<string, TunnelConfig[]>();
    for (const t of $tunnels) {
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

<div class="space-y-3">
  {#if showForm}
    <TunnelForm tunnel={editingTunnel} on:close={handleFormClose} />
  {:else}
    <div class="flex items-center justify-between">
      <h2 class="text-sm font-medium text-zinc-500 dark:text-zinc-400">
        {$tunnels.length} tunnel{$tunnels.length !== 1 ? "s" : ""}
      </h2>
      <button
        class="px-3 py-1.5 text-sm rounded bg-blue-600 hover:bg-blue-500 text-white"
        on:click={handleAdd}
      >
        + Add Tunnel
      </button>
    </div>
  {/if}

  {#if $loading}
    <p class="text-sm text-zinc-500 text-center py-8">Loading…</p>
  {:else if !showForm}
    {#if $tunnels.length === 0}
      <div class="text-center py-12">
        <p class="text-zinc-500 text-sm mb-3">No tunnels configured yet.</p>
        <button
          class="px-3 py-1.5 text-sm rounded bg-blue-600 hover:bg-blue-500 text-white"
          on:click={handleAdd}
        >
          Add your first tunnel
        </button>
      </div>
    {:else}
      {#each grouped.ungrouped as tunnel (tunnel.id)}
        <TunnelCard
          {tunnel}
          status={getStatus($statuses, tunnel.id)}
          on:edit={handleEdit}
          on:logs={handleLogs}
        />
      {/each}

      {#each grouped.groups as group (group.name)}
        <div class="space-y-2">
          <button
            class="flex items-center gap-1.5 text-xs font-medium text-zinc-500 dark:text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-300 uppercase tracking-wide"
            on:click={() => toggleGroup(group.name)}
          >
            <span class="text-[10px]">{collapsedGroups[group.name] ? '▸' : '▾'}</span>
            {group.name}
            <span class="text-zinc-500 font-normal normal-case">({group.tunnels.length})</span>
          </button>
          {#if !collapsedGroups[group.name]}
            <div class="space-y-2 ml-3 border-l border-zinc-200 dark:border-zinc-700 pl-3">
              {#each group.tunnels as tunnel (tunnel.id)}
                <TunnelCard
                  {tunnel}
                  status={getStatus($statuses, tunnel.id)}
                  on:edit={handleEdit}
                  on:logs={handleLogs}
                />
              {/each}
            </div>
          {/if}
        </div>
      {/each}
    {/if}
  {/if}
</div>
