<script lang="ts">
  import { createEventDispatcher } from "svelte";
  import type { TunnelConfig, TunnelStatus, StatusEvent } from "../types";
  import { connectTunnel, disconnectTunnel } from "../stores/tunnels";
  import { CopyToClipboard } from "../../bindings/ssh-tunnel-manager/app";
  import { showToast } from "../stores/toast";
  import { openTool } from "../lib/openTool";
  import { toolsIn, shortError, isOn } from "../lib/rowLayout.js";
  import { forwardAddress, forwardAttach, forwardTags } from "../lib/forwards.js";
  import { effectiveForwardProtocol } from "../lib/portForwards.js";
  import type { EffectiveSettings } from "../stores/settings";
  import Icon from "./Icon.svelte";
  import AttachButton from "./AttachButton.svelte";
  import ResourceWidget from "./ResourceWidget.svelte";

  export let tunnel: TunnelConfig;
  export let event: StatusEvent | undefined = undefined;
  export let settings: EffectiveSettings;
  export let caps: { docker?: boolean; htop?: boolean; rdp?: boolean } | undefined = undefined;
  export let attachOptions: { protocol: string; label: string }[] = [];
  export let expanded = false;
  export let selected = false;

  const dispatch = createEventDispatcher<{ toggle: void; select: void }>();

  let busy = false;

  $: status = (event?.status ?? "disconnected") as TunnelStatus;
  $: on = isOn(status);
  $: failed = status === "error";
  $: connected = status === "connected";
  $: row = settings.row;
  $: rowTools = toolsIn(row, "row", caps);
  $: panelTools = toolsIn(row, "panel", caps);
  $: forwards = (tunnel.portForwards ?? []).length;
  $: address = `${tunnel.user}@${tunnel.host}:${tunnel.port}`;

  async function flip() {
    if (busy) return;
    busy = true;
    try {
      if (on) await disconnectTunnel(tunnel.id);
      else await connectTunnel(tunnel.id);
    } catch (e) {
      console.error("connection change failed:", e);
    } finally {
      busy = false;
    }
  }

  async function copy(text: string) {
    try {
      await CopyToClipboard(text);
      showToast(`Copied ${text}`, "success");
    } catch (e) {
      console.error("copy failed:", e);
      showToast("Could not copy to the clipboard", "error");
    }
  }

  function tool(name: string) {
    void openTool({ tool: name, tunnelId: tunnel.id });
  }

  function forwardTool(name: string, pf: { remoteHost: string; remotePort: number }) {
    void openTool({ tool: name, tunnelId: tunnel.id, remoteHost: pf.remoteHost, remotePort: pf.remotePort });
  }
</script>

<div class="tunnel" class:open={expanded} data-testid="tunnel" data-name={tunnel.name} data-status={status}>
  <!-- svelte-ignore a11y-click-events-have-key-events -->
  <div
    class="row"
    class:selected
    class:tinted={tunnel.color}
    role="button"
    tabindex="-1"
    aria-expanded={expanded}
    style={tunnel.color ? `--tunnel-color: ${tunnel.color}` : undefined}
    on:click={() => dispatch("toggle")}
  >
    <span class="chevron" class:down={expanded}><Icon name="chevron" size={12} /></span>
    <span class="dot {status}" title={String(status)}>{#if failed}!{/if}</span>
    <span class="name">{tunnel.name}</span>
    <div class="slots">
      {#if failed}
        <span class="reason" title={event?.error}>{shortError(event?.error)}</span>
      {/if}
      {#if row.forwards === "row" && forwards > 0}
        <span class="count">{forwards} forward{forwards > 1 ? "s" : ""}</span>
      {/if}
      {#if row.address === "row"}
        <code class="address">{address}</code>
      {/if}
      {#if row.stats === "row" && connected}
        <ResourceWidget tunnelId={tunnel.id} intervalSeconds={settings.refresh.stats} />
      {/if}
    </div>
    {#each expanded ? [] : rowTools as item}
      <button
        type="button"
        class="icon-btn"
        title={item.label}
        aria-label={`${item.label} on ${tunnel.name}`}
        on:click|stopPropagation={() => tool(item.tool)}
      ><Icon name={item.key} /></button>
    {/each}
    {#if row.attach === "row" && !expanded}
      <AttachButton tunnelId={tunnel.id} options={attachOptions} small />
    {/if}
    <button
      type="button"
      class="icon-btn"
      title="Tunnel settings"
      aria-label={`Settings of ${tunnel.name}`}
      data-testid="tunnel-settings"
      on:click|stopPropagation={() => tool("tunnel-settings")}
    ><Icon name="settings" /></button>
    <button
      type="button"
      class="switch"
      class:on
      role="switch"
      aria-checked={on}
      aria-label={`${on ? "Disconnect" : "Connect"} ${tunnel.name}`}
      disabled={busy}
      on:click|stopPropagation={flip}
    ></button>
  </div>

  {#if expanded}
    <div class="panel">
      <div class="panel-head">
        {#if row.address !== "off"}<code class="address full">{address}</code>{/if}
        {#if row.attach !== "off"}
          <AttachButton tunnelId={tunnel.id} options={attachOptions} align="left" />
        {/if}
        {#each panelTools as item}
          <button type="button" class="btn" on:click={() => tool(item.tool)}>
            <Icon name={item.key} />{item.label}
          </button>
        {/each}
      </div>

      {#if row.stats !== "off" && row.stats !== "row" && connected}
        <ResourceWidget tunnelId={tunnel.id} intervalSeconds={settings.refresh.stats} />
      {/if}

      {#if failed && event?.error}
        <p class="error-detail">{event.error}</p>
      {/if}

      {#if forwards > 0}
        <p class="label">Forwards, click an address to copy</p>
        {#each tunnel.portForwards as pf}
          {@const addr = forwardAddress(pf)}
          {@const attach = forwardAttach(pf)}
          <div class="forward" data-testid="forward">
            <button type="button" class="forward-address" title={`Copy ${addr}`} on:click={() => copy(addr)}>
              <code>{addr}</code>
              {#each forwardTags(pf) as tag}<span class="tag">{tag}</span>{/each}
              {#if pf.description}<span class="desc">{pf.description}</span>{/if}
              <small>to {pf.remoteHost}:{pf.remotePort}</small>
            </button>
            {#if attach}
              <button type="button" class="btn small" title={attach.label} on:click={() => forwardTool(attach.tool, pf)}>
                <Icon name={attach.tool === "rdp" ? "rdp" : "terminal"} />Attach
              </button>
              {#if effectiveForwardProtocol(pf) === "ssh"}
                <button type="button" class="btn small" on:click={() => forwardTool("files", pf)}>Files</button>
              {/if}
            {/if}
          </div>
        {/each}
      {/if}
    </div>
  {/if}
</div>

<style>
  :global(.tunnel + .tunnel > .row) {
    border-top: 1px solid var(--border-soft);
  }
  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    min-height: 38px;
    padding: 0 10px 0 9px;
    cursor: pointer;
    border-left: 3px solid var(--tunnel-color, transparent);
  }
  .row:hover,
  .open .row {
    background: var(--surface);
  }
  .row.tinted {
    background: color-mix(in srgb, var(--tunnel-color) 10%, transparent);
  }
  .row.tinted:hover,
  .open .row.tinted {
    background: color-mix(in srgb, var(--tunnel-color) 16%, var(--surface));
  }
  .row.selected {
    box-shadow: inset 0 0 0 1px var(--accent2);
  }
  .chevron {
    display: inline-flex;
    color: var(--muted);
  }
  .chevron.down {
    transform: rotate(90deg);
  }
  .dot {
    width: 8px;
    height: 8px;
    flex: none;
    border-radius: 50%;
    border: 1.5px solid var(--muted);
    display: inline-grid;
    place-items: center;
  }
  .dot.connected {
    background: var(--live);
    border-color: var(--live);
    box-shadow: var(--glow);
  }
  .dot.connecting,
  .dot.reconnecting {
    border-color: var(--warning);
    background: var(--warning);
    opacity: 0.8;
  }
  .dot.error {
    width: 14px;
    height: 14px;
    background: var(--error);
    border-color: var(--error);
    color: var(--bg);
    font-size: 12px;
    font-weight: 700;
    line-height: 1;
  }
  .name {
    font-size: 13px;
    font-weight: 600;
    white-space: nowrap;
  }
  .tunnel[data-status="disconnected"] .name {
    font-weight: 450;
    color: var(--muted);
  }
  .slots {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 10px;
    margin-left: 4px;
    overflow: hidden;
    font-size: 12px;
    color: var(--muted);
    white-space: nowrap;
  }
  .reason {
    color: var(--error);
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .address {
    font-size: 11.5px;
    color: var(--muted);
  }
  .panel {
    background: var(--surface);
    padding: 2px 12px 12px 45px;
    font-size: 12.5px;
  }
  .panel-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    margin-bottom: 8px;
  }
  .address.full {
    flex-basis: 100%;
    margin-bottom: 2px;
  }
  .push {
    margin-left: auto;
  }
  .label {
    margin: 10px 0 3px;
    font-size: 12px;
    color: var(--muted);
  }
  .error-detail {
    margin: 4px 0;
    font-size: 12px;
    color: var(--error);
    word-break: break-word;
  }
  .forward {
    display: flex;
    align-items: center;
    gap: 6px;
    min-height: 30px;
    border-top: 1px solid var(--border-soft);
  }
  .forward-address {
    flex: 1;
    min-width: 0;
    padding: 3px 0;
    text-align: left;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .forward-address code {
    font-size: 11.5px;
    font-weight: 500;
  }
  .forward-address:hover code {
    color: var(--accent2);
  }
  .tag {
    margin-left: 6px;
    font-size: 12px;
    color: var(--accent2);
  }
  .desc {
    margin-left: 6px;
    font-size: 11.5px;
    color: var(--muted);
  }
  .forward-address small {
    display: block;
    font: 11px var(--mono);
    color: var(--muted);
  }
</style>
