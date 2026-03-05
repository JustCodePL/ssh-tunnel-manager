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
  let collapsedFiles: Record<string, boolean> = {};

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

  function toggleFile(file: string) {
    collapsedFiles[file] = !collapsedFiles[file];
    collapsedFiles = collapsedFiles;
  }

  function shortPath(path: string): string {
    return path.replace(/^~\/\.ssh\//, "");
  }

  interface GroupedTunnels {
    ungrouped: TunnelConfig[];
    groups: { name: string; tunnels: TunnelConfig[] }[];
  }

  interface FileSection {
    file: string;
    grouped: GroupedTunnels;
  }

  function groupTunnels(list: TunnelConfig[]): GroupedTunnels {
    const result: GroupedTunnels = { ungrouped: [], groups: [] };
    const groupMap = new Map<string, TunnelConfig[]>();
    for (const t of list) {
      if (t.group) {
        const arr = groupMap.get(t.group);
        if (arr) {
          arr.push(t);
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
  }

  function sectionCount(s: FileSection): number {
    return s.grouped.ungrouped.length + s.grouped.groups.reduce((acc, g) => acc + g.tunnels.length, 0);
  }

  $: filteredTunnels = (() => {
    if (filterGroup === null) return $tunnels;
    if (filterGroup === "__ungrouped__") return $tunnels.filter(t => !t.group);
    return $tunnels.filter(t => t.group === filterGroup);
  })();

  $: fileSections = (() => {
    const fileMap = new Map<string, TunnelConfig[]>();
    for (const t of filteredTunnels) {
      const key = t.sourceFile || "";
      const arr = fileMap.get(key);
      if (arr) {
        arr.push(t);
      } else {
        fileMap.set(key, [t]);
      }
    }
    const sections: FileSection[] = [];
    for (const [file, list] of fileMap) {
      sections.push({ file, grouped: groupTunnels(list) });
    }
    return sections;
  })();

  $: hasMultipleFiles = fileSections.length > 1;
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
        {#each fileSections as section (section.file)}
          {#if hasMultipleFiles}
            <div class="file-section">
              <button
                class="file-header"
                on:click={() => toggleFile(section.file)}
              >
                <span class="file-arrow">{collapsedFiles[section.file] ? '▸' : '▾'}</span>
                <span class="file-name">{shortPath(section.file)}</span>
                <span class="file-count">({sectionCount(section)})</span>
              </button>
              {#if !collapsedFiles[section.file]}
                <div class="file-tunnels">
                  {#each section.grouped.ungrouped as tunnel (tunnel.id)}
                    <TunnelCard
                      {tunnel}
                      status={getStatus($statuses, tunnel.id)}
                      on:edit={handleEdit}
                      on:logs={handleLogs}
                      on:terminal={handleTerminal}
                    />
                  {/each}
                  {#each section.grouped.groups as group (group.name)}
                    <div class="group-section">
                      <button class="group-header" on:click={() => toggleGroup(group.name)}>
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
            </div>
          {:else}
            {#each section.grouped.ungrouped as tunnel (tunnel.id)}
              <TunnelCard
                {tunnel}
                status={getStatus($statuses, tunnel.id)}
                on:edit={handleEdit}
                on:logs={handleLogs}
                on:terminal={handleTerminal}
              />
            {/each}
            {#each section.grouped.groups as group (group.name)}
              <div class="group-section">
                <button class="group-header" on:click={() => toggleGroup(group.name)}>
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
          {/if}
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

  .file-section {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-bottom: 8px;
  }

  .file-header {
    display: flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: none;
    border-bottom: 1px solid var(--border);
    cursor: pointer;
    padding: 6px 0;
    color: var(--accent2);
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    text-align: left;
  }

  .file-arrow {
    font-size: 8px;
    color: var(--accent);
  }

  .file-name {
    letter-spacing: 0.05em;
    color: var(--accent2);
  }

  .file-header:hover .file-name {
    color: var(--accent);
  }

  .file-count {
    color: var(--muted);
    font-size: 9px;
  }

  .file-tunnels {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding-left: 10px;
    border-left: 1px solid var(--border);
    margin-left: 4px;
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
