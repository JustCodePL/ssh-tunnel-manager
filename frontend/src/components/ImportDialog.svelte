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
        error = "No host entries found in the selected file.";
        return;
      }
      // Select all by default
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

<div class="bg-zinc-50 dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 rounded-lg p-5">
  <h2 class="text-base font-semibold mb-4">Import from SSH Config</h2>

  {#if error}
    <div class="mb-3 text-sm text-red-400 bg-red-900/30 border border-red-800 rounded px-3 py-2">
      {error}
    </div>
  {/if}

  {#if step === "pick"}
    <p class="text-sm text-zinc-500 dark:text-zinc-400 mb-4">
      Select an SSH config file to import host entries from.
    </p>
    <div class="flex justify-end gap-2">
      <button
        class="px-3 py-1.5 text-sm rounded bg-zinc-200 dark:bg-zinc-700 hover:bg-zinc-300 dark:hover:bg-zinc-600 text-zinc-700 dark:text-zinc-300"
        on:click={() => dispatch("close")}
      >
        Cancel
      </button>
      <button
        class="px-3 py-1.5 text-sm rounded bg-blue-600 hover:bg-blue-500 text-white"
        on:click={handlePickFile}
      >
        Choose File…
      </button>
    </div>

  {:else if step === "preview"}
    <p class="text-sm text-zinc-500 dark:text-zinc-400 mb-3">
      Found {previewed.length} host{previewed.length !== 1 ? "s" : ""} in
      <span class="font-mono text-zinc-300">{filePath}</span>
    </p>

    <div class="mb-3 max-h-64 overflow-y-auto border border-zinc-200 dark:border-zinc-700 rounded">
      <table class="w-full text-sm">
        <thead class="bg-zinc-900 sticky top-0">
          <tr>
            <th class="px-3 py-2 text-left w-8">
              <input
                type="checkbox"
                checked={allSelected}
                on:change={(e) => toggleAll(e.currentTarget.checked)}
              />
            </th>
            <th class="px-3 py-2 text-left text-xs text-zinc-400 font-medium">Name</th>
            <th class="px-3 py-2 text-left text-xs text-zinc-400 font-medium">Host</th>
            <th class="px-3 py-2 text-left text-xs text-zinc-400 font-medium">User</th>
            <th class="px-3 py-2 text-left text-xs text-zinc-400 font-medium">Forwards</th>
          </tr>
        </thead>
        <tbody>
          {#each previewed as t}
            <tr class="border-t border-zinc-700 hover:bg-zinc-750">
              <td class="px-3 py-2">
                <input type="checkbox" bind:checked={selected[t.name]} />
              </td>
              <td class="px-3 py-2 text-zinc-100 font-mono">{t.name}</td>
              <td class="px-3 py-2 text-zinc-400">{t.host}:{t.port}</td>
              <td class="px-3 py-2 text-zinc-400">{t.user || "—"}</td>
              <td class="px-3 py-2 text-zinc-400">{t.portForwards?.length || 0}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <div class="flex justify-end gap-2">
      <button
        class="px-3 py-1.5 text-sm rounded bg-zinc-200 dark:bg-zinc-700 hover:bg-zinc-300 dark:hover:bg-zinc-600 text-zinc-700 dark:text-zinc-300"
        on:click={() => dispatch("close")}
      >
        Cancel
      </button>
      <button
        class="px-3 py-1.5 text-sm rounded bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50"
        disabled={selectedCount === 0 || importing}
        on:click={handleImport}
      >
        {importing ? "Importing…" : `Import ${selectedCount} tunnel${selectedCount !== 1 ? "s" : ""}`}
      </button>
    </div>

  {:else if step === "done"}
    <p class="text-sm text-zinc-300 mb-4">
      Successfully imported {importedCount} tunnel{importedCount !== 1 ? "s" : ""}.
    </p>
    <div class="flex justify-end">
      <button
        class="px-3 py-1.5 text-sm rounded bg-blue-600 hover:bg-blue-500 text-white"
        on:click={() => dispatch("close")}
      >
        Done
      </button>
    </div>
  {/if}
</div>
