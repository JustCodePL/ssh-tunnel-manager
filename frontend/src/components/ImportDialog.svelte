<script lang="ts">
  import type { TunnelConfig } from "../types";
  import { selectImportFile, previewImport, importTunnels } from "../stores/tunnels";
  import { createEventDispatcher } from "svelte";

  const dispatch = createEventDispatcher<{ close: void }>();

  let step: "pick" | "preview" | "done" = "pick";
  let filePath = "";
  let previewed: TunnelConfig[] = [];
  let selected: Record<string, boolean> = {};
  let importing = false;
  let error = "";
  let importedCount = 0;

  async function handlePickFile() {
    error = "";
    try {
      const path = await selectImportFile();
      if (!path) return;
      filePath = path;
      previewed = await previewImport(path);
      if (previewed.length === 0) {
        error = "no host entries found in the selected file";
        return;
      }
      selected = {};
      for (const t of previewed) {
        selected[t.name] = true;
      }
      step = "preview";
    } catch (e: any) {
      error = e?.message ?? String(e);
    }
  }

  function toggleAll(checked: boolean) {
    for (const t of previewed) {
      selected[t.name] = checked;
    }
    selected = selected;
  }

  $: selectedCount = Object.values(selected).filter(Boolean).length;
  $: allSelected = selectedCount === previewed.length;

  async function handleImport() {
    const names = Object.entries(selected)
      .filter(([, v]) => v)
      .map(([k]) => k);
    if (names.length === 0) return;

    importing = true;
    error = "";
    try {
      importedCount = await importTunnels(filePath, names);
      step = "done";
    } catch (e: any) {
      error = e?.message ?? String(e);
    } finally {
      importing = false;
    }
  }
</script>

<div class="import-container">
  <div class="import-header">
    <span class="import-title">// import from ssh config</span>
    <button class="close-btn" on:click={() => dispatch("close")}>[ × ]</button>
  </div>

  {#if error}
    <div class="error-bar">! {error}</div>
  {/if}

  <div class="import-body">
    {#if step === "pick"}
      <p class="hint-text">select an ssh config file to import host entries from.</p>
      <div class="import-actions">
        <button class="action-btn accent" on:click={handlePickFile}>[ choose file ]</button>
        <button class="action-btn" on:click={() => dispatch("close")}>[ cancel ]</button>
      </div>

    {:else if step === "preview"}
      <div class="preview-info">
        <span class="preview-count">{previewed.length} host{previewed.length !== 1 ? "s" : ""} found</span>
        <span class="preview-path">{filePath}</span>
      </div>

      <div class="table-wrapper">
        <table class="preview-table">
          <thead>
            <tr>
              <th class="th-check">
                <input
                  type="checkbox"
                  checked={allSelected}
                  on:change={(e) => toggleAll(e.currentTarget.checked)}
                />
              </th>
              <th>name</th>
              <th>host</th>
              <th>user</th>
              <th>fwd</th>
            </tr>
          </thead>
          <tbody>
            {#each previewed as t}
              <tr>
                <td class="td-check">
                  <input type="checkbox" bind:checked={selected[t.name]} />
                </td>
                <td class="td-name">{t.name}</td>
                <td class="td-muted">{t.host}:{t.port}</td>
                <td class="td-muted">{t.user || "—"}</td>
                <td class="td-muted">{t.portForwards?.length || 0}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      <div class="import-actions">
        <button
          class="action-btn accent"
          disabled={selectedCount === 0 || importing}
          on:click={handleImport}
        >
          {importing ? "// importing..." : `[ import ${selectedCount} tunnel${selectedCount !== 1 ? "s" : ""} ]`}
        </button>
        <button class="action-btn" on:click={() => dispatch("close")}>[ cancel ]</button>
      </div>

    {:else if step === "done"}
      <div class="done-msg">
        <span class="accent-text">✓</span>
        imported {importedCount} tunnel{importedCount !== 1 ? "s" : ""} successfully
      </div>
      <div class="import-actions">
        <button class="action-btn accent" on:click={() => dispatch("close")}>[ done ]</button>
      </div>
    {/if}
  </div>
</div>

<style>
  .import-container {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 2px;
  }

  .import-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 14px;
    border-bottom: 1px solid var(--border);
  }

  .import-title {
    font-size: 11px;
    color: var(--accent);
    font-family: 'JetBrains Mono', monospace;
    letter-spacing: 0.05em;
  }

  .close-btn {
    background: transparent;
    border: none;
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    cursor: pointer;
    padding: 2px 6px;
    transition: color 0.15s;
  }

  .close-btn:hover {
    color: #ff4444;
  }

  .error-bar {
    background: rgba(255, 68, 68, 0.1);
    border-bottom: 1px solid rgba(255, 68, 68, 0.3);
    color: #ff4444;
    font-size: 10px;
    padding: 6px 14px;
    font-family: 'JetBrains Mono', monospace;
  }

  .import-body {
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .hint-text {
    font-size: 10px;
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    margin: 0;
  }

  .preview-info {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .preview-count {
    font-size: 11px;
    color: var(--text);
    font-family: 'JetBrains Mono', monospace;
  }

  .preview-path {
    font-size: 9px;
    color: #333;
    font-family: 'JetBrains Mono', monospace;
    word-break: break-all;
  }

  .table-wrapper {
    border: 1px solid var(--border);
    border-radius: 2px;
    overflow: auto;
    max-height: 240px;
  }

  .preview-table {
    width: 100%;
    border-collapse: collapse;
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
  }

  .preview-table thead {
    position: sticky;
    top: 0;
    background: var(--surface2);
  }

  .preview-table th {
    padding: 6px 10px;
    text-align: left;
    color: var(--muted);
    font-weight: 400;
    font-size: 9px;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    border-bottom: 1px solid var(--border);
  }

  .preview-table td {
    padding: 5px 10px;
    border-bottom: 1px solid var(--border);
  }

  .preview-table tr:last-child td {
    border-bottom: none;
  }

  .preview-table tr:hover td {
    background: var(--surface2);
  }

  .th-check, .td-check {
    width: 30px;
  }

  .td-name {
    color: var(--text);
  }

  .td-muted {
    color: var(--muted);
  }

  .import-actions {
    display: flex;
    gap: 6px;
  }

  .action-btn {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    padding: 5px 12px;
    cursor: pointer;
    border-radius: 2px;
    transition: color 0.15s, border-color 0.15s, background 0.15s;
    letter-spacing: 0.05em;
  }

  .action-btn:hover:not(:disabled) {
    color: var(--text);
    border-color: var(--muted);
  }

  .action-btn.accent {
    color: var(--accent);
    border-color: var(--accent);
  }

  .action-btn.accent:hover:not(:disabled) {
    background: rgba(0, 255, 136, 0.1);
  }

  .action-btn:disabled {
    opacity: 0.4;
    cursor: default;
  }

  .done-msg {
    font-size: 11px;
    color: var(--text);
    font-family: 'JetBrains Mono', monospace;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .accent-text {
    color: var(--accent);
    text-shadow: 0 0 6px rgba(0, 255, 136, 0.5);
  }
</style>
