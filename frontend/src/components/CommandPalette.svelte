<script lang="ts">
  import { createEventDispatcher, onMount } from "svelte";
  import { tunnels, statuses, getStatus } from "../stores/tunnels";
  import { capabilities } from "../stores/capabilities";
  import { attachOptionsFor } from "../stores/settings";
  import { buildActions, searchActions } from "../lib/palette.js";

  const dispatch = createEventDispatcher<{ close: void; run: any }>();

  let input: HTMLInputElement;
  let query = "";
  let index = 0;
  let options: Record<string, { protocol: string; label: string }[]> = {};

  onMount(async () => {
    input.focus();
    const entries = await Promise.all(
      $tunnels.map(async (t) => [t.id, await attachOptionsFor(t.id)] as const),
    );
    options = Object.fromEntries(entries);
  });

  $: actions = buildActions({
    tunnels: $tunnels,
    statusOf: (id: string) => getStatus($statuses, id),
    capabilities: $capabilities,
    attachOptions: (id: string) => options[id] ?? [{ protocol: "ssh", label: "Terminal" }],
  });
  $: results = searchActions(actions, query);
  $: if (index >= results.length) index = Math.max(0, results.length - 1);

  function onKey(event: KeyboardEvent) {
    if (event.key === "Escape") {
      dispatch("close");
    } else if (event.key === "ArrowDown") {
      event.preventDefault();
      index = (index + 1) % Math.max(results.length, 1);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      index = (index - 1 + results.length) % Math.max(results.length, 1);
    } else if (event.key === "Enter" && results[index]) {
      dispatch("run", results[index].run);
    }
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
<div class="backdrop" on:click|self={() => dispatch("close")}>
  <div class="palette" role="dialog" aria-label="Actions" data-testid="palette">
    <input
      bind:this={input}
      bind:value={query}
      placeholder="Find a tunnel or run an action"
      aria-label="Find a tunnel or run an action"
      data-testid="palette-input"
      on:keydown={onKey}
      on:input={() => (index = 0)}
    />
    <div class="results" role="listbox">
      {#each results as action, i (action.id)}
        <button
          type="button"
          class="result"
          class:selected={i === index}
          role="option"
          aria-selected={i === index}
          on:mouseenter={() => (index = i)}
          on:click={() => dispatch("run", action.run)}
        >
          <span>{action.label}</span>
          {#if action.hint && action.hint !== "term attach"}<small>{action.hint}</small>{/if}
        </button>
      {:else}
        <p>No matching action.</p>
      {/each}
    </div>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 60;
    display: flex;
    justify-content: center;
    align-items: flex-start;
    padding-top: 12vh;
    background: rgba(0, 0, 0, 0.45);
  }
  .palette {
    width: min(520px, calc(100vw - 32px));
    overflow: hidden;
    background: var(--surface);
    border: 1px solid var(--accent2);
    border-radius: var(--radius-lg);
    box-shadow: 0 18px 40px rgba(0, 0, 0, 0.5);
  }
  input {
    width: 100%;
    padding: 10px 12px;
    font-size: 13px;
    color: var(--text);
    background: transparent;
    border: 0;
    border-bottom: 1px solid var(--border);
    outline: none;
  }
  .results {
    max-height: 320px;
    overflow-y: auto;
    padding: 4px;
  }
  .result {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    width: 100%;
    padding: 6px 8px;
    font-size: 12.5px;
    text-align: left;
    border-radius: var(--radius);
  }
  .result.selected {
    background: var(--raise);
  }
  small {
    color: var(--muted);
    white-space: nowrap;
  }
  p {
    margin: 0;
    padding: 12px;
    color: var(--muted);
  }
</style>
