<script lang="ts">
  import { SubmitPassphrase } from "../../wailsjs/go/main/App";
  import { createEventDispatcher, onMount } from "svelte";

  export let keyPath: string;

  const dispatch = createEventDispatcher<{ close: void }>();

  let passphrase = "";
  let submitting = false;
  let inputEl: HTMLInputElement | null = null;

  onMount(() => {
    inputEl?.focus();
  });

  async function handleSubmit() {
    submitting = true;
    await SubmitPassphrase(passphrase);
    passphrase = "";
    submitting = false;
    dispatch("close");
  }

  async function handleCancel() {
    await SubmitPassphrase("");
    dispatch("close");
  }
</script>

<div class="passphrase-backdrop">
  <div class="passphrase-panel">
    <div class="panel-header">
      <span class="panel-title">// passphrase required</span>
    </div>

    <div class="panel-body">
      <div class="key-info">
        <span class="key-label">key</span>
        <span class="key-path">{keyPath}</span>
      </div>

      <form on:submit|preventDefault={handleSubmit}>
        <input
          type="password"
          bind:value={passphrase}
          bind:this={inputEl}
          class="passphrase-input"
          placeholder="enter passphrase..."
        />
        <div class="form-actions">
          <button
            type="submit"
            class="action-btn accent"
            disabled={!passphrase || submitting}
          >
            {submitting ? "// unlocking..." : "[ unlock ]"}
          </button>
          <button
            type="button"
            class="action-btn"
            on:click={handleCancel}
          >
            [ cancel ]
          </button>
        </div>
      </form>
    </div>
  </div>
</div>

<style>
  .passphrase-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.8);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 50;
  }

  .passphrase-panel {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 2px;
    width: 100%;
    max-width: 400px;
    margin: 0 16px;
    box-shadow: 0 0 40px rgba(0, 0, 0, 0.6);
  }

  .panel-header {
    padding: 10px 14px;
    border-bottom: 1px solid var(--border);
  }

  .panel-title {
    font-size: 11px;
    color: var(--accent);
    font-family: 'JetBrains Mono', monospace;
    letter-spacing: 0.05em;
  }

  .panel-body {
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .key-info {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .key-label {
    font-size: 9px;
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  .key-path {
    font-size: 10px;
    color: var(--text);
    font-family: 'JetBrains Mono', monospace;
    word-break: break-all;
    background: var(--surface2);
    border: 1px solid var(--border);
    padding: 5px 8px;
    border-radius: 2px;
  }

  .passphrase-input {
    width: 100%;
    background: var(--surface2);
    border: 1px solid var(--border);
    border-radius: 2px;
    color: var(--text);
    font-family: 'JetBrains Mono', monospace;
    font-size: 12px;
    padding: 7px 10px;
    outline: none;
    transition: border-color 0.15s;
    box-sizing: border-box;
  }

  .passphrase-input:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 1px rgba(0, 255, 136, 0.1);
  }

  .form-actions {
    display: flex;
    gap: 6px;
    margin-top: 10px;
  }

  .action-btn {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    padding: 5px 12px;
    cursor: pointer;
    border-radius: 2px;
    transition: color 0.15s, border-color 0.15s, background 0.15s;
    letter-spacing: 0.05em;
  }

  .action-btn:hover:not(:disabled) {
    color: var(--text);
    border-color: var(--muted);
  }

  .action-btn.accent {
    color: var(--accent);
    border-color: var(--accent);
  }

  .action-btn.accent:hover:not(:disabled) {
    background: rgba(0, 255, 136, 0.1);
  }

  .action-btn:disabled {
    opacity: 0.4;
    cursor: default;
  }
</style>
