<script lang="ts">
  import { onMount } from "svelte";
  import { EventsOn, EventsOff } from "../../wailsjs/runtime/runtime";
  import type { PortForwardError } from "../types";

  let err: PortForwardError | null = null;

  onMount(() => {
    EventsOn("port-forward:failed", (event: PortForwardError) => {
      err = event;
    });
    return () => EventsOff("port-forward:failed");
  });
</script>

{#if err}
  <div class="forward-banner" role="alert">
    <div class="forward-body">
      <span class="forward-tag">[ SSH FORWARD ]</span>
      <span class="forward-msg">{err.message}</span>
    </div>
    <div class="forward-actions">
      <button class="forward-btn" on:click={() => (err = null)}>[ DISMISS ]</button>
    </div>
  </div>
{/if}

<style>
  .forward-banner {
    position: fixed;
    bottom: 16px;
    left: 16px;
    right: 16px;
    z-index: 9998;
    display: flex;
    flex-direction: column;
    gap: 8px;
    background: #0a0a0a;
    border: 1px solid #ff4444;
    border-radius: 2px;
    padding: 10px 14px;
    box-shadow: 0 2px 16px rgba(255, 68, 68, 0.25);
    font-family: 'JetBrains Mono', monospace;
    font-size: 11px;
    color: var(--text);
    animation: slide-up 0.18s ease-out;
  }

  .forward-body {
    display: flex;
    align-items: baseline;
    gap: 8px;
  }

  .forward-tag {
    color: #ff4444;
    font-weight: 600;
    white-space: nowrap;
  }

  .forward-msg {
    line-height: 1.4;
  }

  .forward-actions {
    display: flex;
    justify-content: flex-end;
  }

  .forward-btn {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--text);
    font-family: inherit;
    font-size: 11px;
    padding: 4px 10px;
    border-radius: 2px;
    cursor: pointer;
  }

  .forward-btn:hover {
    border-color: var(--text);
  }

  @keyframes slide-up {
    from {
      opacity: 0;
      transform: translateY(8px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
