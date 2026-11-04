<script lang="ts">
  import type { TunnelConfig, PortForward } from "../types";
  import { PORTLESS_TLD } from "../types";
  import { addTunnel, updateTunnel, tunnels, groups, includedConfigFiles } from "../stores/tunnels";
  import Select from "./Select.svelte";
  import Combobox from "./Combobox.svelte";
  import { domainWithSuffix, domainWithoutSuffix, effectiveForwardProtocol, normalizePortlessSuffix, preparePortForwardsForSave } from "../lib/portForwards";
  import { createEventDispatcher, onMount } from "svelte";
  import { createEventScope } from "../lib/runtime";
  const events = createEventScope();

  export let tunnel: TunnelConfig | null = null;
  // Inside the settings window the form has no overlay and shows one section.
  export let embedded = false;
  export let section: "all" | "connection" | "forwards" = "all";
  // The group a new tunnel starts in.
  export let initialGroup = "";
  // Inside the settings window these are edited on the Credentials page.
  export let user = tunnel?.user ?? "";
  export let keyPath = tunnel?.keyPath ?? "";
  export let rdpPort = tunnel?.rdpPort ? String(tunnel.rdpPort) : "";
  export let pinned = tunnel?.pinned ?? false;

  const dispatch = createEventDispatcher<{ close: void }>();

  let name = tunnel?.name ?? "";
  let host = tunnel?.host ?? "";
  let port = tunnel?.port ?? 22;
  let group = tunnel?.group ?? initialGroup;
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
  let portlessSuffix = tunnel?.portlessSuffix ?? "";
  // While editing, a forward's domain is only its own part. The suffix joins it
  // when saving, so changing the suffix moves every forward with it.
  let portForwards: PortForward[] = (tunnel?.portForwards ?? []).map((pf) => ({
    ...pf,
    domain: domainWithoutSuffix(pf.domain, tunnel?.portlessSuffix ?? ""),
  }));
  $: activeSuffix = normalizePortlessSuffix(portlessSuffix);
  let showAdvanced = !!(tunnel?.proxyCommand || tunnel?.proxyJump || tunnel?.rdpPort);
  let hasIncludeOptions = false;
  let minimized = false;
  let pfSettingsIndex: number | null = null;

  let saving = false;
  let savingLabel = "Saving...";
  let error = "";

  onMount(() => {
    events.on("portless:setup-started", () => {
      savingLabel = "Waiting for Portless system setup...";
    });
    events.on("portless:setup-finished", () => {
      savingLabel = "Saving...";
    });
    return () => {
      events.off();
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

  $: existingGroups = [...new Set([...$tunnels.map(t => t.group), ...$groups].filter(Boolean))].sort();

  function addPortForward() {
    portForwards = [...portForwards, { localPort: 0, remoteHost: "127.0.0.1", remotePort: 0, description: "" }];
  }

  const FORWARD_PROTOCOLS: { value: "rdp" | "ssh" | "none"; label: string }[] = [
    { value: "rdp", label: "rdp" },
    { value: "ssh", label: "ssh" },
    { value: "none", label: "undefined" },
  ];

  function setForwardProtocol(index: number, protocol: "rdp" | "ssh" | "none") {
    portForwards[index].protocol = protocol;
    portForwards = portForwards;
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
      if (pf.portless && pf.domain) taken.add(domainWithSuffix(pf.domain, activeSuffix));
    });
    return taken;
  }

  function suggestDomain(index: number): string {
    const tunnelSlug = slugify(name) || "tunnel";
    const taken = otherDomains(index);
    const isTaken = (candidate: string) => taken.has(domainWithSuffix(candidate, activeSuffix));
    if (!isTaken(tunnelSlug)) return tunnelSlug;
    const desc = portForwards[index]?.description;
    if (desc) {
      const candidate = `${slugify(desc)}.${tunnelSlug}`;
      if (!isTaken(candidate)) return candidate;
    }
    let n = 1;
    while (isTaken(`stm-${n}.${tunnelSlug}`)) n++;
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

  // Validates the form and returns what to save, or null with `error` set.
  function prepare(): TunnelConfig | null {
    error = "";
    const trimmedName = name.trim();
    if (!trimmedName) {
      error = "name is required";
      return null;
    }
    if (/\s/.test(trimmedName)) {
      error = "name cannot contain spaces (used as SSH host alias)";
      return null;
    }
    const validForwards = preparePortForwardsForSave(portForwards);

    const domainRe = /^[a-z0-9-]+(\.[a-z0-9-]+)*$/;
    if (activeSuffix && !domainRe.test(activeSuffix)) {
      error = `invalid portless suffix "${activeSuffix}" — use letters, digits, dashes, dots only`;
      return null;
    }
    const seenDomains = new Set<string>();
    for (const pf of validForwards) {
      if (!pf.portless) continue;
      const d = domainWithSuffix(pf.domain, activeSuffix);
      if (!d) {
        error = "portless forward requires a domain name";
        return null;
      }
      if (!domainRe.test(d)) {
        error = `invalid domain "${d}" — use letters, digits, dashes, dots only`;
        return null;
      }
      if (seenDomains.has(d)) {
        error = `domain "${d}" is used twice in this tunnel`;
        return null;
      }
      seenDomains.add(d);
      for (const t of $tunnels) {
        if (tunnel && t.id === tunnel.id) continue;
        for (const opf of t.portForwards ?? []) {
          if (opf.portless && (opf.domain ?? "").toLowerCase() === d) {
            error = `domain "${d}" is already used by tunnel "${t.name}"`;
            return null;
          }
        }
      }
      pf.domain = d;
    }

    const rdpPortText = rdpPort.trim();
    const rdpPortValue = rdpPortText === "" ? 0 : Number(rdpPortText);
    if (!Number.isInteger(rdpPortValue) || rdpPortValue < 0 || rdpPortValue > 65535 || rdpPortText === "0") {
      error = "rdp port must be between 1 and 65535";
      return null;
    }

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
      pinned: tunnel ? pinned : undefined,
      rdpPort: rdpPortValue || undefined,
      portlessSuffix: activeSuffix || undefined,
    };
    if (hasIncludeOptions && selectedConfigFile) {
      cfg.sourceFile = selectedConfigFile;
    }
    return cfg;
  }

  // What saving would change, ignoring the pin, which has its own call. Updating
  // a tunnel reconnects it, so a save without changes must not reach the backend.
  function fingerprint(cfg: TunnelConfig): string {
    return JSON.stringify({ ...cfg, pinned: undefined });
  }

  let baseline = "";
  onMount(() => {
    const cfg = tunnel ? prepare() : null;
    error = "";
    baseline = cfg ? fingerprint(cfg) : "";
  });

  /** Reports a problem with the form, without saving. */
  export function validate(): string {
    prepare();
    return error;
  }

  /** Saves the tunnel if it changed, and returns the problem if there is one. */
  export async function save(): Promise<string> {
    const cfg = prepare();
    if (!cfg) return error;
    const current = fingerprint(cfg);
    if (isEdit && current === baseline) return "";
    saving = true;
    try {
      if (isEdit) {
        await updateTunnel(cfg);
      } else {
        await addTunnel(cfg);
      }
      baseline = current;
      return "";
    } catch (e: any) {
      error = e?.message ?? String(e);
      return error;
    } finally {
      saving = false;
    }
  }

  async function handleSubmit() {
    if (!(await save())) dispatch("close");
  }
</script>

<div class="form-overlay" class:embedded class:hidden={minimized} data-testid="tunnel-form">
  <div class="form-window">
    {#if !embedded}
    <div class="form-titlebar">
      <span class="form-title">{isEdit ? `edit tunnel — ${tunnel?.name ?? ""}` : "new tunnel"}</span>
      <div class="titlebar-actions">
        <button class="title-btn minimize-btn" on:click={minimize} title="Minimize">−</button>
        <button class="close-btn" on:click={handleClose} title="Cancel">×</button>
      </div>
    </div>
    {/if}

    {#if error}
      <div class="error-bar">! {error}</div>
    {/if}

    <form on:submit|preventDefault={handleSubmit} class="form-body" data-section={section}>
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
          <Combobox bind:value={group} options={existingGroups} placeholder="Choose or type a group" testid="group-input" />
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

      {#if !embedded}
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
      {/if}

      {#if hasIncludeOptions}
        <div class="field">
          <label class="field-label">config file</label>
          <Select bind:value={selectedConfigFile} ariaLabel="Config file" options={$includedConfigFiles.map((file) => ({ value: file.path, label: file.label }))} />
          <p class="hint">choose target config file for this host</p>
        </div>
      {/if}

      <div class="field forwards-field">
        {#if !embedded}<label class="field-label forwards-label">port forwards</label>{/if}
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
            <div class="protocol-row">
              <span class="protocol-label">type</span>
              {#each FORWARD_PROTOCOLS as option}
                <label class="protocol-option">
                  <input
                    type="radio"
                    name="forward-protocol-{i}"
                    value={option.value}
                    checked={effectiveForwardProtocol(pf) === option.value}
                    on:change={() => setForwardProtocol(i, option.value)}
                  />
                  {option.label}
                </label>
              {/each}
              {#if effectiveForwardProtocol(pf) === "ssh"}
                <input
                  type="text"
                  bind:value={pf.sshUser}
                  class="field-input ssh-user-input"
                  placeholder={user || "ssh user"}
                  autocomplete="off"
                  spellcheck="false"
                  title="user on the target machine. leave empty to use this tunnel's user"
                />
              {/if}
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
                  data-testid="forward-domain"
                  placeholder="domain"
                  autocomplete="off"
                  spellcheck="false"
                />
                <span class="tld-suffix" data-testid="domain-suffix">.{activeSuffix ? `${activeSuffix}.` : ""}{PORTLESS_TLD}</span>
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
        <button type="button" class="add-forward-btn" on:click={addPortForward}>
          + add port forwarding
        </button>
      </div>

      <div class="field">
        {#if !embedded}
        <button
          type="button"
          class="link-btn"
          on:click={() => (showAdvanced = !showAdvanced)}
        >
          {showAdvanced ? "▾" : "▸"} advanced
        </button>
        {/if}
        {#if showAdvanced || embedded}
          <div class="advanced-fields" class:plain={embedded}>
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
                placeholder="jump.example.com"
              />
            </div>
            <p class="hint">tokens: %h (host) %p (port) %r (user). use one of proxycommand or proxyjump.</p>
            <div class="field">
              <label class="field-label">portless suffix</label>
              <input
                type="text"
                bind:value={portlessSuffix}
                class="field-input"
                data-testid="portless-suffix"
                placeholder="dev"
                autocomplete="off"
                spellcheck="false"
              />
            </div>
            <p class="hint">
              {#if activeSuffix}forwards use name.{activeSuffix}.{PORTLESS_TLD}{:else}shared end of this tunnel's portless domains, e.g. dev gives name.dev.{PORTLESS_TLD}. leave empty for name.{PORTLESS_TLD}.{/if}
            </p>
            {#if !embedded}
            <div class="field">
              <label class="field-label">rdp port</label>
              <input
                type="text"
                inputmode="numeric"
                bind:value={rdpPort}
                class="field-input"
                placeholder="3389"
              />
            </div>
            <p class="hint">only this port is checked for remote desktop. leave empty for the default, 3389.</p>
            {/if}
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

    {#if !embedded}
    <div class="form-actions">
      <button class="cancel-btn" type="button" on:click={handleClose}>Cancel</button>
      <button
        class="submit-btn"
        type="button"
        on:click={handleSubmit}
        disabled={saving}
      >
        {saving ? savingLabel : isEdit ? "Update" : "Add tunnel"}
      </button>
    </div>
    {/if}
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
        <button type="button" class="cancel-btn" on:click={() => { if (pfSettingsIndex !== null) { portForwards[pfSettingsIndex].hostHeader = ""; portForwards = [...portForwards]; } }}>Clear</button>
        <button type="button" class="submit-btn" on:click={closePfSettings}>Done</button>
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
  .form-overlay.embedded { position: static; background: none; padding: 0; display: block; z-index: auto; }
  .form-overlay.embedded .form-window { width: auto; max-width: none; max-height: none; border: 0; box-shadow: none; background: none; }
  .form-overlay.embedded .form-window { height: auto; }
  .form-overlay.embedded .form-body { flex: none; overflow: visible; padding: 0; }
  .advanced-fields.plain { padding-left: 0; border-left: 0; margin-top: 0; }
  .form-body[data-section="connection"] > .forwards-field { display: none; }
  .form-body[data-section="forwards"] > :not(.forwards-field):not(.form-actions) { display: none; }

  .form-overlay { position: fixed; top: var(--titlebar-height); left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.85); z-index: 997; display: flex; align-items: center; justify-content: center; }
  .form-overlay.hidden { display: none; }
  .form-window { width: 90vw; max-width: 720px; height: 80vh; background: var(--bg); border: 1px solid var(--accent); border-radius: 2px; display: flex; flex-direction: column; min-height: 0; }
  .form-titlebar { display: flex; justify-content: space-between; align-items: center; padding: 6px 12px; border-bottom: 1px solid var(--border); background: var(--surface); flex-shrink: 0; }
  .form-title { font-family: var(--ui); font-size: 12px; color: var(--accent); letter-spacing: 0.05em; }
  .titlebar-actions { display: flex; gap: 4px; align-items: center; }
  .title-btn, .close-btn { background: none; border: 1px solid var(--border); color: var(--muted); font-size: 16px; width: 24px; height: 24px; cursor: pointer; border-radius: 2px; line-height: 1; padding: 0; }
  .minimize-btn:hover { color: var(--accent); border-color: var(--accent); }
  .close-btn:hover { color: var(--error); border-color: var(--error); }

  .error-bar {
    background: color-mix(in srgb, var(--error) 14%, transparent);
    border-bottom: 1px solid color-mix(in srgb, var(--error) 14%, transparent);
    color: var(--error);
    font-size: 11.5px;
    padding: 6px 14px;
    font-family: var(--ui);
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
    font-size: 11.5px;
    font-family: var(--ui);
    color: var(--accent);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  .field-input {
    background: var(--surface2);
    border: 1px solid var(--border);
    border-radius: 2px;
    color: var(--text);
    font-family: var(--ui);
    font-size: 12px;
    padding: 5px 8px;
    outline: none;
    transition: border-color 0.15s;
    width: 100%;
  }
  .field-input:focus { border-color: var(--accent); }
  .field-input.mono { font-size: 11.5px; }

  .color-row { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
  .color-swatch {
    width: 20px;
    height: 20px;
    border-radius: 2px;
    border: 1px solid var(--border);
    cursor: pointer;
    transition: transform 0.1s, border-color 0.15s;
    padding: 0;
  }
  .color-swatch:hover { transform: scale(1.15); }
  .color-swatch.selected { border-color: var(--text); border-width: 2px; }
  .none-swatch {
    background: var(--surface2);
    color: var(--muted);
    font-size: 12px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .forwards-label { margin-bottom: 2px; }
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
  .arrow, .colon { color: var(--muted); font-size: 12px; flex-shrink: 0; }
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
  .remove-btn:hover { color: var(--error); }
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
  .desc-input { font-size: 11.5px; color: var(--muted); }

  .add-forward-btn {
    width: 100%;
    background: transparent;
    border: 1px solid var(--accent);
    border-radius: 2px;
    color: var(--accent);
    font-family: var(--ui);
    font-size: 12px;
    letter-spacing: 0.04em;
    padding: 8px 12px;
    cursor: pointer;
    transition: background 0.15s;
  }
  .add-forward-btn:hover {
    background: var(--accent-soft);
  }
  .add-forward-btn:focus-visible {
    outline: 1px solid var(--accent);
    outline-offset: 2px;
  }

  .port-input.dimmed { opacity: 0.4; }

  .protocol-row {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 6px;
    font-size: 12px;
  }
  .protocol-label { color: var(--muted); }
  .protocol-option { display: flex; align-items: center; gap: 4px; color: var(--muted); cursor: pointer; }
  .protocol-option input { accent-color: var(--accent); margin: 0; }
  .ssh-user-input { flex: 1; min-width: 80px; padding: 3px 6px; font-size: 12px; }
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
    font-family: var(--ui);
    font-size: 11.5px;
    user-select: none;
    color: var(--muted);
  }
  .portless-toggle:hover .check-empty { color: var(--text); }
  .portless-toggle:hover .portless-text:not(.active) { color: var(--text); }
  .portless-text { transition: color 0.15s; }
  .portless-text.active { color: var(--accent); }
  .hacker-check.small { width: 13px; height: 13px; font-size: 12px; }
  .domain-input { font-size: 11.5px; flex: 1; min-width: 80px; }
  .expose-input { font-size: 11.5px; width: 60px; flex-shrink: 0; }
  .tld-suffix, .port-sep {
    font-family: var(--ui);
    font-size: 11.5px;
    color: var(--muted);
    white-space: nowrap;
  }

  .link-btn {
    background: transparent;
    border: none;
    color: var(--muted);
    font-family: var(--ui);
    font-size: 11.5px;
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

  .hint { font-size: 11.5px; color: var(--muted); margin: 0; font-family: var(--ui); }

  .checkbox-label {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    font-size: 12px;
    color: var(--text);
    font-family: var(--ui);
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
    background: var(--surface);
    flex-shrink: 0;
  }

  .cancel-btn {
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
  .cancel-btn:hover { color: var(--text); border-color: var(--muted); }

  .submit-btn {
    background: transparent;
    border: 1px solid var(--accent);
    color: var(--accent);
    font-family: var(--ui);
    font-size: 11.5px;
    padding: 5px 14px;
    cursor: pointer;
    border-radius: 2px;
    transition: background 0.15s;
    letter-spacing: 0.05em;
  }
  .submit-btn:hover:not(:disabled) { background: var(--accent-soft); }
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
    background: var(--bg);
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
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .pf-modal-title {
    font-family: var(--ui);
    font-size: 12px;
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
    background: var(--surface);
  }
  .pf-modal-label-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 4px;
  }
  .pf-modal-label {
    font-size: 11.5px;
    font-family: var(--ui);
    color: var(--text);
    letter-spacing: 0.05em;
  }
  .pf-modal-toggle {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    cursor: pointer;
    font-family: var(--ui);
    font-size: 11.5px;
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
    font-size: 11.5px;
    color: var(--muted);
    margin: 6px 0 0;
    font-family: var(--ui);
    line-height: 1.4;
  }
  .pf-modal-code {
    font-family: var(--ui);
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
    background: var(--bg);
    border: 1px solid var(--accent);
    border-radius: 2px;
    color: var(--accent);
    font-family: var(--ui);
    font-size: 12px;
    cursor: pointer;
    box-shadow: 0 0 12px var(--accent-soft);
  }
  .restore-pill:hover { background: var(--surface); box-shadow: 0 0 16px var(--accent-soft); }
  .pill-icon { font-size: 13px; }
  .pill-text { max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pill-close { color: var(--muted); padding: 0 4px; font-size: 14px; line-height: 1; }
  .pill-close:hover { color: var(--error); }
</style>
