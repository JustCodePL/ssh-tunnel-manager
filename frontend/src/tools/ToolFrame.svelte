<script lang="ts">
  import { createEventDispatcher } from "svelte";
  import { tunnels, statuses, getStatus, connectTunnel } from "../stores/tunnels";
  import { effective, settingsFor } from "../stores/settings";
  import { needsTunnel, toolState, shouldRestart } from "../lib/toolConnection.js";
  import { shortError } from "../lib/rowLayout.js";
  import TerminalWindow from "../components/TerminalWindow.svelte";
  import RDPWindow from "../components/RDPWindow.svelte";
  import FileBrowserWindow from "../components/FileBrowserWindow.svelte";
  import EditorTool from "./EditorTool.svelte";
  import DockerWindow from "../components/DockerWindow.svelte";
  import MonitorWindow from "../components/MonitorWindow.svelte";
  import DiskWindow from "../components/DiskWindow.svelte";
  import TunnelLogs from "../components/TunnelLogs.svelte";
  import SettingsTool from "./SettingsTool.svelte";
  import TunnelSettingsTool from "./TunnelSettingsTool.svelte";

  export let spec: { tool: string; tunnelId: string; remoteHost?: string; remotePort?: number; path?: string };
  export let tabId: string;
  export let visible = true;

  const dispatch = createEventDispatcher<{ close: void }>();

  let generation = 0;
  let wasConnected = false;
  let state = "connecting";
  let ended = false;
  let busy = false;

  $: tunnel = $tunnels.find((t) => t.id === spec.tunnelId);
  $: forward = spec.remoteHost
    ? tunnel?.portForwards?.find((pf) => pf.remoteHost === spec.remoteHost && pf.remotePort === spec.remotePort) ?? null
    : null;
  $: status = getStatus($statuses, spec.tunnelId);
  $: settings = settingsFor($effective, spec.tunnelId);
  $: event = $statuses[spec.tunnelId];
  $: updateState(status, settings.afterDisconnect);

  function updateState(next: string, mode: string) {
    const needs = needsTunnel(spec);
    const computed = toolState(next, needs, wasConnected, mode);
    if (shouldRestart(state, computed, mode)) {
      generation += 1;
      ended = false;
    }
    if (next === "connected") wasConnected = true;
    state = computed;
  }

  async function reconnect() {
    busy = true;
    try {
      if (needsTunnel(spec) && status !== "connected") await connectTunnel(spec.tunnelId);
      generation += 1;
      ended = false;
    } catch (e) {
      console.error("reconnect failed:", e);
    } finally {
      busy = false;
    }
  }

  function sessionEnded() {
    ended = true;
  }

  $: showTool = state === "live" || wasConnected;
</script>

<div class="frame" data-testid="tool" data-tool={spec.tool} data-state={state}>
  {#if spec.tool === "settings"}
    <SettingsTool />
  {:else if !tunnel}
    <p class="message">This tunnel no longer exists.</p>
  {:else if spec.tool === "tunnel-settings"}
    <TunnelSettingsTool {tunnel} on:close={() => dispatch("close")} />
  {:else if spec.tool === "logs"}
    <TunnelLogs tunnelId={tunnel.id} tunnelName={tunnel.name} />
  {:else}
    {#if showTool}
      {#key generation}
        {#if spec.tool === "terminal"}
          <TerminalWindow {tunnel} {forward} {tabId} {visible} on:ended={sessionEnded} />
        {:else if spec.tool === "rdp"}
          <RDPWindow {tunnel} {forward} {tabId} clipboardMode={settings.clipboard} />
        {:else if spec.tool === "files"}
          <FileBrowserWindow {tunnel} {forward} {tabId} />
        {:else if spec.tool === "editor"}
          <EditorTool {tunnel} {forward} {tabId} path={spec.path ?? ""} on:close={() => dispatch("close")} />
        {:else if spec.tool === "docker"}
          <DockerWindow {tunnel} active={visible && state === "live"} intervalSeconds={settings.refresh.docker} />
        {:else if spec.tool === "htop"}
          <MonitorWindow {tunnel} active={visible && state === "live"} intervalSeconds={settings.refresh.htop} />
        {:else if spec.tool === "disk"}
          <DiskWindow {tunnel} active={visible && state === "live"} intervalSeconds={settings.refresh.disk} />
        {/if}
      {/key}
    {/if}

    {#if state === "connecting"}
      <div class="status-overlay"><p>Connecting to {tunnel.name}</p></div>
    {:else if state === "reconnecting"}
      <div class="status-overlay" data-testid="overlay-reconnecting"><p>Connection lost. Reconnecting to {tunnel.name}</p></div>
    {:else if state === "lost"}
      <div class="status-overlay" data-testid="overlay-lost">
        <p>{status === "error" ? shortError(event?.error) : `${tunnel.name} is disconnected.`}</p>
        <button type="button" class="btn primary" disabled={busy} data-testid="reconnect" on:click={reconnect}>Reconnect</button>
      </div>
    {:else if ended}
      <div class="status-overlay" data-testid="overlay-ended">
        <p>The session ended.</p>
        <span class="actions">
          <button type="button" class="btn primary" disabled={busy} data-testid="reconnect" on:click={reconnect}>Reconnect</button>
          <button type="button" class="btn" on:click={() => dispatch("close")}>Close tab</button>
        </span>
      </div>
    {/if}
  {/if}
</div>

<style>
  .frame {
    position: relative;
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .message {
    padding: 24px;
    text-align: center;
    color: var(--muted);
  }
  .actions {
    display: flex;
    gap: 8px;
  }
</style>
