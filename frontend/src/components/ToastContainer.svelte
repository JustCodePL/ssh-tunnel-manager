<script lang="ts">
  import { toasts } from "../stores/toast";
</script>

<div class="toast-stack">
  {#each $toasts as t (t.id)}
    <div class="toast" class:success={t.kind === "success"} class:error={t.kind === "error"}>
      {t.message}
    </div>
  {/each}
</div>

<style>
  .toast-stack {
    position: fixed;
    /* Above the action bar a window keeps along its bottom edge. */
    bottom: 56px;
    left: 16px;
    right: 16px;
    z-index: 9999;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    pointer-events: none;
  }

  .toast {
    background: var(--bg);
    border: 1px solid var(--border);
    color: var(--text);
    font-family: var(--ui);
    font-size: 12px;
    padding: 6px 12px;
    border-radius: 2px;
    box-shadow: 0 2px 12px rgba(0, 0, 0, 0.6);
    max-width: min(420px, 100%);
    overflow-wrap: anywhere;
    text-align: center;
    animation: slide-in 0.18s ease-out;
  }

  .toast.success {
    border-color: var(--accent);
    color: var(--accent);
    box-shadow: 0 2px 12px var(--accent-soft);
  }

  .toast.error {
    border-color: var(--error);
    color: var(--error);
    box-shadow: 0 2px 12px color-mix(in srgb, var(--error) 14%, transparent);
  }

  @keyframes slide-in {
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
