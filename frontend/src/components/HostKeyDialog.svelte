<script lang="ts">
  import { AnswerHostKey } from "../../bindings/ssh-tunnel-manager/app";
  import { createEventDispatcher } from "svelte";

  export let prompt: { id: number; tunnelName: string; address: string; keyType: string; fingerprint: string };

  const dispatch = createEventDispatcher<{ close: void }>();

  async function answer(accepted: boolean) {
    await AnswerHostKey(prompt.id, accepted);
    dispatch("close");
  }
</script>

<div class="backdrop" data-testid="host-key-dialog">
  <div class="panel" role="alertdialog" aria-labelledby="host-key-title" aria-describedby="host-key-text">
    <div class="panel-header">
      <span class="panel-title" id="host-key-title">Unknown server</span>
    </div>
    <div class="panel-body">
      <p id="host-key-text">
        The app has not seen this server before and it is not in <code>~/.ssh/known_hosts</code>. Trust it only if the
        fingerprint matches the one the server's administrator gave you.
      </p>
      <dl>
        <dt>tunnel</dt>
        <dd>{prompt.tunnelName}</dd>
        <dt>server</dt>
        <dd>{prompt.address}</dd>
        <dt>{prompt.keyType}</dt>
        <dd data-testid="host-key-fingerprint">{prompt.fingerprint}</dd>
      </dl>
      <div class="form-actions">
        <button type="button" class="action-btn" data-testid="host-key-reject" on:click={() => answer(false)}>Cancel</button>
        <button type="button" class="action-btn accent" data-testid="host-key-trust" on:click={() => answer(true)}>Trust this server</button>
      </div>
    </div>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.8);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 60;
  }

  .panel {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 2px;
    width: 100%;
    max-width: 460px;
    margin: 0 16px;
    box-shadow: 0 0 40px rgba(0, 0, 0, 0.6);
  }

  .panel-header {
    padding: 10px 14px;
    border-bottom: 1px solid var(--border);
  }

  .panel-title {
    font-size: 12px;
    color: var(--accent);
    font-family: var(--ui);
    letter-spacing: 0.05em;
  }

  .panel-body {
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 12px;
    font-family: var(--ui);
    font-size: 12px;
    color: var(--text);
  }

  p {
    margin: 0;
    line-height: 1.5;
  }

  dl {
    margin: 0;
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 4px 10px;
  }

  dt {
    color: var(--muted);
    font-size: 11.5px;
  }

  dd {
    margin: 0;
    word-break: break-all;
    user-select: text;
  }

  .form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
  }

  .action-btn {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--muted);
    font-family: var(--ui);
    font-size: 11.5px;
    padding: 5px 12px;
    cursor: pointer;
    border-radius: 2px;
    letter-spacing: 0.05em;
  }

  .action-btn:hover {
    color: var(--text);
    border-color: var(--muted);
  }

  .action-btn.accent {
    color: var(--accent);
    border-color: var(--accent);
  }

  .action-btn.accent:hover {
    background: var(--accent-soft);
  }
</style>
