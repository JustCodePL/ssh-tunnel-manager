<script lang="ts">
  import { onMount } from "svelte";
  import { createEventScope } from "../lib/runtime";
  import type { PortForwardError } from "../types";
  const events = createEventScope();

  let err: PortForwardError | null = null;

  onMount(() => {
    events.on("port-forward:failed", (event: PortForwardError) => {
      err = event;
    });
    return () => events.off();
  });
</script>

{#if err}
  <div class="forward-banner" role="alert">
    <div class="forward-body">
      <span class="forward-tag">SSH FORWARD</span>
      <span class="forward-msg">{err.message}</span>
    </div>
    <div class="forward-actions">
      <button class="forward-btn" on:click={() => (err = null)}>DISMISS</button>
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
    background: var(--bg);
    border: 1px solid var(--error);
    border-radius: 2px;
    padding: 10px 14px;
    box-shadow: 0 2px 16px color-mix(in srgb, var(--error) 14%, transparent);
    font-family: var(--ui);
    font-size: 12px;
    color: var(--text);
    animation: slide-up 0.18s ease-out;
  }

  .forward-body {
    display: flex;
    align-items: baseline;
    gap: 8px;
  }

  .forward-tag {
    color: var(--error);
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
    font-size: 12px;
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
