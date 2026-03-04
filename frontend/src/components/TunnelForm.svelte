<script lang="ts">
  import type { TunnelConfig, PortForward } from "../types";
  import { addTunnel, updateTunnel } from "../stores/tunnels";
  import { createEventDispatcher } from "svelte";

  export let tunnel: TunnelConfig | null = null;

  const dispatch = createEventDispatcher<{ close: void }>();

  let name = tunnel?.name ?? "";
  let host = tunnel?.host ?? "";
  let port = tunnel?.port ?? 22;
  let user = tunnel?.user ?? "";
  let keyPath = tunnel?.keyPath ?? "";
  let group = tunnel?.group ?? "";
  let autoConnect = tunnel?.autoConnect ?? false;
  let tunnelColor = tunnel?.color ?? "";
  let proxyCommand = tunnel?.proxyCommand ?? "";
  let proxyJump = tunnel?.proxyJump ?? "";

  const presetColors = [
    { name: "red", hex: "#ef4444" },
    { name: "orange", hex: "#f97316" },
    { name: "yellow", hex: "#eab308" },
    { name: "green", hex: "#22c55e" },
    { name: "cyan", hex: "#06b6d4" },
    { name: "blue", hex: "#3b82f6" },
    { name: "purple", hex: "#a855f7" },
    { name: "pink", hex: "#ec4899" },
  ];
  let portForwards: PortForward[] = tunnel?.portForwards?.length
    ? [...tunnel.portForwards]
    : [];
  let showAdvanced = !!(tunnel?.proxyCommand || tunnel?.proxyJump);

  let saving = false;
  let error = "";

  $: isEdit = tunnel !== null;

  function addPortForward() {
    portForwards = [...portForwards, { localPort: 0, remoteHost: "127.0.0.1", remotePort: 0, description: "" }];
  }

  function removePortForward(index: number) {
    portForwards = portForwards.filter((_, i) => i !== index);
  }

  async function handleSubmit() {
    error = "";
    const trimmedName = name.trim();
    if (!trimmedName) {
      error = "name is required";
      return;
    }
    if (/\s/.test(trimmedName)) {
      error = "name cannot contain spaces (used as SSH host alias)";
      return;
    }
    const validForwards = portForwards.filter(
      (pf) => pf.localPort > 0 && pf.remotePort > 0
    );

    saving = true;
    try {
      const cfg: TunnelConfig = {
        id: tunnel?.id ?? trimmedName,
        name: trimmedName,
        host: host.trim(),
        port,
        user: user.trim(),
        keyPath: keyPath.trim(),
        portForwards: validForwards,
        proxyCommand: proxyCommand.trim() || undefined,
        proxyJump: proxyJump.trim() || undefined,
        color: tunnelColor || undefined,
        group: group.trim(),
        autoConnect,
      };
      if (isEdit) {
        await updateTunnel(cfg);
      } else {
        await addTunnel(cfg);
      }
      dispatch("close");
    } catch (e: any) {
      error = e?.message ?? String(e);
    } finally {
      saving = false;
    }
  }
</script>

<div class="form-container">
  <div class="form-header">
    <span class="form-title">// {isEdit ? "edit tunnel" : "new tunnel"}</span>
    <button class="close-btn" on:click={() => dispatch("close")}>[ cancel ]</button>
  </div>

  {#if error}
    <div class="error-bar">! {error}</div>
  {/if}

  <form on:submit|preventDefault={handleSubmit} class="form-body">
    <div class="form-grid two-col">
      <div class="field">
        <label class="field-label">name</label>
        <input
          type="text"
          bind:value={name}
          class="field-input"
          placeholder="production-db"
        />
      </div>
      <div class="field">
        <label class="field-label">group</label>
        <input
          type="text"
          bind:value={group}
          class="field-input"
          placeholder="production"
        />
      </div>
    </div>

    <div class="field">
      <label class="field-label">color</label>
      <div class="color-row">
        <button
          type="button"
          class="color-swatch none-swatch"
          class:selected={tunnelColor === ""}
          title="no color"
          on:click={() => (tunnelColor = "")}
        >×</button>
        {#each presetColors as c}
          <button
            type="button"
            class="color-swatch"
            class:selected={tunnelColor === c.hex}
            style="background-color: {c.hex};"
            title={c.name}
            on:click={() => (tunnelColor = c.hex)}
          />
        {/each}
      </div>
    </div>

    <div class="form-grid three-col">
      <div class="field col-span-2">
        <label class="field-label">host</label>
        <input
          type="text"
          bind:value={host}
          class="field-input"
          placeholder="bastion.example.com"
        />
      </div>
      <div class="field">
        <label class="field-label">port</label>
        <input
          type="number"
          bind:value={port}
          class="field-input"
          min="1"
          max="65535"
        />
      </div>
    </div>

    <div class="form-grid two-col">
      <div class="field">
        <label class="field-label">user</label>
        <input
          type="text"
          bind:value={user}
          class="field-input"
          placeholder="deploy"
        />
      </div>
      <div class="field">
        <label class="field-label">ssh key path</label>
        <input
          type="text"
          bind:value={keyPath}
          class="field-input"
          placeholder="~/.ssh/id_ed25519"
        />
      </div>
    </div>

    <div class="field">
      <div class="forwards-header">
        <label class="field-label">port forwards</label>
        <button type="button" class="link-btn" on:click={addPortForward}>+ add</button>
      </div>
      {#each portForwards as pf, i}
        <div class="forward-row">
          <input
            type="number"
            bind:value={pf.localPort}
            class="field-input port-input"
            placeholder="local"
            min="1"
            max="65535"
          />
          <span class="arrow">→</span>
          <input
            type="text"
            bind:value={pf.remoteHost}
            class="field-input"
            placeholder="127.0.0.1"
          />
          <span class="colon">:</span>
          <input
            type="number"
            bind:value={pf.remotePort}
            class="field-input port-input"
            placeholder="remote"
            min="1"
            max="65535"
          />
          <button type="button" class="remove-btn" on:click={() => removePortForward(i)}>×</button>
        </div>
        <input
          type="text"
          bind:value={pf.description}
          class="field-input desc-input"
          placeholder="description"
        />
      {/each}
    </div>

    <div class="field">
      <button
        type="button"
        class="link-btn"
        on:click={() => (showAdvanced = !showAdvanced)}
      >
        {showAdvanced ? "▾" : "▸"} advanced (proxy)
      </button>
      {#if showAdvanced}
        <div class="advanced-fields">
          <div class="field">
            <label class="field-label">proxy command</label>
            <input
              type="text"
              bind:value={proxyCommand}
              class="field-input mono"
              placeholder="aws ssm start-session --target %h ..."
            />
          </div>
          <div class="field">
            <label class="field-label">proxy jump</label>
            <input
              type="text"
              bind:value={proxyJump}
              class="field-input"
              placeholder="bastion.example.com"
            />
          </div>
          <p class="hint">tokens: %h (host) %p (port) %r (user). use one of proxycommand or proxyjump.</p>
        </div>
      {/if}
    </div>

    <label class="checkbox-label">
      <input type="checkbox" bind:checked={autoConnect} />
      <span>auto-connect on startup</span>
    </label>

    <div class="form-actions">
      <button
        type="submit"
        class="submit-btn"
        disabled={saving}
      >
        {saving ? "// saving..." : isEdit ? "[ update ]" : "[ add tunnel ]"}
      </button>
    </div>
  </form>
</div>

<style>
  .form-container {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 2px;
  }

  .form-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 14px;
    border-bottom: 1px solid var(--border);
  }

  .form-title {
    font-size: 11px;
    color: var(--accent);
    font-family: 'JetBrains Mono', monospace;
    letter-spacing: 0.05em;
  }

  .close-btn {
    background: transparent;
    border: none;
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    cursor: pointer;
    padding: 2px 6px;
    transition: color 0.15s;
  }

  .close-btn:hover {
    color: var(--text);
  }

  .error-bar {
    background: rgba(255, 68, 68, 0.1);
    border-bottom: 1px solid rgba(255, 68, 68, 0.3);
    color: #ff4444;
    font-size: 10px;
    padding: 6px 14px;
    font-family: 'JetBrains Mono', monospace;
  }

  .form-body {
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .form-grid {
    display: grid;
    gap: 8px;
  }

  .two-col {
    grid-template-columns: 1fr 1fr;
  }

  .three-col {
    grid-template-columns: 1fr 1fr 0.5fr;
  }

  .col-span-2 {
    grid-column: span 2;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .field-label {
    font-size: 9px;
    font-family: 'JetBrains Mono', monospace;
    color: var(--accent);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  .field-input {
    background: var(--surface2);
    border: 1px solid var(--border);
    border-radius: 2px;
    color: var(--text);
    font-family: 'JetBrains Mono', monospace;
    font-size: 11px;
    padding: 5px 8px;
    outline: none;
    transition: border-color 0.15s;
    width: 100%;
  }

  .field-input:focus {
    border-color: var(--accent);
  }

  .field-input.mono {
    font-size: 10px;
  }

  .color-row {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }

  .color-swatch {
    width: 20px;
    height: 20px;
    border-radius: 2px;
    border: 1px solid #333;
    cursor: pointer;
    transition: transform 0.1s, border-color 0.15s;
    padding: 0;
  }

  .color-swatch:hover {
    transform: scale(1.15);
  }

  .color-swatch.selected {
    border-color: #fff;
    border-width: 2px;
  }

  .none-swatch {
    background: var(--surface2);
    color: var(--muted);
    font-size: 11px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .forwards-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .forward-row {
    display: flex;
    align-items: center;
    gap: 4px;
    margin-bottom: 4px;
  }

  .port-input {
    width: 70px;
    flex-shrink: 0;
  }

  .arrow, .colon {
    color: var(--muted);
    font-size: 11px;
    flex-shrink: 0;
  }

  .remove-btn {
    background: transparent;
    border: none;
    color: var(--muted);
    cursor: pointer;
    font-size: 14px;
    padding: 0 2px;
    flex-shrink: 0;
    transition: color 0.15s;
  }

  .remove-btn:hover {
    color: #ff4444;
  }

  .desc-input {
    margin-bottom: 4px;
    font-size: 10px;
    color: var(--muted);
  }

  .link-btn {
    background: transparent;
    border: none;
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    cursor: pointer;
    padding: 0;
    text-align: left;
    transition: color 0.15s;
  }

  .link-btn:hover {
    color: var(--accent2);
  }

  .advanced-fields {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-top: 8px;
    padding-left: 10px;
    border-left: 1px solid var(--border);
  }

  .hint {
    font-size: 9px;
    color: #333;
    margin: 0;
    font-family: 'JetBrains Mono', monospace;
  }

  .checkbox-label {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    font-size: 10px;
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
  }

  .form-actions {
    display: flex;
    justify-content: flex-end;
    padding-top: 4px;
    border-top: 1px solid var(--border);
  }

  .submit-btn {
    background: transparent;
    border: 1px solid var(--accent);
    color: var(--accent);
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    padding: 5px 14px;
    cursor: pointer;
    border-radius: 2px;
    transition: background 0.15s;
    letter-spacing: 0.05em;
  }

  .submit-btn:hover:not(:disabled) {
    background: rgba(0, 255, 136, 0.1);
  }

  .submit-btn:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
