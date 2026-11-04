<script lang="ts">
  import { createEventDispatcher } from "svelte";

  export let row: Record<string, string> = {};
  // Off is not offered where a module always has a place, like the forward count.
  const MODULES = [
    { key: "attach", label: "Attach" },
    { key: "forwards", label: "Forward count" },
    { key: "stats", label: "CPU and RAM" },
    { key: "address", label: "Address" },
    { key: "files", label: "Files" },
    { key: "logs", label: "Logs" },
    { key: "disk", label: "Disk" },
    { key: "docker", label: "Docker" },
    { key: "htop", label: "htop" },
  ];
  const PLACES = [
    { value: "row", label: "Row" },
    { value: "panel", label: "Panel" },
    { value: "off", label: "Off" },
  ];

  const dispatch = createEventDispatcher<{ change: Record<string, string> }>();

  function set(key: string, value: string) {
    dispatch("change", { ...row, [key]: value });
  }
</script>

<p class="hint">Row: always visible. Panel: after expanding. Off: hidden.</p>
<div class="grid" data-testid="row-layout">
  {#each MODULES as module}
    <div class="item">
      <span>{module.label}</span>
      <span class="segmented">
        {#each PLACES as place}
          <button
            type="button"
            class:on={(row[module.key] ?? "panel") === place.value}
            data-testid={`row-${module.key}-${place.value}`}
            on:click={() => set(module.key, place.value)}
          >{place.label}</button>
        {/each}
      </span>
    </div>
  {/each}
</div>

<style>
  .hint {
    margin: 0 0 8px;
    color: var(--muted);
    font-size: 12px;
  }
  .grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 3px 20px;
  }
  .item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    min-height: 28px;
  }
  @media (max-width: 720px) {
    .grid {
      grid-template-columns: 1fr;
    }
  }
</style>
