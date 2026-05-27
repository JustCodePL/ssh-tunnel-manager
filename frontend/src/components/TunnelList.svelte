<script lang="ts">
  import { createEventDispatcher } from "svelte";
  import type { TunnelConfig } from "../types";
  import { tunnels, statuses, loading, getStatus, connectGroup, disconnectGroup, disconnectTunnel, renameGroup } from "../stores/tunnels";
  import TunnelCard from "./TunnelCard.svelte";
  import TunnelForm from "./TunnelForm.svelte";
  import TunnelLogs from "./TunnelLogs.svelte";
  import TerminalWindow from "./TerminalWindow.svelte";
  import FileBrowserWindow from "./FileBrowserWindow.svelte";

  export let filterGroup: string | null = null;

  const dispatch = createEventDispatcher<{ groupRenamed: string }>();

  let showForm = false;
  let editingTunnel: TunnelConfig | null = null;
  let loggingTunnel: TunnelConfig | null = null;
  let terminalTunnel: TunnelConfig | null = null;
  let filesTunnel: TunnelConfig | null = null;
  let collapsedGroups: Record<string, boolean> = {};
  let collapsedFiles: Record<string, boolean> = {};
  let renamingGroup = false;
  let renameValue = "";

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

  function handleFiles(event: CustomEvent<TunnelConfig>) {
    filesTunnel = event.detail;
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
    if (filterGroup === "__pinned__") return $tunnels.filter(t => t.pinned);
    if (filterGroup === "__ungrouped__") return $tunnels.filter(t => !t.group);
    return $tunnels.filter(t => t.group === filterGroup);
  })();

  $: pinnedTunnels = filterGroup === null ? $tunnels.filter(t => t.pinned) : [];
  $: unpinnedTunnels = filterGroup === null
    ? filteredTunnels.filter(t => !t.pinned)
    : filteredTunnels;

  $: fileSections = (() => {
    const fileMap = new Map<string, TunnelConfig[]>();
    for (const t of unpinnedTunnels) {
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

  $: hasMultipleFiles = (() => {
    const files = new Set(filteredTunnels.map(t => t.sourceFile || ""));
    return files.size > 1;
  })();
  $: isAllView = filterGroup === null;
  $: showInlineGroupTitle = filterGroup !== null && filterGroup !== "__pinned__" && filterGroup !== "__ungrouped__";
  $: singleGroupAllConnected = showInlineGroupTitle ? groupAllConnected(filteredTunnels) : false;
  $: anyActive = filteredTunnels.some(t => {
    const s = getStatus($statuses, t.id);
    return s === "connected" || s === "connecting" || s === "reconnecting";
  });
  $: currentTitle = (() => {
    if (filterGroup === null) return "all tunnels";
    if (filterGroup === "__pinned__") return "pinned";
    if (filterGroup === "__ungrouped__") return "unassigned";
    return filterGroup || "all tunnels";
  })();

  function groupAllConnected(tunnelList: TunnelConfig[]): boolean {
    return tunnelList.length > 0 && tunnelList.every(t => {
      const s = getStatus($statuses, t.id);
      return s === "connected" || s === "connecting" || s === "reconnecting";
    });
  }

  function handleGroupConnect(groupName: string) {
    connectGroup(groupName);
  }

  function handleGroupDisconnect(groupName: string) {
    disconnectGroup(groupName);
  }

  function handleDisconnectFiltered() {
    for (const t of filteredTunnels) {
      const s = getStatus($statuses, t.id);
      if (s === "connected" || s === "connecting" || s === "reconnecting") {
        disconnectTunnel(t.id);
      }
    }
  }

  function startRenameGroup() {
    if (filterGroup && filterGroup !== "__pinned__" && filterGroup !== "__ungrouped__") {
      renameValue = filterGroup;
      renamingGroup = true;
    }
  }

  async function confirmRenameGroup() {
    if (!renamingGroup || !renameValue.trim() || !filterGroup) {
      renamingGroup = false;
      return;
    }
    const newName = renameValue.trim();
    const oldName = filterGroup;
    renamingGroup = false;
    if (oldName === newName) return;
    await renameGroup(oldName, newName);
    dispatch("groupRenamed", newName);
  }

  function handleRenameKeydown(e: KeyboardEvent) {
    if (e.key === "Enter") confirmRenameGroup();
    if (e.key === "Escape") renamingGroup = false;
  }
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

{#if filesTunnel}
  <FileBrowserWindow tunnel={filesTunnel} on:close={() => filesTunnel = null} />
{/if}

<div class="tunnel-list">
  {#if showForm}
    <TunnelForm tunnel={editingTunnel} on:close={handleFormClose} />
  {:else}
    <div class="list-header">
      {#if renamingGroup}
        <input
          class="rename-input"
          bind:value={renameValue}
          on:keydown={handleRenameKeydown}
          on:blur={confirmRenameGroup}
          autofocus
        />
      {:else}
        <div class="list-title">{currentTitle}</div>
      {/if}
      <div class="header-actions">
        {#if showInlineGroupTitle}
          {#if singleGroupAllConnected}
            <button class="add-btn disconnect-btn" on:click={() => handleGroupDisconnect(filterGroup)}>disconnect all</button>
          {:else}
            <button class="add-btn" on:click={() => handleGroupConnect(filterGroup)}>connect all</button>
            {#if anyActive}
              <button class="add-btn disconnect-btn" on:click={() => handleGroupDisconnect(filterGroup)}>disconnect all</button>
            {/if}
          {/if}
          <button class="add-btn" on:click={startRenameGroup} title="Rename group">✎</button>
        {:else if anyActive}
          <button class="add-btn disconnect-btn" on:click={handleDisconnectFiltered}>disconnect all</button>
        {/if}
        <button class="add-btn" on:click={handleAdd}>+ new tunnel</button>
      </div>
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
        {#if pinnedTunnels.length > 0}
          <div class="pinned-section">
            <div class="pinned-header">// pinned</div>
            <div class="pinned-tunnels">
              {#each pinnedTunnels as tunnel (tunnel.id)}
                <TunnelCard
                  {tunnel}
                  status={getStatus($statuses, tunnel.id)}
                  on:edit={handleEdit}
                  on:logs={handleLogs}
                  on:terminal={handleTerminal}
                  on:files={handleFiles}
                />
              {/each}
            </div>
          </div>
        {/if}
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
                  {#if showInlineGroupTitle}
                    {#each section.grouped.groups as group}
                      {#each group.tunnels as tunnel (tunnel.id)}
                        <TunnelCard
                          {tunnel}
                          status={getStatus($statuses, tunnel.id)}
                          on:edit={handleEdit}
                          on:logs={handleLogs}
                          on:terminal={handleTerminal}
                          on:files={handleFiles}
                        />
                      {/each}
                    {/each}
                    {#each section.grouped.ungrouped as tunnel (tunnel.id)}
                      <TunnelCard
                        {tunnel}
                        status={getStatus($statuses, tunnel.id)}
                        on:edit={handleEdit}
                        on:logs={handleLogs}
                        on:terminal={handleTerminal}
                        on:files={handleFiles}
                      />
                    {/each}
                  {:else}
                  {#if isAllView && section.grouped.groups.length > 0}
                    <div class="section-subheader">// Groups</div>
                  {/if}
                  {#each section.grouped.groups as group (group.name)}
                    <div class="group-section">
                      <div class="group-header-row">
                        <button class="group-header" on:click={() => toggleGroup(group.name)}>
                          <span class="group-arrow">{collapsedGroups[group.name] ? '▸' : '▾'}</span>
                          <span class="group-name">{group.name.toUpperCase()}</span>
                          <span class="group-count">({group.tunnels.length})</span>
                        </button>
                        {#if groupAllConnected(group.tunnels)}
                          <button class="group-action-btn disconnect" on:click|stopPropagation={() => handleGroupDisconnect(group.name)}>disconnect</button>
                        {:else}
                          <button class="group-action-btn" on:click|stopPropagation={() => handleGroupConnect(group.name)}>connect all</button>
                        {/if}
                      </div>
                      {#if !collapsedGroups[group.name]}
                        <div class="group-tunnels">
                          {#each group.tunnels as tunnel (tunnel.id)}
                            <TunnelCard
                              {tunnel}
                              status={getStatus($statuses, tunnel.id)}
                              on:edit={handleEdit}
                              on:logs={handleLogs}
                              on:terminal={handleTerminal}
                              on:files={handleFiles}
                            />
                          {/each}
                        </div>
                      {/if}
                    </div>
                  {/each}
                  {#if isAllView && section.grouped.ungrouped.length > 0}
                    <div class="section-subheader">// Unassigned</div>
                  {/if}
                  {#each section.grouped.ungrouped as tunnel (tunnel.id)}
                    <TunnelCard
                      {tunnel}
                      status={getStatus($statuses, tunnel.id)}
                      on:edit={handleEdit}
                      on:logs={handleLogs}
                      on:terminal={handleTerminal}
                      on:files={handleFiles}
                    />
                  {/each}
                  {/if}
                </div>
              {/if}
            </div>
          {:else}
            {#if showInlineGroupTitle}
              {#each section.grouped.groups as group}
                {#each group.tunnels as tunnel (tunnel.id)}
                  <TunnelCard
                    {tunnel}
                    status={getStatus($statuses, tunnel.id)}
                    on:edit={handleEdit}
                    on:logs={handleLogs}
                    on:terminal={handleTerminal}
                    on:files={handleFiles}
                  />
                {/each}
              {/each}
              {#each section.grouped.ungrouped as tunnel (tunnel.id)}
                <TunnelCard
                  {tunnel}
                  status={getStatus($statuses, tunnel.id)}
                  on:edit={handleEdit}
                  on:logs={handleLogs}
                  on:terminal={handleTerminal}
                  on:files={handleFiles}
                />
              {/each}
            {:else}
            {#if isAllView && section.grouped.groups.length > 0}
              <div class="section-subheader">// Groups</div>
            {/if}
            {#each section.grouped.groups as group (group.name)}
              <div class="group-section">
                <div class="group-header-row">
                  <button class="group-header" on:click={() => toggleGroup(group.name)}>
                    <span class="group-arrow">{collapsedGroups[group.name] ? '▸' : '▾'}</span>
                    <span class="group-name">{group.name.toUpperCase()}</span>
                    <span class="group-count">({group.tunnels.length})</span>
                  </button>
                  {#if groupAllConnected(group.tunnels)}
                    <button class="group-action-btn disconnect" on:click|stopPropagation={() => handleGroupDisconnect(group.name)}>disconnect</button>
                  {:else}
                    <button class="group-action-btn" on:click|stopPropagation={() => handleGroupConnect(group.name)}>connect all</button>
                  {/if}
                </div>
                {#if !collapsedGroups[group.name]}
                  <div class="group-tunnels">
                    {#each group.tunnels as tunnel (tunnel.id)}
                      <TunnelCard
                        {tunnel}
                        status={getStatus($statuses, tunnel.id)}
                        on:edit={handleEdit}
                        on:logs={handleLogs}
                        on:terminal={handleTerminal}
                        on:files={handleFiles}
                      />
                    {/each}
                  </div>
                {/if}
              </div>
            {/each}
            {#if isAllView && section.grouped.ungrouped.length > 0}
              <div class="section-subheader">// Unassigned</div>
            {/if}
            {#each section.grouped.ungrouped as tunnel (tunnel.id)}
              <TunnelCard
                {tunnel}
                status={getStatus($statuses, tunnel.id)}
                on:edit={handleEdit}
                on:logs={handleLogs}
                on:terminal={handleTerminal}
                on:files={handleFiles}
              />
            {/each}
            {/if}
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
    justify-content: space-between;
    align-items: center;
    margin-bottom: 4px;
    gap: 8px;
  }

  .rename-input {
    flex: 1;
    background: var(--surface2);
    border: 1px solid var(--accent);
    border-radius: 2px;
    color: var(--text);
    font-family: 'JetBrains Mono', monospace;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    padding: 2px 6px;
    outline: none;
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .list-title {
    font-family: 'JetBrains Mono', monospace;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--text);
  }

  .section-subheader {
    font-family: 'JetBrains Mono', monospace;
    font-size: 9px;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--muted);
    padding: 6px 0 2px;
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

  .add-btn.disconnect-btn {
    border-color: #ff4444;
    color: #ff4444;
  }

  .add-btn.disconnect-btn:hover {
    background: rgba(255, 68, 68, 0.1);
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

  .group-header-row {
    display: flex;
    align-items: center;
    gap: 6px;
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

  .group-action-btn {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    font-size: 8px;
    padding: 1px 6px;
    cursor: pointer;
    border-radius: 2px;
    transition: color 0.15s, border-color 0.15s;
    letter-spacing: 0.05em;
    white-space: nowrap;
  }

  .group-action-btn:hover {
    color: var(--accent);
    border-color: var(--accent);
  }

  .group-action-btn.disconnect:hover {
    color: #ff4444;
    border-color: #ff4444;
  }

  .group-arrow {
    font-size: 8px;
    color: var(--accent);
  }

  .group-name {
    letter-spacing: 0.1em;
    color: var(--muted);
  }

  .group-header:hover .group-name {
    color: var(--text);
  }

  .group-count {
    color: var(--muted);
  }

  .group-tunnels {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding-left: 14px;
    border-left: 1px solid var(--border);
    margin-left: 4px;
  }

  .pinned-section {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-bottom: 8px;
  }

  .pinned-header {
    font-size: 9px;
    color: var(--accent2);
    font-family: 'JetBrains Mono', monospace;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    padding: 4px 0;
    border-bottom: 1px solid var(--border);
  }

  .pinned-tunnels {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
</style>
