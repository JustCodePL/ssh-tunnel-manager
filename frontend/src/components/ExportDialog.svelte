<script lang="ts">
  import { createEventDispatcher } from "svelte";
  import { tunnels, remoteDesktops, exportTunnels } from "../stores/tunnels";
  import { showToast } from "../stores/toast";
  import { errorText } from "../lib/runtime";
  import Icon from "./Icon.svelte";

  const dispatch = createEventDispatcher<{ close: void }>();

  // Hosts and remote desktops, all chosen to start with.
  $: entries = [
    ...$tunnels.map((t) => ({ id: t.id, name: t.name, detail: `${t.user ? `${t.user}@` : ""}${t.host}:${t.port}`, icon: "terminal", label: "SSH" })),
    ...$remoteDesktops.map((d) => ({ id: d.id, name: d.name, detail: `${d.address}:${d.port}`, icon: d.protocol === "vnc" ? "vnc" : "rdp", label: d.protocol.toUpperCase() })),
  ];
  let excluded: Record<string, boolean> = {};
  let busy = false;

  $: chosen = entries.filter((e) => !excluded[e.id]);

  async function save() {
    busy = true;
    try {
      if (await exportTunnels(chosen.map((e) => e.id))) {
        showToast(`Exported ${chosen.length} entr${chosen.length === 1 ? "y" : "ies"}`, "success");
        dispatch("close");
      }
    } catch (e) {
      showToast(errorText(e), "error");
    } finally {
      busy = false;
    }
  }
</script>

<div class="backdrop" data-testid="export-dialog">
  <div class="panel" role="dialog" aria-labelledby="export-title">
    <div class="panel-header">
      <span class="panel-title" id="export-title">Export to an SSH config file</span>
      <button type="button" class="icon-btn" aria-label="Cancel" on:click={() => dispatch("close")}>×</button>
    </div>
    <p class="hint">Logins, certificates and settings stay in this app. Remote desktops go into the file's remote desktops section.</p>
    <div class="entries">
      {#each entries as entry (entry.id)}
        <label class="entry" data-testid="export-entry" data-name={entry.name}>
          <input type="checkbox" checked={!excluded[entry.id]} on:change={(e) => (excluded = { ...excluded, [entry.id]: !e.currentTarget.checked })} />
          <span class="icon" title={entry.label}><Icon name={entry.icon} size={13} /></span>
          <span class="name">{entry.name}</span>
          <span class="detail">{entry.detail}</span>
        </label>
      {/each}
    </div>
    <div class="actions">
      <button type="button" class="btn" on:click={() => dispatch("close")}>Cancel</button>
      <button type="button" class="btn primary" data-testid="export-save" disabled={!chosen.length || busy} on:click={save}>Export {chosen.length}</button>
    </div>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 50;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.7);
  }
  .panel {
    display: flex;
    flex-direction: column;
    width: 100%;
    max-width: 520px;
    max-height: calc(100vh - 32px);
    margin: 0 16px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border);
    font-size: 13px;
    font-weight: 600;
  }
  .hint {
    margin: 0;
    padding: 10px 12px 4px;
    color: var(--muted);
    font-size: 12px;
  }
  .entries {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 4px 12px;
  }
  .entry {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 0;
    font-size: 12.5px;
  }
  .icon {
    display: inline-flex;
    color: var(--muted);
  }
  .name {
    font-weight: 600;
  }
  .detail {
    color: var(--muted);
    font-size: 12px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
    padding: 10px 12px;
    border-top: 1px solid var(--border);
  }
</style>
