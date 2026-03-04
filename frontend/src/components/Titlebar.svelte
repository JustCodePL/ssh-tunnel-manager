<script lang="ts">
  import { WindowMinimise, Quit } from "../../wailsjs/runtime/runtime";
</script>

<div class="titlebar">
  <div class="titlebar-drag">
    <span class="titlebar-title">[ SSH TUNNEL MANAGER ]</span>
  </div>
  <div class="titlebar-controls">
    <button class="win-btn minimize" on:click={WindowMinimise} title="Minimize">−</button>
    <button class="win-btn close" on:click={Quit} title="Close">×</button>
  </div>
</div>

<style>
  .titlebar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 36px;
    background: #0d0d0d;
    border-bottom: 1px solid #1e1e1e;
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
    font-family: 'JetBrains Mono', monospace;
    font-size: 11px;
    font-weight: 600;
    color: #00ff88;
    letter-spacing: 0.1em;
    text-shadow: 0 0 8px rgba(0, 255, 136, 0.4);
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
    font-family: 'JetBrains Mono', monospace;
  }

  .win-btn:hover.minimize {
    color: #00d4ff;
    background: rgba(0, 212, 255, 0.1);
  }

  .win-btn:hover.close {
    color: #ff4444;
    background: rgba(255, 68, 68, 0.1);
  }
</style>
