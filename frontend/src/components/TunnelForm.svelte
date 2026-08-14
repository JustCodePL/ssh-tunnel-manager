<script lang="ts">
  import type { TunnelConfig, PortForward } from "../types";
  import { PORTLESS_TLD } from "../types";
  import { addTunnel, updateTunnel, tunnels, includedConfigFiles } from "../stores/tunnels";
  import { preparePortForwardsForSave } from "../lib/portForwards";
  import { createEventDispatcher, onMount } from "svelte";
  import { EventsOn, EventsOff } from "../../wailsjs/runtime/runtime";

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
  let selectedConfigFile = tunnel?.sourceFile ?? "";
  let configSelectionKey = tunnel?.id ?? "__new";

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
  let hasIncludeOptions = false;
  let minimized = false;
  let pfSettingsIndex: number | null = null;

  let saving = false;
  let savingLabel = "// saving...";
  let error = "";

  onMount(() => {
    EventsOn("portless:setup-started", () => {
      savingLabel = "// waiting for admin permission...";
    });
    EventsOn("portless:setup-finished", () => {
      savingLabel = "// saving...";
    });
    return () => {
      EventsOff("portless:setup-started");
      EventsOff("portless:setup-finished");
    };
  });

  $: isEdit = tunnel !== null;
  $: hasIncludeOptions = $includedConfigFiles.length > 1;

  $: {
    const key = tunnel?.id ?? "__new";
    if (key !== configSelectionKey) {
      configSelectionKey = key;
      if (tunnel?.sourceFile) {
        selectedConfigFile = tunnel.sourceFile;
      } else {
        selectedConfigFile = $includedConfigFiles[0]?.path ?? "";
      }
    }
  }

  $: if (!selectedConfigFile && !tunnel && hasIncludeOptions) {
    selectedConfigFile = $includedConfigFiles[0]?.path ?? "";
  }

  $: existingGroups = [...new Set($tunnels.map(t => t.group).filter(Boolean))].sort();

  function addPortForward() {
    portForwards = [...portForwards, { localPort: 0, remoteHost: "127.0.0.1", remotePort: 0, description: "" }];
  }

  function removePortForward(index: number) {
    portForwards = portForwards.filter((_, i) => i !== index);
  }

  function slugify(s: string): string {
    return s
      .toLowerCase()
      .trim()
      .replace(/[^a-z0-9-]+/g, "-")
      .replace(/^-+|-+$/g, "");
  }

  // Other portless domains in the app, excluding the forward at the given
  // index of the current tunnel. Used for uniqueness checks and default
  // domain generation that avoids collisions.
  function otherDomains(skipIndex: number): Set<string> {
    const taken = new Set<string>();
    for (const t of $tunnels) {
      if (tunnel && t.id === tunnel.id) continue;
      for (const pf of t.portForwards ?? []) {
        if (pf.portless && pf.domain) taken.add(pf.domain.toLowerCase());
      }
    }
    portForwards.forEach((pf, i) => {
      if (i === skipIndex) return;
      if (pf.portless && pf.domain) taken.add(pf.domain.toLowerCase());
    });
    return taken;
  }

  function suggestDomain(index: number): string {
    const tunnelSlug = slugify(name) || "tunnel";
    const taken = otherDomains(index);
    if (!taken.has(tunnelSlug)) return tunnelSlug;
    const desc = portForwards[index]?.description;
    if (desc) {
      const candidate = `${slugify(desc)}.${tunnelSlug}`;
      if (!taken.has(candidate)) return candidate;
    }
    let n = 1;
    while (taken.has(`stm-${n}.${tunnelSlug}`)) n++;
    return `stm-${n}.${tunnelSlug}`;
  }

  function looksLikeFQDN(h: string | undefined): boolean {
    if (!h) return false;
    const v = h.trim();
    if (!v) return false;
    if (v.toLowerCase() === "localhost") return false;
    // IPv4 check
    if (/^\d+\.\d+\.\d+\.\d+$/.test(v)) return false;
    // IPv6 has colons
    if (v.includes(":")) return false;
    return v.includes(".");
  }

  function autoHostHeader(pf: PortForward): string {
    if (!pf.portless) return "";
    return looksLikeFQDN(pf.remoteHost) ? pf.remoteHost : "";
  }

  function toggleHostHeaderOn(index: number) {
    const pf = portForwards[index];
    pf.hostHeaderOn = !pf.hostHeaderOn;
    portForwards = [...portForwards];
  }

  function openPfSettings(index: number) {
    pfSettingsIndex = index;
  }

  function closePfSettings() {
    pfSettingsIndex = null;
  }

  function togglePortless(index: number) {
    const pf = portForwards[index];
    pf.portless = !pf.portless;
    if (pf.portless) {
      if (!pf.domain) pf.domain = suggestDomain(index);
      // Don't auto-fill the local port — for portless, empty means "use
      // remote port", which is the most common desired behaviour. User can
      // override (e.g. 80 so a browser doesn't need :8080 in the URL).
    }
    portForwards = [...portForwards];
  }

  function minimize() { minimized = true; }
  function restore() { minimized = false; }
  function handleClose() { dispatch("close"); }

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
    const validForwards = preparePortForwardsForSave(portForwards);

    const domainRe = /^[a-z0-9-]+(\.[a-z0-9-]+)*$/;
    const seenDomains = new Set<string>();
    for (const pf of validForwards) {
      if (!pf.portless) continue;
      const d = (pf.domain ?? "").trim().toLowerCase();
      if (!d) {
        error = "portless forward requires a domain name";
        return;
      }
      if (!domainRe.test(d)) {
        error = `invalid domain "${d}" — use letters, digits, dashes, dots only`;
        return;
      }
      if (seenDomains.has(d)) {
        error = `domain "${d}" is used twice in this tunnel`;
        return;
      }
      seenDomains.add(d);
      for (const t of $tunnels) {
        if (tunnel && t.id === tunnel.id) continue;
        for (const opf of t.portForwards ?? []) {
          if (opf.portless && (opf.domain ?? "").toLowerCase() === d) {
            error = `domain "${d}" is already used by tunnel "${t.name}"`;
            return;
          }
        }
      }
      pf.domain = d;
    }

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
        pinned: tunnel?.pinned,
      };
      if (hasIncludeOptions && selectedConfigFile) {
        cfg.sourceFile = selectedConfigFile;
      }
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

<div class="form-overlay" class:hidden={minimized}>
  <div class="form-window">
    <div class="form-titlebar">
      <span class="form-title">{isEdit ? `edit tunnel — ${tunnel?.name ?? ""}` : "new tunnel"}</span>
      <div class="titlebar-actions">
        <button class="title-btn minimize-btn" on:click={minimize} title="Minimize">−</button>
        <button class="close-btn" on:click={handleClose} title="Cancel">×</button>
      </div>
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
            list="group-options"
            autocomplete="off"
          />
          <datalist id="group-options">
            {#each existingGroups as g}
              <option value={g} />
            {/each}
          </datalist>
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

      {#if hasIncludeOptions}
        <div class="field">
          <label class="field-label">config file</label>
          <select class="field-input" bind:value={selectedConfigFile}>
            {#each $includedConfigFiles as file}
              <option value={file.path}>{file.label}</option>
            {/each}
          </select>
          <p class="hint">choose target config file for this host</p>
        </div>
      {/if}

      <div class="field">
        <div class="forwards-header">
          <label class="field-label">port forwards</label>
          <button type="button" class="link-btn" on:click={addPortForward}>+ add</button>
        </div>
        {#each portForwards as pf, i}
          <div class="forward-block">
            <div class="forward-block-head">
              <input
                type="text"
                bind:value={pf.description}
                class="field-input desc-input"
                placeholder="description (optional)"
              />
              <button
                type="button"
                class="gear-btn"
                class:active={pf.hostHeaderOn}
                on:click={() => openPfSettings(i)}
                title={pf.hostHeaderOn ? (pf.hostHeader ? `Host header: ${pf.hostHeader}` : "Host header rewrite: on") : "Forward settings"}
              >⚙</button>
              <button type="button" class="remove-btn" on:click={() => removePortForward(i)} title="remove">×</button>
            </div>
            <div class="forward-row">
              <input
                type="number"
                bind:value={pf.localPort}
                class="field-input port-input"
                placeholder="local"
                min="1"
                max="65535"
                title="local listen port on 127.0.0.1 (used when portless is off, and as fallback for plain ssh from a terminal)"
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
            </div>
            <div class="portless-row">
              <label class="portless-toggle" on:click|preventDefault={() => togglePortless(i)}>
                <span class="hacker-check small" class:checked={pf.portless}>
                  {#if pf.portless}<span class="check-mark">■</span>{:else}<span class="check-empty">□</span>{/if}
                </span>
                <span class="portless-text" class:active={pf.portless}>portless</span>
              </label>
              {#if pf.portless}
                <input
                  type="text"
                  bind:value={pf.domain}
                  class="field-input domain-input"
                  placeholder="domain"
                  autocomplete="off"
                  spellcheck="false"
                />
                <span class="tld-suffix">.{PORTLESS_TLD}</span>
                <span class="port-sep">:</span>
                <input
                  type="number"
                  bind:value={pf.exposePort}
                  class="field-input expose-input"
                  placeholder="auto"
                  min="0"
                  max="65535"
                  title="optional — empty/0 = use remote port. Set e.g. 80 so a browser can hit the domain without :port; low ports may prompt for platform authorization."
                />
              {/if}
            </div>
          </div>
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

      <label class="checkbox-label" on:click|preventDefault={() => (autoConnect = !autoConnect)}>
        <span class="hacker-check" class:checked={autoConnect}>
          {#if autoConnect}<span class="check-mark">■</span>{:else}<span class="check-empty">□</span>{/if}
        </span>
        <span class="checkbox-text" class:active={autoConnect}>auto-connect on startup</span>
      </label>
    </form>

    <div class="form-actions">
      <button class="cancel-btn" type="button" on:click={handleClose}>[ cancel ]</button>
      <button
        class="submit-btn"
        type="button"
        on:click={handleSubmit}
        disabled={saving}
      >
        {saving ? savingLabel : isEdit ? "[ update ]" : "[ add tunnel ]"}
      </button>
    </div>
  </div>
</div>

{#if pfSettingsIndex !== null && portForwards[pfSettingsIndex]}
  <div class="pf-modal-overlay" on:mousedown|self={closePfSettings}>
    <div class="pf-modal">
      <div class="pf-modal-head">
        <span class="pf-modal-title">forward settings</span>
        <button class="close-btn" type="button" on:click={closePfSettings} title="Close">×</button>
      </div>
      <div class="pf-modal-body">
        <div class="field">
          <div class="pf-modal-label-row">
            <label class="pf-modal-label">Host header override</label>
            <label class="pf-modal-toggle" on:click|preventDefault={() => pfSettingsIndex !== null && toggleHostHeaderOn(pfSettingsIndex)}>
              <span class="hacker-check small" class:checked={portForwards[pfSettingsIndex].hostHeaderOn}>
                {#if portForwards[pfSettingsIndex].hostHeaderOn}<span class="check-mark">■</span>{:else}<span class="check-empty">□</span>{/if}
              </span>
              <span class="pf-toggle-text" class:active={portForwards[pfSettingsIndex].hostHeaderOn}>{portForwards[pfSettingsIndex].hostHeaderOn ? "enabled" : "disabled"}</span>
            </label>
          </div>
          <input
            type="text"
            class="field-input pf-modal-input"
            placeholder={autoHostHeader(portForwards[pfSettingsIndex]) ? `auto: ${autoHostHeader(portForwards[pfSettingsIndex])}` : "(none — Host header passes through)"}
            bind:value={portForwards[pfSettingsIndex].hostHeader}
            disabled={!portForwards[pfSettingsIndex].hostHeaderOn}
            autocomplete="off"
            spellcheck="false"
          />
          {#if !portForwards[pfSettingsIndex].hostHeaderOn}
            <p class="pf-modal-hint">off (default) — raw TCP, browser's Host header passes through unchanged. Enable only if a remote reverse proxy needs it.</p>
          {:else if portForwards[pfSettingsIndex].portless}
            <p class="pf-modal-hint">
              {#if portForwards[pfSettingsIndex].hostHeader}
                upstream receives <span class="pf-modal-code">Host: {portForwards[pfSettingsIndex].hostHeader}</span>
              {:else if autoHostHeader(portForwards[pfSettingsIndex])}
                empty → auto-rewrites to <span class="pf-modal-code">{autoHostHeader(portForwards[pfSettingsIndex])}</span> (FQDN remote host)
              {:else}
                empty → no rewrite. Browser's <span class="pf-modal-code">{portForwards[pfSettingsIndex].domain || "<domain>"}.ssh-local</span> passes through.
              {/if}
            </p>
            <p class="pf-modal-hint">use when a remote reverse proxy (Traefik, nginx, Portainer) routes by Host header. HTTP only.</p>
          {:else}
            <p class="pf-modal-hint warning">host header only applies in portless mode</p>
          {/if}
        </div>
      </div>
      <div class="pf-modal-actions">
        <button type="button" class="cancel-btn" on:click={() => { if (pfSettingsIndex !== null) { portForwards[pfSettingsIndex].hostHeader = ""; portForwards = [...portForwards]; } }}>[ clear ]</button>
        <button type="button" class="submit-btn" on:click={closePfSettings}>[ done ]</button>
      </div>
    </div>
  </div>
{/if}

{#if minimized}
  <button class="restore-pill" on:click={restore} title="Restore tunnel form">
    <span class="pill-icon">▢</span>
    <span class="pill-text">{isEdit ? `edit — ${tunnel?.name ?? ""}` : "new tunnel"}</span>
    <span class="pill-close" on:click|stopPropagation={handleClose} title="Cancel">×</span>
  </button>
{/if}

<style>
  .form-overlay { position: fixed; top: 36px; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.85); z-index: 997; display: flex; align-items: center; justify-content: center; }
  .form-overlay.hidden { display: none; }
  .form-window { width: 90vw; max-width: 720px; height: 80vh; background: #0a0a0a; border: 1px solid var(--accent); border-radius: 2px; display: flex; flex-direction: column; min-height: 0; }
  .form-titlebar { display: flex; justify-content: space-between; align-items: center; padding: 6px 12px; border-bottom: 1px solid #1a1a1a; background: #111; flex-shrink: 0; }
  .form-title { font-family: 'JetBrains Mono', monospace; font-size: 11px; color: var(--accent); letter-spacing: 0.05em; }
  .titlebar-actions { display: flex; gap: 4px; align-items: center; }
  .title-btn, .close-btn { background: none; border: 1px solid #333; color: #888; font-size: 16px; width: 24px; height: 24px; cursor: pointer; border-radius: 2px; line-height: 1; padding: 0; }
  .minimize-btn:hover { color: var(--accent); border-color: var(--accent); }
  .close-btn:hover { color: #ff4444; border-color: #ff4444; }

  .error-bar {
    background: rgba(255, 68, 68, 0.1);
    border-bottom: 1px solid rgba(255, 68, 68, 0.3);
    color: #ff4444;
    font-size: 10px;
    padding: 6px 14px;
    font-family: 'JetBrains Mono', monospace;
    flex-shrink: 0;
  }

  .form-body {
    flex: 1;
    overflow-y: auto;
    min-height: 0;
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .form-grid { display: grid; gap: 8px; }
  .two-col { grid-template-columns: 1fr 1fr; }
  .three-col { grid-template-columns: 1fr 1fr 0.5fr; }
  .col-span-2 { grid-column: span 2; }

  .field { display: flex; flex-direction: column; gap: 4px; }
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
  .field-input:focus { border-color: var(--accent); }
  .field-input.mono { font-size: 10px; }

  .color-row { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
  .color-swatch {
    width: 20px;
    height: 20px;
    border-radius: 2px;
    border: 1px solid #333;
    cursor: pointer;
    transition: transform 0.1s, border-color 0.15s;
    padding: 0;
  }
  .color-swatch:hover { transform: scale(1.15); }
  .color-swatch.selected { border-color: #fff; border-width: 2px; }
  .none-swatch {
    background: var(--surface2);
    color: var(--muted);
    font-size: 11px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .forwards-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; }
  .forward-block {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 8px 10px;
    margin-bottom: 8px;
    background: rgba(255, 255, 255, 0.015);
    border: 1px solid var(--border);
    border-left: 2px solid var(--border);
    border-radius: 2px;
    transition: border-left-color 0.15s;
  }
  .forward-block:focus-within { border-left-color: var(--accent); }
  .forward-block-head {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .forward-block-head .desc-input { flex: 1; margin: 0; }
  .forward-row { display: flex; align-items: center; gap: 4px; }
  .port-input { width: 70px; flex-shrink: 0; }
  .arrow, .colon { color: var(--muted); font-size: 11px; flex-shrink: 0; }
  .remove-btn {
    background: transparent;
    border: none;
    color: var(--muted);
    cursor: pointer;
    font-size: 16px;
    padding: 0 4px;
    flex-shrink: 0;
    transition: color 0.15s;
    line-height: 1;
  }
  .remove-btn:hover { color: #ff4444; }
  .gear-btn {
    background: transparent;
    border: none;
    color: var(--muted);
    cursor: pointer;
    font-size: 13px;
    padding: 0 4px;
    flex-shrink: 0;
    transition: color 0.15s;
    line-height: 1;
  }
  .gear-btn:hover { color: var(--accent); }
  .gear-btn.active { color: var(--accent); text-shadow: 0 0 4px var(--accent); }
  .desc-input { font-size: 10px; color: var(--muted); }

  .port-input.dimmed { opacity: 0.4; }

  .portless-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .portless-toggle {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    cursor: pointer;
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    user-select: none;
    color: var(--muted);
  }
  .portless-toggle:hover .check-empty { color: var(--text); }
  .portless-toggle:hover .portless-text:not(.active) { color: var(--text); }
  .portless-text { transition: color 0.15s; }
  .portless-text.active { color: var(--accent); }
  .hacker-check.small { width: 13px; height: 13px; font-size: 11px; }
  .domain-input { font-size: 10px; flex: 1; min-width: 80px; }
  .expose-input { font-size: 10px; width: 60px; flex-shrink: 0; }
  .tld-suffix, .port-sep {
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    color: var(--muted);
    white-space: nowrap;
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
  .link-btn:hover { color: var(--accent2); }

  .advanced-fields {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-top: 8px;
    padding-left: 10px;
    border-left: 1px solid var(--border);
  }

  .hint { font-size: 9px; color: #333; margin: 0; font-family: 'JetBrains Mono', monospace; }

  .checkbox-label {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    font-size: 11px;
    color: var(--text);
    font-family: 'JetBrains Mono', monospace;
    user-select: none;
    padding: 4px 0;
  }

  .hacker-check {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 16px;
    height: 16px;
    font-size: 14px;
    line-height: 1;
    transition: all 0.15s;
  }

  .check-empty { color: var(--muted); }
  .check-mark { color: var(--accent); text-shadow: 0 0 6px var(--accent); }
  .checkbox-text { color: var(--muted); transition: color 0.15s; }
  .checkbox-text.active { color: var(--accent); }
  .checkbox-label:hover .check-empty { color: var(--text); }
  .checkbox-label:hover .checkbox-text:not(.active) { color: var(--text); }

  .form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
    padding: 8px 14px;
    border-top: 1px solid var(--border);
    background: #0d0d0d;
    flex-shrink: 0;
  }

  .cancel-btn {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--muted);
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    padding: 5px 12px;
    cursor: pointer;
    border-radius: 2px;
    letter-spacing: 0.05em;
  }
  .cancel-btn:hover { color: var(--text); border-color: var(--muted); }

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
  .submit-btn:hover:not(:disabled) { background: rgba(0, 255, 136, 0.1); }
  .submit-btn:disabled { opacity: 0.5; cursor: default; }

  .pf-modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.7);
    z-index: 1100;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .pf-modal {
    width: 90vw;
    max-width: 420px;
    background: #0a0a0a;
    border: 1px solid var(--accent);
    border-radius: 2px;
    display: flex;
    flex-direction: column;
  }
  .pf-modal-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 6px 12px;
    border-bottom: 1px solid #1a1a1a;
    background: #111;
  }
  .pf-modal-title {
    font-family: 'JetBrains Mono', monospace;
    font-size: 11px;
    color: var(--accent);
    letter-spacing: 0.05em;
  }
  .pf-modal-body {
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .pf-modal-actions {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
    padding: 8px 14px;
    border-top: 1px solid var(--border);
    background: #0d0d0d;
  }
  .pf-modal-label-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 4px;
  }
  .pf-modal-label {
    font-size: 10px;
    font-family: 'JetBrains Mono', monospace;
    color: var(--text);
    letter-spacing: 0.05em;
  }
  .pf-modal-toggle {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    cursor: pointer;
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    user-select: none;
    color: var(--muted);
  }
  .pf-modal-toggle:hover .check-empty { color: var(--text); }
  .pf-modal-toggle:hover .pf-toggle-text:not(.active) { color: var(--text); }
  .pf-toggle-text { transition: color 0.15s; }
  .pf-toggle-text.active { color: var(--text); }
  .pf-modal-input { color: var(--text); }
  .pf-modal-input:disabled { opacity: 0.4; cursor: not-allowed; }
  .pf-modal-hint {
    font-size: 10px;
    color: var(--muted);
    margin: 6px 0 0;
    font-family: 'JetBrains Mono', monospace;
    line-height: 1.4;
  }
  .pf-modal-code {
    font-family: 'JetBrains Mono', monospace;
    color: var(--text);
    background: var(--surface2);
    border: 1px solid var(--border);
    padding: 0 4px;
    border-radius: 2px;
  }
  .pf-modal-hint.warning { color: #f97316; }

  .restore-pill {
    position: fixed;
    bottom: 12px;
    right: 636px;
    z-index: 1000;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px;
    background: #0a0a0a;
    border: 1px solid var(--accent);
    border-radius: 2px;
    color: var(--accent);
    font-family: 'JetBrains Mono', monospace;
    font-size: 11px;
    cursor: pointer;
    box-shadow: 0 0 12px rgba(0, 255, 136, 0.3);
  }
  .restore-pill:hover { background: #111; box-shadow: 0 0 16px rgba(0, 255, 136, 0.5); }
  .pill-icon { font-size: 13px; }
  .pill-text { max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pill-close { color: #888; padding: 0 4px; font-size: 14px; line-height: 1; }
  .pill-close:hover { color: #ff4444; }
</style>
