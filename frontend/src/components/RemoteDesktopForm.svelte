<script lang="ts">
  import { createEventDispatcher, onMount } from "svelte";
  import { BindCredential, GetCredentialBinding, GetTunnelSettings, ListCredentials, SetTunnelSettings } from "../../bindings/ssh-tunnel-manager/app";
  import { addRemoteDesktop, updateRemoteDesktop, includedConfigFiles, tunnels, remoteDesktops, groups } from "../stores/tunnels";
  import { desktopPort } from "../lib/clients.js";
  import { openTool } from "../lib/openTool";
  import { errorText } from "../lib/runtime";
  import type { RemoteDesktop } from "../types";
  import Select from "./Select.svelte";
  import Combobox from "./Combobox.svelte";

  /** The desktop to edit, or null for a new one. */
  export let desktop: RemoteDesktop | null = null;
  export let initialGroup = "";

  const dispatch = createEventDispatcher<{ close: void }>();
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
  const CLIPBOARD_MODES = [
    { value: "", label: "App default (both ways)" },
    { value: "both", label: "Both ways" },
    { value: "to", label: "To remote" },
    { value: "from", label: "From remote" },
    { value: "off", label: "Off" },
  ];

  const isEdit = !!desktop;
  // The name the entry is saved under, once saved: a retry after a failed
  // login assignment edits it instead of adding it again.
  let savedName = desktop?.name ?? "";
  let name = desktop?.name ?? "";
  let address = desktop?.address ?? "";
  let protocol: "rdp" | "vnc" = desktop?.protocol ?? "rdp";
  let port = desktop ? String(desktop.port) : "";
  let group = desktop?.group ?? initialGroup;
  let color = desktop?.color ?? "";
  let pinned = desktop?.pinned ?? false;
  let sourceFile = desktop?.sourceFile ?? "";
  let login = "";
  let savedLogin = "";
  let clipboard = "";
  let savedClipboard = "";
  let logins: { value: string; label: string }[] = [];
  let behindHost = "";
  let error = "";
  let saving = false;

  $: defaultPort = protocol === "vnc" ? 5900 : 3389;
  $: existingGroups = [...new Set([...$tunnels.map((t) => t.group), ...$remoteDesktops.map((d) => d.group), ...$groups].filter(Boolean))].sort((a, b) => a.localeCompare(b));
  $: files = $includedConfigFiles ?? [];
  $: hostOptions = $tunnels.map((t) => ({ value: t.id, label: t.name }));

  onMount(async () => {
    try {
      const all = ((await ListCredentials()) ?? []).filter((c: any) => c.hasPassword);
      logins = [{ value: "", label: "Ask when connecting" }, ...all.map((c: any) => ({ value: c.id, label: c.name }))];
      if (desktop) {
        savedLogin = login = (await GetCredentialBinding(desktop.protocol, desktop.id, { remoteHost: "", remotePort: 0 } as any)) || "";
        savedClipboard = clipboard = (await GetTunnelSettings(desktop.id))?.clipboard ?? "";
      }
    } catch (e) {
      console.warn("reading the desktop's login failed:", e);
    }
  });

  // A standard port follows the protocol; a port typed by hand stays.
  function setProtocol(next: "rdp" | "vnc") {
    if (port === "3389" || port === "5900") port = "";
    protocol = next;
  }

  function openHostForwards(id: string) {
    if (!id) return;
    void openTool({ tool: "tunnel-settings", tunnelId: id, path: "forwards" });
  }

  async function save() {
    error = "";
    const entry = {
      id: "",
      name: name.trim(),
      address: address.trim(),
      port: desktopPort(port, protocol),
      protocol,
      group: group.trim(),
      color,
      pinned,
      sourceFile: isEdit ? desktop?.sourceFile : sourceFile,
    } as RemoteDesktop;
    saving = true;
    try {
      if (savedName) await updateRemoteDesktop(savedName, entry);
      else await addRemoteDesktop(entry);
      savedName = entry.name;
      const id = `rd:${entry.name}`;
      // A changed protocol keeps nothing of the other protocol's assignment.
      if (login !== savedLogin || (desktop && desktop.protocol !== protocol)) {
        await BindCredential(protocol, id, { remoteHost: "", remotePort: 0 } as any, login);
        savedLogin = login;
      }
      if (clipboard !== savedClipboard) {
        const current = (await GetTunnelSettings(id)) ?? {};
        await SetTunnelSettings(id, { ...current, clipboard } as any);
        savedClipboard = clipboard;
      }
      dispatch("close");
    } catch (e) {
      error = errorText(e);
    } finally {
      saving = false;
    }
  }
</script>

<div class="backdrop" data-testid="desktop-form">
  <form class="panel" on:submit|preventDefault={save} aria-labelledby="desktop-form-title">
    <div class="panel-header">
      <span class="panel-title" id="desktop-form-title">{isEdit ? `Edit ${desktop?.name}` : "New remote desktop"}</span>
      <button type="button" class="icon-btn" aria-label="Cancel" on:click={() => dispatch("close")}>×</button>
    </div>
    {#if error}<div class="error" data-testid="desktop-form-error">{error}</div>{/if}
    <div class="panel-body">
      <p class="hint">
        For a computer you reach directly, such as a Windows PC on your network. Machine behind an SSH server? Add an RDP or VNC forward on that host instead.
        {#if hostOptions.length}
          <span class="host-pick">
            <Select bind:value={behindHost} options={hostOptions} placeholder="Open a host's forwards" ariaLabel="Open a host's forwards" testid="desktop-form-host" on:change={(e) => openHostForwards(e.detail)} />
          </span>
        {/if}
      </p>

      <div class="grid two">
        <label>name <input class="field-control" data-testid="desktop-name" bind:value={name} placeholder="work-laptop" required autocomplete="off" spellcheck="false" /></label>
        <span class="field">protocol
          <span class="segmented" role="radiogroup" aria-label="Protocol">
            <button type="button" role="radio" aria-checked={protocol === "rdp"} class:on={protocol === "rdp"} data-testid="desktop-protocol-rdp" on:click={() => setProtocol("rdp")}>RDP</button>
            <button type="button" role="radio" aria-checked={protocol === "vnc"} class:on={protocol === "vnc"} data-testid="desktop-protocol-vnc" on:click={() => setProtocol("vnc")}>VNC</button>
          </span>
        </span>
      </div>
      <div class="grid address">
        <label>address <input class="field-control" data-testid="desktop-address" bind:value={address} placeholder="192.168.1.20 or laptop.lan" required autocomplete="off" spellcheck="false" /></label>
        <label>port <input class="field-control" data-testid="desktop-port" type="number" min="1" max="65535" bind:value={port} placeholder={String(defaultPort)} /></label>
      </div>

      {#if protocol === "vnc"}
        <div class="warning" data-testid="desktop-vnc-warning">VNC does not encrypt the connection. Use it only on a network you trust, or through an SSH tunnel.</div>
      {/if}

      <div class="grid two">
        <span class="field">group <Combobox bind:value={group} options={existingGroups} placeholder="Choose or type a group" testid="desktop-group" /></span>
        <span class="field">saved login <Select bind:value={login} options={logins} ariaLabel="Saved login" testid="desktop-login" /></span>
      </div>
      <div class="grid two">
        <span class="field">clipboard <Select bind:value={clipboard} options={CLIPBOARD_MODES} ariaLabel="Clipboard" testid="desktop-clipboard" /></span>
        {#if !isEdit && files.length > 1}
          <span class="field">config file
            <Select bind:value={sourceFile} options={[{ value: "", label: "~/.ssh/config" }, ...files.slice(1).map((f) => ({ value: f.path, label: f.label }))]} ariaLabel="Config file" testid="desktop-file" />
          </span>
        {:else if isEdit && desktop?.sourceFileLabel}
          <span class="field">config file <span class="muted">{desktop.sourceFileLabel}</span></span>
        {/if}
      </div>

      <div class="field">color
        <div class="colors">
          <button type="button" class="swatch none" class:selected={color === ""} title="no color" on:click={() => (color = "")}>×</button>
          {#each presetColors as c}
            <button type="button" class="swatch" class:selected={color === c.hex} style="background-color: {c.hex};" title={c.name} on:click={() => (color = c.hex)} />
          {/each}
        </div>
      </div>
      <label class="check"><input type="checkbox" data-testid="desktop-pinned" bind:checked={pinned} /> pinned</label>

      <div class="actions">
        <button type="button" class="btn" on:click={() => dispatch("close")}>Cancel</button>
        <button type="submit" class="btn primary" data-testid="desktop-save" disabled={saving}>{isEdit ? "Save" : "Add desktop"}</button>
      </div>
    </div>
  </form>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 50;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.7);
  }
  .panel {
    width: 100%;
    max-width: 520px;
    max-height: calc(100vh - 32px);
    overflow-y: auto;
    margin: 0 16px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: 0 0 40px rgba(0, 0, 0, 0.5);
  }
  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border);
  }
  .panel-title {
    font-size: 13px;
    font-weight: 600;
    color: var(--text);
  }
  .panel-body {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 12px;
    font-size: 12.5px;
  }
  .error {
    padding: 6px 12px;
    color: var(--error);
    background: color-mix(in srgb, var(--error) 12%, transparent);
    font-size: 12px;
  }
  .hint {
    margin: 0;
    color: var(--muted);
    line-height: 1.5;
  }
  .host-pick {
    display: block;
    margin-top: 6px;
    max-width: 260px;
  }
  .grid {
    display: grid;
    gap: 10px;
  }
  .grid.two {
    grid-template-columns: 1fr 1fr;
  }
  .grid.address {
    grid-template-columns: 1fr 110px;
  }
  label,
  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
    color: var(--muted);
    min-width: 0;
  }
  label input.field-control {
    padding: 5px 8px;
  }
  .segmented {
    align-self: flex-start;
  }
  .warning {
    padding: 6px 8px;
    color: var(--warning);
    border: 1px solid color-mix(in srgb, var(--warning) 40%, transparent);
    border-radius: var(--radius);
    line-height: 1.4;
  }
  .muted {
    color: var(--text);
    padding: 5px 0;
  }
  .colors {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .swatch {
    width: 18px;
    height: 18px;
    border-radius: 50%;
    border: 1px solid var(--border);
    color: var(--muted);
    font-size: 11px;
    line-height: 1;
  }
  .swatch.selected {
    border: 2px solid var(--text);
  }
  label.check {
    flex-direction: row;
    align-items: center;
    gap: 6px;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
    margin-top: 4px;
  }
</style>
