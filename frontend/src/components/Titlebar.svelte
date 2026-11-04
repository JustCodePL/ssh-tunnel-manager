<script lang="ts">
  import { WindowMinimise, WindowToggleMaximise, WindowClose } from "../lib/runtime";

  function handleDragDblClick() {
    WindowToggleMaximise();
  }
</script>

<div class="titlebar">
  <div class="titlebar-drag" on:dblclick={handleDragDblClick}>
    <span class="titlebar-title">SSH TUNNEL MANAGER</span>
  </div>
  <div class="titlebar-controls">
    <button class="win-btn minimize" on:click={WindowMinimise} title="Minimize">−</button>
    <button class="win-btn maximize" on:click={WindowToggleMaximise} title="Maximize">□</button>
    <button class="win-btn close" on:click={WindowClose} title="Close">×</button>
  </div>
</div>

<style>
  :global([data-theme="terminal"]) .titlebar-title {
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--live);
    text-shadow: 0 0 8px color-mix(in srgb, var(--live) 45%, transparent);
    font-family: var(--mono);
  }
  :global([data-theme="terminal"]) .titlebar-title::before { content: "[ "; }
  :global([data-theme="terminal"]) .titlebar-title::after { content: " ]"; }

  .titlebar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 36px;
    background: var(--surface);
    border-bottom: 1px solid var(--border);
    padding: 0 8px 0 12px;
    flex-shrink: 0;
    user-select: none;
  }

  .titlebar-drag {
    flex: 1;
    display: flex;
    align-items: center;
    height: 100%;
    --wails-draggable: drag;
  }

  .titlebar-title {
    font-family: var(--ui);
    font-size: 12px;
    font-weight: 600;
    color: var(--accent);
    letter-spacing: 0.1em;
    text-shadow: 0 0 8px var(--accent-soft);
  }

  .titlebar-controls {
    display: flex;
    align-items: center;
    gap: 2px;
    --wails-draggable: nodrag;
  }

  .win-btn {
    width: 28px;
    height: 24px;
    background: transparent;
    border: none;
    color: #555555;
    font-size: 16px;
    line-height: 1;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 2px;
    transition: color 0.15s, background 0.15s;
    font-family: var(--ui);
  }

  .win-btn:hover.minimize {
    color: var(--accent2);
    background: var(--accent-soft);
  }

  .win-btn:hover.maximize {
    color: var(--accent);
    background: var(--accent-soft);
  }

  .win-btn:hover.close {
    color: var(--error);
    background: color-mix(in srgb, var(--error) 14%, transparent);
  }
</style>
