<script lang="ts">
  import { onDestroy, tick } from "svelte";
  import Icon from "./Icon.svelte";

  // A text field that also offers existing values, so a new value can be typed.
  export let value = "";
  export let options: string[] = [];
  export let placeholder = "";
  export let testid: string | undefined = undefined;

  let open = false;
  let box: HTMLDivElement;
  let list: HTMLDivElement;
  let position = "";
  let showAll = false;

  $: needle = value.trim().toLowerCase();
  $: shown = showAll || !needle ? options : options.filter((option) => option.toLowerCase().includes(needle));

  async function show(all: boolean) {
    showAll = all;
    open = true;
    await tick();
    const rect = box.getBoundingClientRect();
    const height = list ? Math.min(list.scrollHeight, 220) : 0;
    const below = window.innerHeight - rect.bottom - 8;
    const top = below >= height || rect.top < height ? rect.bottom + 3 : rect.top - height - 3;
    position = `left:${rect.left}px;top:${top}px;width:${rect.width}px;`;
    window.addEventListener("mousedown", onOutside, true);
  }

  function hide() {
    open = false;
    window.removeEventListener("mousedown", onOutside, true);
  }

  function onOutside(event: MouseEvent) {
    if (!box?.contains(event.target as Node) && !list?.contains(event.target as Node)) hide();
  }

  function choose(option: string) {
    value = option;
    hide();
  }

  onDestroy(hide);
</script>

<div class="combo" bind:this={box}>
  <input
    class="combo-input"
    type="text"
    autocomplete="off"
    spellcheck="false"
    data-testid={testid}
    role="combobox"
    aria-expanded={open}
    {placeholder}
    bind:value
    on:input={() => void show(false)}
    on:focus={() => void show(true)}
    on:keydown={(e) => {
      if (e.key === "Escape" || e.key === "Tab" || e.key === "Enter") hide();
    }}
  />
  <button type="button" class="combo-toggle" class:open tabindex="-1" aria-label="Show options" on:click={() => (open ? hide() : void show(true))}>
    <Icon name="chevron" size={12} />
  </button>
  {#if open && shown.length}
    <div class="combo-list" role="listbox" bind:this={list} style={position}>
      {#each shown as option}
        <button type="button" role="option" aria-selected={option === value} class="combo-option" on:click={() => choose(option)}>{option}</button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .combo {
    position: relative;
  }
  .combo-input {
    width: 100%;
    padding: 5px 30px 5px 8px;
    background: var(--surface2);
    border: 1px solid var(--border);
    border-radius: 2px;
    color: var(--text);
    font-family: var(--ui);
    font-size: 12px;
    outline: none;
    transition: border-color 0.15s;
  }
  .combo-input:focus {
    border-color: var(--accent);
  }
  .combo-toggle {
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    width: 28px;
    display: grid;
    place-items: center;
    color: var(--accent);
  }
  .combo-toggle :global(svg) {
    transform: rotate(90deg);
  }
  .combo-toggle.open :global(svg) {
    transform: rotate(-90deg);
  }
  .combo-list {
    position: fixed;
    z-index: 60;
    max-height: 220px;
    overflow-y: auto;
    padding: 4px;
    background: var(--raise);
    border: 1px solid var(--accent);
    border-radius: var(--radius);
    box-shadow: 0 10px 26px rgba(0, 0, 0, 0.45);
  }
  .combo-option {
    display: block;
    width: 100%;
    padding: 5px 8px;
    font-size: 12.5px;
    text-align: left;
    border-radius: var(--radius);
  }
  .combo-option:hover {
    background: var(--surface2);
  }
  .combo-option[aria-selected="true"] {
    color: var(--accent);
  }
</style>
