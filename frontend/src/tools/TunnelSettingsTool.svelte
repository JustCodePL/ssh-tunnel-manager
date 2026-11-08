<script lang="ts">
  import { EventsOn, errorText } from "../lib/runtime";
  import { createEventDispatcher, onDestroy, onMount } from "svelte";
  import {
    ApplyRowToGroup,
    BindCredential,
    ForgetSSHHostKeys,
    GetSSHHostKeys,
    GetCredentialBinding,
    GetTunnelSettings,
    ListCredentials,
    SetTunnelSettings,
  } from "../../bindings/ssh-tunnel-manager/app";
  import type { TunnelConfig } from "../types";
  import { appSettings, effective, settingsFor } from "../stores/settings";
  import { deleteTunnel, setTunnelPinned } from "../stores/tunnels";
  import { capabilities } from "../stores/capabilities";
  import { REFRESH_CHOICES, formatInterval } from "../lib/toolConnection.js";
  import { effectiveForwardProtocol } from "../lib/portForwards.js";
  import { openTool } from "../lib/openTool";
  import { showToast } from "../stores/toast";
  import { askConfirm } from "../stores/confirm";
  import TunnelForm from "../components/TunnelForm.svelte";
  import Select from "../components/Select.svelte";
  import RowLayoutEditor from "./RowLayoutEditor.svelte";

  export let tunnel: TunnelConfig;

  const dispatch = createEventDispatcher<{ close: void }>();
  const MONITORS = [
    { key: "stats", label: "CPU and RAM" },
    { key: "htop", label: "htop" },
    { key: "disk", label: "Disk" },
    { key: "docker", label: "Docker" },
  ];
  const pages = [
    { id: "connection", label: "Connection" },
    { id: "forwards", label: "Forwards" },
    { id: "credentials", label: "Credentials" },
    { id: "row", label: "Row display" },
    { id: "monitoring", label: "Monitoring" },
    { id: "windows", label: "Windows" },
  ];

  let page = "connection";
  let credentialsView = "all";
  let form: TunnelForm;
  let saving = false;

  // Everything here is a draft until Save.
  let overrides: { afterDisconnect?: string; refresh?: Record<string, number>; row?: Record<string, string>; clipboard?: string; rdp?: string } = {};
  let logins: { id: string; name: string; username: string }[] = [];
  let loginMethod = "key";
  let sshLogin = "";
  let savedSSHLogin = "";
  let hostKeys: { address: string; fingerprint: string }[] = [];
  let bindings: Record<string, string> = {};
  let savedBindings: Record<string, string> = {};
  let user = tunnel.user ?? "";
  let keyPath = tunnel.keyPath ?? "";
  let rdpPort = tunnel.rdpPort ? String(tunnel.rdpPort) : "";
  let pinned = tunnel.pinned ?? false;

  $: settings = settingsFor($effective, tunnel.id);
  $: detected = !!$capabilities[tunnel.id]?.rdp;
  // Detection decides until the user turns it on or off here.
  $: rdpOn = overrides.rdp ? overrides.rdp === "on" : detected;
  $: forwardTargets = (tunnel.portForwards ?? [])
    .filter((pf) => effectiveForwardProtocol(pf) === "rdp")
    .map((pf) => ({
      key: `${pf.remoteHost}:${pf.remotePort}`,
      label: pf.description || `${pf.remoteHost}:${pf.remotePort}`,
      remoteHost: pf.remoteHost,
      remotePort: pf.remotePort,
    }));
  $: hostTarget = { key: "", label: "this host", remoteHost: "", remotePort: 0 };
  $: rdpTargets = [...(rdpOn ? [hostTarget] : []), ...forwardTargets];
  $: loginOptions = logins.map((login) => ({ value: login.id, label: login.name }));
  $: chosenLogin = logins.find((login) => login.id === sshLogin);

  let offSettings: (() => void) | undefined;

  async function loadLogins() {
    logins = ((await ListCredentials()) ?? []) as any[];
  }

  onMount(async () => {
    offSettings = EventsOn("settings:changed", () => void loadLogins()) as any;
    overrides = ((await GetTunnelSettings(tunnel.id)) ?? {}) as any;
    await loadLogins();
    await loadBindings();
    savedSSHLogin = sshLogin = await GetCredentialBinding("ssh", tunnel.id, {} as any);
    loginMethod = sshLogin ? "login" : "key";
    hostKeys = ((await GetSSHHostKeys(tunnel.id)) ?? []) as any;
  });

  onDestroy(() => offSettings?.());

  async function loadBindings() {
    const next: Record<string, string> = {};
    for (const target of [hostTarget, ...forwardTargets]) {
      next[target.key] = await GetCredentialBinding("rdp", tunnel.id, { remoteHost: target.remoteHost, remotePort: target.remotePort } as any).catch(() => "");
    }
    bindings = next;
    savedBindings = { ...next };
  }

  async function forgetHostKey() {
    try {
      await ForgetSSHHostKeys(tunnel.id);
      hostKeys = [];
    } catch (e: any) {
      showToast(errorText(e), "error");
    }
  }

  function setRefresh(key: string, value: string) {
    const refresh = { ...(overrides.refresh ?? {}) };
    if (value === "") delete refresh[key];
    else refresh[key] = Number(value);
    overrides = { ...overrides, refresh };
  }

  async function saveAll() {
    const problem = form.validate();
    if (problem) {
      showToast(problem, "error");
      return;
    }
    if (loginMethod === "login" && !sshLogin) {
      page = "credentials";
      showToast("Choose a saved login, or use a key file.", "error");
      return;
    }
    saving = true;
    try {
      await SetTunnelSettings(tunnel.id, overrides as any);
      for (const target of [hostTarget, ...forwardTargets]) {
        const id = bindings[target.key] ?? "";
        if (id !== (savedBindings[target.key] ?? "")) {
          await BindCredential("rdp", tunnel.id, { remoteHost: target.remoteHost, remotePort: target.remotePort } as any, id);
        }
      }
      savedBindings = { ...bindings };
      const login = loginMethod === "login" ? sshLogin : "";
      if (login !== savedSSHLogin) {
        await BindCredential("ssh", tunnel.id, {} as any, login);
        savedSSHLogin = login;
      }
      if (pinned !== (tunnel.pinned ?? false)) await setTunnelPinned(tunnel.id, pinned);
      // Last, because a rename moves all of the above to the tunnel's new ID.
      const failed = await form.save();
      if (failed) showToast(failed, "error");
      else showToast("Saved", "success");
    } catch (e: any) {
      showToast(errorText(e), "error");
    } finally {
      saving = false;
    }
  }

  async function applyToGroup() {
    try {
      await SetTunnelSettings(tunnel.id, overrides as any);
      const count = await ApplyRowToGroup(tunnel.id);
      showToast(`Copied to ${count} tunnel${count === 1 ? "" : "s"} in ${tunnel.group}`, "success");
    } catch (e: any) {
      showToast(errorText(e), "error");
    }
  }

  async function remove() {
    const accepted = await askConfirm({ title: "Delete tunnel", message: `Delete tunnel "${tunnel.name}"?`, confirmLabel: "Delete", danger: true });
    if (!accepted) return;
    await deleteTunnel(tunnel.id);
    dispatch("close");
  }
</script>

<div class="settings" data-testid="tunnel-settings-window">
  <nav aria-label="Tunnel settings sections">
    {#each pages as item}
      <button type="button" class:on={page === item.id} data-testid={`tunnel-${item.id}`} on:click={() => (page = item.id)}>{item.label}</button>
    {/each}
  </nav>

  <div class="main">
    <div class="page">
      <!-- The form stays mounted so edits survive switching between pages. -->
      <div class:hidden={page !== "connection" && page !== "forwards"}>
        <h2>{page === "forwards" ? "Forwards" : "Connection"}</h2>
        <p class="lead">
          {page === "forwards"
            ? "Attach follows a forward's type. Portless gives it a local domain instead of a local port."
            : "Saved in the SSH config file."}
        </p>
        <TunnelForm bind:this={form} {tunnel} embedded section={page === "forwards" ? "forwards" : "connection"} bind:user bind:keyPath bind:rdpPort bind:pinned />
        {#if page === "connection"}
          <label class="check pin"><input type="checkbox" data-testid="tunnel-pinned" bind:checked={pinned} /> Pinned at the top of the list</label>
        {/if}
      </div>

      {#if page === "credentials"}
        <h2>Credentials</h2>
        <p class="lead">How this tunnel logs in, and what it does with remote desktops.</p>

        <div class="credentials">
          <nav class="sub-nav" aria-label="Filter credentials">
            {#each [["all", "All"], ["ssh", "SSH"], ["rdp", "RDP"]] as [id, label]}
              <button type="button" class:on={credentialsView === id} data-testid={`credentials-${id}`} on:click={() => (credentialsView = id)}>{label}</button>
            {/each}
          </nav>
          <div class="sections">
            {#if credentialsView !== "rdp"}
              <section class="section" data-testid="credentials-ssh-section">
                <h3>SSH</h3>
                <div class="field">
                  <span>Login method</span>
                  <span class="segmented" data-testid="login-method">
                    <button type="button" class:on={loginMethod === "key"} data-testid="login-method-key" on:click={() => (loginMethod = "key")}>Key file</button>
                    <button type="button" class:on={loginMethod === "login"} data-testid="login-method-login" on:click={() => (loginMethod = "login")}>Credentials</button>
                  </span>
                </div>
                {#if loginMethod === "key"}
                  <div class="field">
                    <span>User</span>
                    <input class="field-control" data-testid="tunnel-user" bind:value={user} placeholder="deploy" autocomplete="off" spellcheck="false" />
                  </div>
                  <div class="field">
                    <span>Key file</span>
                    <input class="field-control" data-testid="tunnel-key" bind:value={keyPath} placeholder="~/.ssh/id_ed25519" autocomplete="off" spellcheck="false" />
                  </div>
                  <p class="note">Empty: the SSH agent and the default keys are tried.</p>
                {:else}
                  <div class="field">
                    <span>Credentials</span>
                    <Select bind:value={sshLogin} testid="ssh-login" ariaLabel="SSH credentials" placeholder="Choose credentials" options={loginOptions} />
                  </div>
                  {#if chosenLogin?.username}<div class="field"><span>User</span><code data-testid="ssh-login-user">{chosenLogin.username}</code></div>{/if}
                  {#if !logins.length}
                    <p class="note">No saved credentials yet.</p>
                    <button type="button" class="btn" on:click={() => void openTool({ tool: "settings", tunnelId: "" })}>Open Settings</button>
                  {/if}
                  <p class="note">A key and the agent are still tried first. The password stays in the system keychain and goes only to a server whose host key you trusted or that is in known_hosts. It applies from the next connection.</p>
                {/if}
                {#if hostKeys.length}
                  {#each hostKeys as key}
                    <div class="field">
                      <span title={key.address}>{key.address}</span>
                      <code data-testid="ssh-host-key">{key.fingerprint}</code>
                    </div>
                  {/each}
                  <div class="actions">
                    <button type="button" class="btn" data-testid="forget-host-key" on:click={forgetHostKey}>Forget host keys</button>
                  </div>
                {/if}
              </section>
            {/if}

            {#if credentialsView !== "ssh"}
              <section class="section" data-testid="credentials-rdp-section">
                <h3>RDP</h3>
                <div class="field">
                  <span>Remote desktop</span>
                  <span class="switch-line">
                    <button
                      type="button"
                      class="switch"
                      class:on={rdpOn}
                      role="switch"
                      aria-checked={rdpOn}
                      aria-label="Remote desktop on this host"
                      data-testid="rdp-enabled"
                      on:click={() => (overrides = { ...overrides, rdp: rdpOn ? "off" : "on" })}
                    ></button>
                    <small>{detected ? "Found on this host" : "Not found on this host"}</small>
                  </span>
                </div>
                {#if rdpOn}
                  <div class="field">
                    <span>Port</span>
                    <input class="field-control short" data-testid="tunnel-rdp-port" inputmode="numeric" bind:value={rdpPort} placeholder="3389" autocomplete="off" />
                  </div>
                {/if}
                {#each rdpTargets as target (target.key)}
                  <div class="field">
                    <span title={target.label}>{target.key ? `Forward ${target.label}` : "Credentials"}</span>
                    <Select
                      bind:value={bindings[target.key]}
                      testid="rdp-login"
                      ariaLabel={`Credentials for ${target.label}`}
                      options={[{ value: "", label: "Ask when connecting" }, ...loginOptions]}
                    />
                  </div>
                {/each}
                {#if rdpTargets.length}
                  <div class="field">
                    <span>Shared clipboard</span>
                    <span class="segmented" data-testid="clipboard-mode">
                      {#each [["both", "Both ways"], ["to", "To remote"], ["from", "From remote"], ["off", "Off"]] as [value, label]}
                        <button type="button" class:on={(overrides.clipboard || "both") === value} on:click={() => (overrides = { ...overrides, clipboard: value })}>{label}</button>
                      {/each}
                    </span>
                  </div>
                  <p class="note">Text, pictures and files. Anything you copy can reach the remote machine, so turn this off for machines you do not trust.</p>
                {/if}
              </section>
            {/if}
          </div>
        </div>
      {:else if page === "row" && $appSettings}
        <h2>Row display</h2>
        <p class="lead">What this tunnel shows in the list. Changes here override the app's default row.</p>
        <RowLayoutEditor row={overrides.row ?? settings.row} on:change={(e) => (overrides = { ...overrides, row: e.detail })} />
        <div class="actions">
          <button type="button" class="btn" data-testid="row-default" on:click={() => (overrides = { ...overrides, row: undefined })}>Use the default row</button>
          {#if tunnel.group}<button type="button" class="btn" data-testid="apply-to-group" on:click={applyToGroup}>Apply to {tunnel.group}</button>{/if}
        </div>
      {:else if page === "monitoring" && $appSettings}
        <h2>Monitoring</h2>
        <p class="lead">Each refresh runs commands over SSH, so slower values help on small servers.</p>
        {#each MONITORS as monitor}
          <div class="field">
            <span>{monitor.label}</span>
            <Select
              testid={`tunnel-refresh-${monitor.key}`}
              ariaLabel={`${monitor.label} refresh`}
              value={overrides.refresh?.[monitor.key] === undefined ? "" : String(overrides.refresh[monitor.key])}
              on:change={(e) => setRefresh(monitor.key, e.detail)}
              options={[
                { value: "", label: `Default (${formatInterval($appSettings.refresh[monitor.key]).toLowerCase()})` },
                ...REFRESH_CHOICES.map((seconds) => ({ value: String(seconds), label: formatInterval(seconds) })),
              ]}
            />
          </div>
        {/each}
      {:else if page === "windows" && $appSettings}
        <h2>Windows</h2>
        <p class="lead">What this tunnel's tool windows do when the connection drops.</p>
        <div class="field">
          <span>After disconnect</span>
          <Select
            testid="tunnel-after-disconnect"
            ariaLabel="After disconnect"
            value={overrides.afterDisconnect ?? ""}
            on:change={(e) => (overrides = { ...overrides, afterDisconnect: e.detail || undefined })}
            options={[
              { value: "", label: `Default (${$appSettings.afterDisconnect === "resume" ? "keep and resume" : $appSettings.afterDisconnect === "keep" ? "keep, reconnect by hand" : "close"})` },
              { value: "resume", label: "Keep windows and resume" },
              { value: "keep", label: "Keep windows, reconnect by hand" },
              { value: "close", label: "Close windows" },
            ]}
          />
        </div>
      {/if}
    </div>

    <footer class="bar">
      <button type="button" class="btn danger" data-testid="delete-tunnel" on:click={remove}>Delete tunnel</button>
      <button type="button" class="btn primary" data-testid="tunnel-save" disabled={saving} on:click={saveAll}>{saving ? "Saving" : "Save"}</button>
    </footer>
  </div>
</div>

<style>
  .settings {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: 150px 1fr;
  }
  nav {
    padding: 8px;
    background: var(--surface);
    border-right: 1px solid var(--border);
  }
  nav button {
    display: block;
    width: 100%;
    padding: 5px 9px;
    text-align: left;
    font-size: 12.5px;
    color: var(--muted);
    border-radius: var(--radius);
  }
  nav button.on {
    background: var(--raise);
    color: var(--text);
  }
  .main {
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }
  .page {
    flex: 1;
    min-height: 0;
    padding: 14px 16px;
    overflow-y: auto;
    font-size: 12.5px;
  }
  .bar {
    flex: none;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding: 9px 16px;
    background: var(--surface);
    border-top: 1px solid var(--border);
  }
  .hidden {
    display: none;
  }
  h2 {
    margin: 0 0 3px;
    font-size: 14px;
  }
  h3 {
    margin: 16px 0 6px;
    font-size: 12.5px;
    font-weight: 600;
  }
  .lead {
    margin: 0 0 12px;
    color: var(--muted);
    line-height: 1.4;
  }
  .note {
    margin: 8px 0;
    color: var(--muted);
  }
  .field {
    display: grid;
    grid-template-columns: 170px 1fr;
    align-items: center;
    gap: 10px;
    min-height: 34px;
    border-top: 1px solid var(--border-soft);
  }
  .field :global(.select),
  .field input.field-control {
    width: 100%;
    max-width: 320px;
    justify-self: start;
  }
  .field input.short {
    max-width: 110px;
  }
  .field span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .field code {
    overflow-wrap: anywhere;
  }
  .pin {
    margin-top: 10px;
  }
  .credentials {
    display: grid;
    grid-template-columns: 76px minmax(0, 1fr);
    gap: 12px;
    align-items: start;
  }
  .sub-nav {
    position: sticky;
    top: 0;
    padding: 0;
    background: none;
    border-right: 0;
  }
  .sections {
    display: flex;
    flex-direction: column;
    gap: 12px;
    min-width: 0;
  }
  .section {
    padding: 4px 12px 10px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }
  .section h3 {
    margin: 6px 0 8px;
    color: var(--accent);
    font-size: 13px;
  }
  .switch-line {
    display: inline-flex;
    align-items: center;
    gap: 10px;
  }
  .switch-line small {
    color: var(--muted);
    font-size: 12px;
  }
  .field .segmented {
    justify-self: start;
  }
  .actions {
    display: flex;
    gap: 6px;
    margin-top: 12px;
  }
  .check {
    display: flex;
    align-items: center;
    gap: 7px;
  }
</style>
