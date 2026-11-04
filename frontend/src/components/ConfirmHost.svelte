<script lang="ts">
  import { confirmRequest } from "../stores/confirm";

  function answer(value: boolean) {
    $confirmRequest?.resolve(value);
    confirmRequest.set(null);
  }

  function onKey(event: KeyboardEvent) {
    if (!$confirmRequest) return;
    if (event.key === "Escape") answer(false);
  }
</script>

<svelte:window on:keydown={onKey} />

{#if $confirmRequest}
  <div class="backdrop" role="presentation">
    <div class="dialog" class:danger={$confirmRequest.danger} role="alertdialog" aria-modal="true" aria-label={$confirmRequest.title} data-testid="confirm">
      <h2>{$confirmRequest.title}</h2>
      <p>{$confirmRequest.message}</p>
      <div class="actions">
        <button type="button" class="btn" data-testid="confirm-no" on:click={() => answer(false)}>Cancel</button>
        <!-- svelte-ignore a11y-autofocus -->
        <button type="button" class="btn primary" class:danger={$confirmRequest.danger} data-testid="confirm-yes" autofocus on:click={() => answer(true)}>{$confirmRequest.confirmLabel}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 80;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.6);
  }
  .dialog {
    width: min(420px, calc(100vw - 32px));
    padding: 14px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }
  .dialog.danger {
    border-color: var(--error);
  }
  h2 {
    margin: 0 0 6px;
    font-size: 14px;
  }
  p {
    margin: 0;
    color: var(--muted);
    word-break: break-word;
    line-height: 1.4;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
    margin-top: 12px;
  }
  .btn.primary.danger {
    background: var(--error);
    border-color: var(--error);
    color: var(--bg);
  }
  :global([data-theme="terminal"]) .btn.primary.danger {
    background: transparent;
    color: var(--error);
  }
</style>
