<script lang="ts">
  import type { TunnelConfig, TunnelStatus } from "../types";
  import { connectTunnel, disconnectTunnel, deleteTunnel, setTunnelPinned } from "../stores/tunnels";
  import StatusBadge from "./StatusBadge.svelte";
  import { createEventDispatcher } from "svelte";

  export let tunnel: TunnelConfig;
  export let status: TunnelStatus = "disconnected";

  const dispatch = createEventDispatcher<{ edit: TunnelConfig; logs: TunnelConfig; terminal: TunnelConfig }>();

  let actionLoading = false;

  async function handleConnect() {
    actionLoading = true;
    try {
      await connectTunnel(tunnel.id);
    } catch (e) {
      console.error("connect failed:", e);
    } finally {
      actionLoading = false;
    }
  }

  async function handleDisconnect() {
    actionLoading = true;
    try {
      await disconnectTunnel(tunnel.id);
    } catch (e) {
      console.error("disconnect failed:", e);
    } finally {
      actionLoading = false;
    }
  }

  async function handleDelete() {
    if (!confirm(`Delete tunnel "${tunnel.name}"?`)) return;
    await deleteTunnel(tunnel.id);
  }

  async function handleTogglePin() {
    await setTunnelPinned(tunnel.id, !tunnel.pinned);
  }

  $: isActive = status === "connected" || status === "connecting" || status === "reconnecting";
  $: isConnected = status === "connected";
  $: isError = status === "error";
</script>

<div
  class="tunnel-card"
  class:connected={isConnected}
  class:error={isError}
  data-has-color={tunnel.color ? "true" : undefined}
  style={tunnel.color ? `--tunnel-tint: ${tunnel.color};` : undefined}
>
  <div class="card-header">
    <div class="card-info">
      <div class="card-name-row">
        <span class="card-name">{tunnel.name}</span>
        <StatusBadge {status} />
      </div>
      <div class="card-host">{tunnel.user}@{tunnel.host}:{tunnel.port}</div>
      {#if tunnel.sourceFileLabel || tunnel.sourceFile}
        <div class="card-source">
          plik config: {tunnel.sourceFileLabel ?? tunnel.sourceFile}
        </div>
      {/if}
      {#if tunnel.portForwards && tunnel.portForwards.length > 0}
        <div class="card-forwards">
          {#each tunnel.portForwards as pf}
            <span class="forward-tag">
              {pf.localPort}→{pf.remoteHost}:{pf.remotePort}
            </span>
          {/each}
        </div>
      {/if}
    </div>

    <div class="card-actions">
      {#if isActive}
        <button
          class="action-btn disconnect"
          on:click={handleDisconnect}
          disabled={actionLoading}
        >
          disconnect
        </button>
      {:else}
        <button
          class="action-btn connect"
          on:click={handleConnect}
          disabled={actionLoading}
        >
          connect
        </button>
      {/if}
      <button
        class="action-btn pin"
        class:pinned={tunnel.pinned}
        on:click={handleTogglePin}
        title={tunnel.pinned ? "Odepnij" : "Przypnij"}
      >{tunnel.pinned ? "unpin" : "pin"}</button>
      <button class="action-btn secondary" on:click={() => dispatch("logs", tunnel)}>logs</button>
      <button class="action-btn secondary" on:click={() => dispatch("terminal", tunnel)}>term</button>
      <button class="action-btn secondary" on:click={() => dispatch("edit", tunnel)}>edit</button>
      <button class="action-btn danger" on:click={handleDelete}>del</button>
    </div>
  </div>
</div>

<style>
  .tunnel-card {
    --tunnel-tint: var(--border);
    position: relative;
    overflow: hidden;
    background: var(--surface);
    border: 1px solid var(--border);
    border-left: 4px solid var(--border);
    padding: 10px 12px;
    border-radius: 2px;
    transition: border-color 0.15s;
  }

  .tunnel-card[data-has-color="true"] {
    border-left-color: var(--tunnel-tint);
  }

  .tunnel-card[data-has-color="true"]::after {
    content: "";
    position: absolute;
    inset: 0;
    background: var(--tunnel-tint);
    opacity: 0.06;
    pointer-events: none;
  }

  .tunnel-card:hover {
    border-color: #333333;
    border-left-color: #333333;
  }

  .tunnel-card.connected {
    border-left-color: #00ff88;
    box-shadow: inset 0 0 0 0 transparent, -2px 0 6px rgba(0, 255, 136, 0.15);
  }

  .tunnel-card.error {
    border-left-color: #ff4444;
    box-shadow: -2px 0 6px rgba(255, 68, 68, 0.15);
  }

  .card-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    position: relative;
    z-index: 1;
  }

  .card-info {
    min-width: 0;
    flex: 1;
    position: relative;
    z-index: 1;
  }

  .card-name-row {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 3px;
  }

  .card-name {
    font-size: 12px;
    font-weight: 600;
    color: var(--accent);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .card-host {
    font-size: 10px;
    color: var(--accent);
    font-family: 'JetBrains Mono', monospace;
    margin-bottom: 4px;
  }

  .card-source {
    font-size: 9px;
    color: var(--accent);
    font-family: 'JetBrains Mono', monospace;
    margin-bottom: 4px;
  }

  .card-forwards {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    margin-top: 4px;
  }

  .forward-tag {
    font-size: 9px;
    font-family: 'JetBrains Mono', monospace;
    color: var(--accent);
    background: rgba(0, 255, 136, 0.08);
    border: 1px solid var(--accent);
    padding: 1px 5px;
    border-radius: 2px;
  }

  .card-actions {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-shrink: 0;
    position: relative;
    z-index: 1;
  }

  .action-btn {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    font-size: 9px;
    padding: 2px 7px;
    cursor: pointer;
    border-radius: 2px;
    text-transform: lowercase;
    transition: color 0.15s, border-color 0.15s, background 0.15s;
    white-space: nowrap;
  }

  .action-btn:disabled {
    opacity: 0.4;
    cursor: default;
  }

  .action-btn.connect {
    color: #00ff88;
    border-color: #00ff88;
  }

  .action-btn.connect:hover:not(:disabled) {
    background: rgba(0, 255, 136, 0.1);
  }

  .action-btn.disconnect {
    color: #ff4444;
    border-color: #ff4444;
  }

  .action-btn.disconnect:hover:not(:disabled) {
    background: rgba(255, 68, 68, 0.1);
  }

  .action-btn.secondary:hover:not(:disabled) {
    color: var(--accent2);
    border-color: var(--accent2);
  }

  .action-btn.danger:hover:not(:disabled) {
    color: #ff4444;
    border-color: #ff4444;
  }

  .action-btn.pin {
    color: var(--muted);
    border-color: var(--border);
  }

  .action-btn.pin:hover:not(:disabled) {
    color: var(--accent2);
    border-color: var(--accent2);
  }

  .action-btn.pin.pinned {
    color: var(--accent2);
    border-color: var(--accent2);
  }
</style>
