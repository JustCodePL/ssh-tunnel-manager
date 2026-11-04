<script lang="ts">
  import { createEventDispatcher, onDestroy, tick } from "svelte";
  import Icon from "./Icon.svelte";

  export let value = "";
  export let options: { value: string; label: string }[] = [];
  export let placeholder = "Choose";
  export let testid: string | undefined = undefined;
  export let ariaLabel: string | undefined = undefined;
  export let disabled = false;

  const dispatch = createEventDispatcher<{ change: string }>();

  let open = false;
  let active = 0;
  let trigger: HTMLButtonElement;
  let list: HTMLDivElement;
  let position = "";

  $: selected = options.find((option) => option.value === value);

  async function show() {
    if (disabled) return;
    open = true;
    active = Math.max(0, options.findIndex((option) => option.value === value));
    await tick();
    place();
    list?.focus();
    window.addEventListener("mousedown", onOutside, true);
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", hide);
  }

  // Fixed, so a scrolling page cannot clip the list, and above the control when
  // there is no room below it.
  function place() {
    const box = trigger.getBoundingClientRect();
    const height = Math.min(list.scrollHeight, 260);
    const below = window.innerHeight - box.bottom - 8;
    const top = below >= height || box.top < height ? box.bottom + 3 : box.top - height - 3;
    position = `left:${box.left}px;top:${top}px;min-width:${box.width}px;max-width:${Math.max(box.width, window.innerWidth - box.left - 8)}px;`;
  }

  function hide() {
    open = false;
    window.removeEventListener("mousedown", onOutside, true);
    window.removeEventListener("scroll", onScroll, true);
    window.removeEventListener("resize", hide);
  }

  function onOutside(event: MouseEvent) {
    const target = event.target as Node;
    if (!list?.contains(target) && !trigger?.contains(target)) hide();
  }

  function onScroll(event: Event) {
    if (!list?.contains(event.target as Node)) hide();
  }

  function choose(option: { value: string }) {
    hide();
    trigger?.focus();
    if (option.value === value) return;
    value = option.value;
    dispatch("change", option.value);
  }

  function onTriggerKey(event: KeyboardEvent) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      if (!open) void show();
    }
  }

  function onListKey(event: KeyboardEvent) {
    if (event.key === "ArrowDown") active = Math.min(active + 1, options.length - 1);
    else if (event.key === "ArrowUp") active = Math.max(active - 1, 0);
    else if (event.key === "Home") active = 0;
    else if (event.key === "End") active = options.length - 1;
    else if (event.key === "Enter" || event.key === " ") {
      if (options[active]) choose(options[active]);
    } else if (event.key === "Escape") {
      hide();
      trigger?.focus();
    } else if (event.key === "Tab") {
      hide();
      return;
    } else {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    list?.querySelector(`[data-index="${active}"]`)?.scrollIntoView({ block: "nearest" });
  }

  onDestroy(hide);
</script>

<span class="select">
  <button
    type="button"
    class="field-control select-trigger"
    class:open
    bind:this={trigger}
    {disabled}
    data-testid={testid}
    data-value={value}
    aria-label={ariaLabel}
    aria-haspopup="listbox"
    aria-expanded={open}
    title={selected?.label}
    on:click={() => (open ? hide() : void show())}
    on:keydown={onTriggerKey}
  >
    <span class="select-label" class:placeholder={!selected}>{selected?.label ?? placeholder}</span>
    <span class="select-chevron"><Icon name="chevron" size={12} /></span>
  </button>
  {#if open}
    <!-- svelte-ignore a11y-no-noninteractive-tabindex -->
    <div class="select-list" role="listbox" tabindex="0" bind:this={list} style={position} on:keydown={onListKey}>
      {#each options as option, i}
        <button
          type="button"
          role="option"
          tabindex="-1"
          data-index={i}
          class="select-option"
          class:active={i === active}
          aria-selected={option.value === value}
          on:mouseenter={() => (active = i)}
          on:click={() => choose(option)}
        >{option.label}</button>
      {/each}
    </div>
  {/if}
</span>

<style>
  .select {
    display: inline-flex;
    min-width: 0;
    max-width: 100%;
  }
  .select-trigger {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    width: 100%;
    min-height: 28px;
    text-align: left;
    cursor: pointer;
  }
  .select-trigger:hover:not(:disabled),
  .select-trigger.open {
    border-color: var(--accent);
  }
  .select-trigger:disabled {
    opacity: 0.55;
    cursor: default;
  }
  .select-label {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .select-label.placeholder {
    color: var(--muted);
  }
  .select-chevron {
    display: inline-flex;
    flex: none;
    color: var(--accent);
    transform: rotate(90deg);
  }
  .select-trigger.open .select-chevron {
    transform: rotate(-90deg);
  }
  .select-list {
    position: fixed;
    z-index: 60;
    max-height: 260px;
    overflow-y: auto;
    padding: 4px;
    background: var(--raise);
    border: 1px solid var(--accent);
    border-radius: var(--radius);
    box-shadow: 0 10px 26px rgba(0, 0, 0, 0.45);
    outline: none;
  }
  .select-option {
    display: block;
    width: 100%;
    padding: 5px 8px;
    font-size: 12.5px;
    text-align: left;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    border-radius: var(--radius);
  }
  .select-option.active {
    background: var(--surface2);
  }
  .select-option[aria-selected="true"] {
    color: var(--accent);
  }
</style>
