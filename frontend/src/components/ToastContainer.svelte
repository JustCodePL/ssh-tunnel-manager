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
    bottom: 16px;
    right: 16px;
    z-index: 9999;
    display: flex;
    flex-direction: column;
    gap: 6px;
    pointer-events: none;
  }

  .toast {
    background: #0a0a0a;
    border: 1px solid var(--border);
    color: var(--text);
    font-family: 'JetBrains Mono', monospace;
    font-size: 11px;
    padding: 6px 12px;
    border-radius: 2px;
    box-shadow: 0 2px 12px rgba(0, 0, 0, 0.6);
    animation: slide-in 0.18s ease-out;
  }

  .toast.success {
    border-color: var(--accent);
    color: var(--accent);
    box-shadow: 0 2px 12px rgba(0, 255, 136, 0.25);
  }

  .toast.error {
    border-color: #ff4444;
    color: #ff4444;
    box-shadow: 0 2px 12px rgba(255, 68, 68, 0.25);
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
