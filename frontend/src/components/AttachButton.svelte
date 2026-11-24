<script lang="ts">
  import Icon from "./Icon.svelte";
  import { openTool } from "../lib/openTool";
  import { attachExpands, attachKey, toolForProtocol, toolIcon } from "../lib/forwards.js";

  export let tunnelId: string;
  export let options: { protocol: string; label: string }[] = [];
  export let small = false;
  // Where the menu opens from, so it stays inside the window.
  export let align: "left" | "right" = "right";

  let open = false;

  function attach(protocol: string) {
    open = false;
    void openTool({ tool: toolForProtocol(protocol), tunnelId });
  }

  function click(event: MouseEvent) {
    event.stopPropagation();
    if (attachExpands(options)) {
      open = !open;
    } else if (options.length === 1) {
      attach(options[0].protocol);
    }
  }

  function closeOnOutside() {
    open = false;
  }

  $: icon = toolIcon(toolForProtocol(options[0]?.protocol ?? ""));
</script>

<svelte:window on:click={closeOnOutside} />

<span class="attach">
  <button
    type="button"
    class="btn"
    class:small
    aria-haspopup={attachExpands(options) ? "menu" : undefined}
    aria-expanded={attachExpands(options) ? open : undefined}
    data-testid="attach"
    title={options.length === 1 ? `Opens ${options[0].label.toLowerCase()}` : "Choose a session"}
    on:click={click}
  >
    <Icon name={icon} />Attach{#if attachExpands(options)}<Icon name="caret" size={11} />{/if}
  </button>
  {#if open}
    <div class="menu {align}" role="menu">
      {#each options as option}
        <button
          type="button"
          class="menu-item"
          role="menuitem"
          on:click|stopPropagation={() => attach(option.protocol)}
        >
          <Icon name={toolIcon(toolForProtocol(option.protocol))} />{option.label}
          <kbd>{attachKey(option.protocol)}</kbd>
        </button>
      {/each}
    </div>
  {/if}
</span>

<style>
  .attach {
    position: relative;
    display: inline-block;
  }
  .menu {
    top: calc(100% + 4px);
  }
  .menu.right {
    right: 0;
  }
  .menu.left {
    left: 0;
  }
</style>
