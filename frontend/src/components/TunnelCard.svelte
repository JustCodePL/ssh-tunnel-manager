<script lang="ts">
  import type { TunnelConfig, TunnelStatus } from "../types";
  import { PORTLESS_TLD } from "../types";
  import { connectTunnel, disconnectTunnel, deleteTunnel, setTunnelPinned } from "../stores/tunnels";
  import StatusBadge from "./StatusBadge.svelte";
  import ResourceWidget from "./ResourceWidget.svelte";
  import DiskWindow from "./DiskWindow.svelte";
  import DockerWindow from "./DockerWindow.svelte";
  import MonitorWindow from "./MonitorWindow.svelte";
  import { CopyToClipboard } from "../../wailsjs/go/main/App";
  import { showResourceStats } from "../stores/prefs";
  import { capabilities, refreshCapabilities, verifyTool, type ToolName } from "../stores/capabilities";
  import { showToast } from "../stores/toast";
  import { createEventDispatcher } from "svelte";

  export let tunnel: TunnelConfig;
  export let status: TunnelStatus = "disconnected";

  const dispatch = createEventDispatcher<{ edit: TunnelConfig; logs: TunnelConfig; terminal: TunnelConfig; files: TunnelConfig }>();

  let actionLoading = false;

  let showDisk = false;
  let showDocker = false;
  let showHtop = false;

  // Tool buttons are driven by persisted capabilities, so they stay stable
  // across reconnects (and app restarts) without re-probing. capsKnown means
  // we've probed this host at least once, so its monitoring buttons stay
  // visible even while disconnected (disabled until reconnected).
  $: caps = $capabilities[tunnel.id] ?? { docker: false, htop: false };
  $: capsKnown = !!$capabilities[tunnel.id];

  // Open a tool window, but re-verify with `command -v` first. If the probe
  // ran and the tool is gone (host reinstalled), notify and hide the button.
  // A probe error (connection issue) is NOT treated as "missing".
  async function openTool(tool: ToolName, open: () => void) {
    const result = await verifyTool(tunnel.id, tool);
    if (result === "absent") {
      showToast(`${tool} is no longer available on this host`, "error");
      return;
    }
    open();
  }

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

  async function handleCopyTag(text: string) {
    try {
      await CopyToClipboard(text);
      showToast(`copied ${text}`, "success");
    } catch (e) {
      console.error("clipboard copy failed:", e);
      showToast("clipboard copy failed", "error");
    }
  }

  $: isActive = status === "connected" || status === "connecting" || status === "reconnecting";
  $: isConnected = status === "connected";
  $: isError = status === "error";

  // Re-probe + persist tools on every connect so newly installed tools (e.g.
  // docker) are detected; close any open monitoring windows when the
  // connection drops.
  $: if (isConnected) {
    refreshCapabilities(tunnel.id);
  } else {
    showDisk = false;
    showDocker = false;
    showHtop = false;
  }
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
      {#if isConnected && $showResourceStats}
        <ResourceWidget tunnelId={tunnel.id} />
      {/if}
      {#if tunnel.sourceFileLabel || tunnel.sourceFile}
        <div class="card-source">
          config file: {tunnel.sourceFileLabel ?? tunnel.sourceFile}
        </div>
      {/if}
      {#if tunnel.portForwards && tunnel.portForwards.length > 0}
        <div class="card-forwards">
          {#each tunnel.portForwards as pf}
            {#if pf.portless && pf.domain}
              {@const exposePort = pf.exposePort && pf.exposePort > 0 ? pf.exposePort : pf.remotePort}
              {@const host = `${pf.domain}.${PORTLESS_TLD}`}
              {@const addr = exposePort === 80
                ? `http://${host}`
                : exposePort === 443
                  ? `https://${host}`
                  : `${host}:${exposePort}`}
              <button
                type="button"
                class="forward-tag portless-tag"
                on:click={() => handleCopyTag(addr)}
                title="click to copy — portless → {pf.remoteHost}:{pf.remotePort}"
              >
                {addr}
              </button>
            {:else}
              {@const addr = `127.0.0.1:${pf.localPort}`}
              <button
                type="button"
                class="forward-tag"
                on:click={() => handleCopyTag(addr)}
                title="click to copy {addr} → {pf.remoteHost}:{pf.remotePort}"
              >
                {pf.localPort}→{pf.remoteHost}:{pf.remotePort}
              </button>
            {/if}
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
      <button class="action-btn secondary" on:click={() => dispatch("files", tunnel)}>files</button>
      {#if isConnected || capsKnown}
        <button
          class="action-btn secondary"
          on:click={() => (showDisk = true)}
          disabled={!isConnected}
          title={isConnected ? "" : "connect to view"}
        >disk</button>
      {/if}
      {#if caps.docker}
        <button
          class="action-btn secondary"
          on:click={() => openTool("docker", () => (showDocker = true))}
          disabled={!isConnected}
          title={isConnected ? "" : "connect to view"}
        >docker</button>
      {/if}
      {#if caps.htop}
        <button
          class="action-btn secondary"
          on:click={() => openTool("htop", () => (showHtop = true))}
          disabled={!isConnected}
          title={isConnected ? "" : "connect to view"}
        >htop</button>
      {/if}
      <button class="action-btn secondary" on:click={() => dispatch("edit", tunnel)}>edit</button>
      <button class="action-btn danger" on:click={handleDelete}>del</button>
    </div>
  </div>
</div>

{#if showDisk}
  <DiskWindow {tunnel} on:close={() => (showDisk = false)} />
{/if}
{#if showDocker}
  <DockerWindow {tunnel} on:close={() => (showDocker = false)} />
{/if}
{#if showHtop}
  <MonitorWindow {tunnel} on:close={() => (showHtop = false)} />
{/if}

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
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .card-host {
    font-size: 10px;
    color: var(--text);
    font-family: 'JetBrains Mono', monospace;
    margin-bottom: 4px;
  }

  .card-source {
    font-size: 9px;
    color: var(--muted);
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
    color: var(--text);
    background: var(--surface2);
    border: 1px solid var(--border);
    padding: 1px 5px;
    border-radius: 2px;
    cursor: pointer;
    transition: background 0.15s, border-color 0.15s, color 0.15s;
  }

  .forward-tag:hover {
    background: var(--surface);
    border-color: var(--accent2);
    color: var(--accent2);
  }

  .forward-tag.portless-tag {
    color: var(--accent);
    border-color: rgba(0, 255, 136, 0.4);
  }

  .forward-tag.portless-tag:hover {
    background: rgba(0, 255, 136, 0.1);
    border-color: var(--accent);
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
