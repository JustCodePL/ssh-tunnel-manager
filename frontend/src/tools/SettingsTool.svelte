<script lang="ts">
  import { isMac } from "../lib/platform";
  import { errorText } from "../lib/runtime";
  import { onDestroy, onMount, tick } from "svelte";
  import {
    CheckForUpdate,
    CopyToClipboard,
    GetAppLog,
    GetDiagnostics,
    OpenLogFolder,
    ResetWindowPositions,
    DeleteCredential,
    GetAutostart,
    GetCloseToTray,
    GetCurrentVersion,
    GetPortlessServiceStatus,
    GetStartMinimized,
    GetUpdateChannel,
    InstallPortlessService,
    InstallUpdate,
    ListCredentials,
    OpenPortlessServiceSettings,
    SaveCredential,
    SetAutostart,
    SetCloseToTray,
    SetStartMinimized,
    SetUpdateChannel,
    UninstallPortlessService,
  } from "../../bindings/ssh-tunnel-manager/app";
  import { BrowserOpenURL, EventsOn } from "../lib/runtime";
  import { appSettings, saveAppSettings } from "../stores/settings";
  import { REFRESH_CHOICES, formatInterval } from "../lib/toolConnection.js";
  import { showToast } from "../stores/toast";
  import { askConfirm } from "../stores/confirm";
  import { exportTunnels, tunnels } from "../stores/tunnels";
  import Select from "../components/Select.svelte";
  import RowLayoutEditor from "./RowLayoutEditor.svelte";

  const websiteUrl = "https://justcodepl.github.io/ssh-tunnel-manager/";
  const githubUrl = "https://github.com/JustCodePL/ssh-tunnel-manager";
  const MONITORS = [
    { key: "stats", label: "CPU and RAM" },
    { key: "htop", label: "htop" },
    { key: "disk", label: "Disk" },
    { key: "docker", label: "Docker" },
  ];
  const pages = [
    { id: "general", label: "General" },
    { id: "appearance", label: "Appearance" },
    { id: "windows", label: "Windows" },
    { id: "monitoring", label: "Monitoring" },
    { id: "credentials", label: "Credentials" },
    { id: "maintenance", label: "Maintenance" },
    { id: "updates", label: "Updates" },
  ];

  let page = "general";

  let autostart = false;
  let startMinimized = true;
  let closeToTray = true;

  let version = "";
  let channel: "stable" | "beta" = "stable";
  let updateInfo: any = null;
  let checking = false;
  let installing = false;
  let updateError = "";

  let portless: any = null;
  let portlessBusy = false;
  let portlessError = "";

  type LogEntry = { time: string; level: string; message: string; details: string };
  let appLog: { path: string; entries: LogEntry[]; truncated: boolean } | null = null;
  let logError = "";
  let logLevel = "all";
  let logQuery = "";
  let logEl: HTMLDivElement;
  let diagnostics: any = null;

  $: logNeedle = logQuery.trim().toLowerCase();
  $: shownLog = (appLog?.entries ?? []).filter((entry) => {
    const level = entry.level.toUpperCase();
    if (logLevel === "error" && level !== "ERROR") return false;
    if (logLevel === "warn" && level !== "WARN" && level !== "ERROR") return false;
    return !logNeedle || `${entry.message} ${entry.details}`.toLowerCase().includes(logNeedle);
  });

  function openPage(id: string) {
    page = id;
    if (id === "maintenance") void openMaintenance();
  }

  async function openMaintenance() {
    await loadLog();
    diagnostics = await GetDiagnostics();
  }

  async function loadLog() {
    try {
      appLog = (await GetAppLog(2000)) as any;
      logError = "";
      // The log opens on its newest entries.
      await tick();
      if (logEl) logEl.scrollTop = logEl.scrollHeight;
    } catch (e: any) {
      logError = errorText(e);
    }
  }

  function logText(): string {
    return shownLog.map((entry) => [entry.time, entry.level, entry.message, entry.details].filter(Boolean).join(" ")).join("\n");
  }

  async function copyLog() {
    await run(async () => {
      await CopyToClipboard(logText());
      showToast(`Copied ${shownLog.length} log entries`, "success");
    });
  }

  async function copyDiagnostics() {
    await run(async () => {
      const facts = (await GetDiagnostics()) as any;
      diagnostics = facts;
      await CopyToClipboard(`SSH Tunnel Manager ${facts.version}\n${facts.system}\n${facts.tunnels} tunnels, ${facts.connected} connected`);
      showToast("Copied", "success");
    });
  }

  async function resetWindows() {
    await run(async () => {
      await ResetWindowPositions();
      showToast("Tool windows open in their default place from now on", "success");
    });
  }

  type Credential = { id: string; name: string; username: string; domain: string; hasPassword: boolean; protocols: string[]; usedBy: number };
  let credentials: Credential[] = [];
  let form = emptyForm();
  let formError = "";
  let editingCredential = false;

  function emptyForm() {
    return { id: "", name: "", username: "", domain: "", password: "", ssh: true, rdp: true, vnc: false, hasPassword: false };
  }

  const offs: Array<() => void> = [];

  async function loadCredentials() {
    credentials = ((await ListCredentials()) ?? []) as any;
  }

  async function loadPortless() {
    try {
      portless = await GetPortlessServiceStatus();
    } catch (e: any) {
      portlessError = errorText(e);
    }
  }

  onMount(async () => {
    offs.push(EventsOn("updater:update-available", (info: any) => (updateInfo = info)));
    offs.push(EventsOn("updater:update-cleared", () => (updateInfo = null)));
    offs.push(EventsOn("settings:changed", () => void loadCredentials()));
    const [auto, minimized, tray, current, configured] = await Promise.all([
      GetAutostart().catch(() => false),
      GetStartMinimized().catch(() => true),
      GetCloseToTray().catch(() => true),
      GetCurrentVersion().catch(() => ""),
      GetUpdateChannel().catch(() => "stable"),
    ]);
    autostart = auto;
    startMinimized = minimized;
    closeToTray = tray;
    version = current;
    channel = configured === "beta" ? "beta" : "stable";
    await Promise.all([loadCredentials(), loadPortless()]);
  });

  onDestroy(() => offs.forEach((off) => off()));

  async function run(action: () => Promise<unknown>) {
    try {
      await action();
    } catch (e: any) {
      showToast(errorText(e), "error");
    }
  }

  function setRefresh(key: string, value: string) {
    if (!$appSettings) return;
    void run(() => saveAppSettings({ refresh: { ...$appSettings!.refresh, [key]: Number(value) } }));
  }

  function createCredential() {
    form = emptyForm();
    formError = "";
    editingCredential = true;
  }

  function closeCredential() {
    form = emptyForm();
    formError = "";
    editingCredential = false;
  }

  function editCredential(c: Credential) {
    editingCredential = true;
    form = {
      id: c.id,
      name: c.name,
      username: c.username,
      domain: c.domain,
      password: "",
      ssh: c.protocols.includes("ssh"),
      rdp: c.protocols.includes("rdp"),
      vnc: c.protocols.includes("vnc"),
      hasPassword: c.hasPassword,
    };
    formError = "";
  }

  async function saveCredential() {
    formError = "";
    const protocols = [form.ssh ? "ssh" : "", form.rdp ? "rdp" : "", form.vnc ? "vnc" : ""].filter(Boolean);
    try {
      const saved: any = await SaveCredential({
        id: form.id,
        name: form.name,
        username: form.username,
        domain: form.domain,
        password: form.password,
        protocols,
      } as any);
      await loadCredentials();
      closeCredential();
      showToast(`Saved ${saved.name}`, "success");
    } catch (e: any) {
      formError = errorText(e);
    }
  }

  async function deleteCredential() {
    if (!form.id) return;
    const accepted = await askConfirm({
      title: "Delete login",
      message: `Delete the login "${form.name}"? Tunnels using it will ask for a login again.`,
      confirmLabel: "Delete",
      danger: true,
    });
    if (!accepted) return;
    await run(async () => {
      await DeleteCredential(form.id);
      closeCredential();
      await loadCredentials();
    });
  }

  async function selectChannel(value: "stable" | "beta") {
    await run(async () => {
      await SetUpdateChannel(value);
      channel = value;
      updateInfo = null;
    });
  }

  async function check() {
    checking = true;
    updateError = "";
    try {
      updateInfo = await CheckForUpdate();
      if (!updateInfo) updateError = "This is the latest version.";
    } catch (e: any) {
      updateError = errorText(e);
    } finally {
      checking = false;
    }
  }

  async function install() {
    installing = true;
    try {
      await InstallUpdate();
    } catch (e: any) {
      updateError = errorText(e);
    } finally {
      installing = false;
    }
  }

  async function removePortless() {
    const accepted = await askConfirm({
      title: "Remove Portless setup",
      message: "Remove what Portless set up on this system? Portless domains stop working until a forward sets it up again.",
      confirmLabel: "Remove",
      danger: true,
    });
    if (accepted) void portlessAction(UninstallPortlessService);
  }

  async function portlessAction(action: () => Promise<unknown>) {
    portlessBusy = true;
    portlessError = "";
    try {
      await action();
    } catch (e: any) {
      portlessError = errorText(e);
    } finally {
      portlessBusy = false;
      await loadPortless();
    }
  }
</script>

<div class="settings" data-testid="settings">
  <nav aria-label="Settings sections">
    {#each pages as item}
      <button type="button" class:on={page === item.id} data-testid={`settings-${item.id}`} on:click={() => openPage(item.id)}>{item.label}</button>
    {/each}
  </nav>

  <div class="page">
    {#if page === "general"}
      <h2>General</h2>
      <p class="lead">Startup behaviour.</p>
      <label class="check"><input type="checkbox" bind:checked={autostart} on:change={() => run(() => SetAutostart(autostart))} /> Start on login</label>
      <label class="check"><input type="checkbox" bind:checked={startMinimized} on:change={() => run(() => SetStartMinimized(startMinimized))} /> Start minimized on login</label>
      <h3>Tunnels</h3>
      <div class="actions start">
        <button type="button" class="btn" data-testid="export-tunnels" disabled={$tunnels.length === 0} on:click={() => run(() => exportTunnels($tunnels.map((t) => t.id)))}>Export tunnels</button>
      </div>
    {:else if page === "appearance" && $appSettings}
      <h2>Appearance</h2>
      <p class="lead">Theme, and what a tunnel's row shows until you change it on that tunnel.</p>
      <div class="field">
        <span>Theme</span>
        <span class="segmented">
          <button type="button" class:on={$appSettings.theme === "terminal"} data-testid="theme-terminal" on:click={() => run(() => saveAppSettings({ theme: "terminal" }))}>Terminal</button>
          <button type="button" class:on={$appSettings.theme === "graphite"} data-testid="theme-graphite" on:click={() => run(() => saveAppSettings({ theme: "graphite" }))}>Graphite</button>
        </span>
      </div>
      <h3>Default row</h3>
      <RowLayoutEditor row={$appSettings.row} on:change={(e) => run(() => saveAppSettings({ row: e.detail }))} />
    {:else if page === "windows" && $appSettings}
      <h2>Windows</h2>
      <p class="lead">How tool windows are organised and how they react to the connection.</p>
      <div class="field">
        <span>Group windows</span>
        <Select
          testid="window-grouping"
          ariaLabel="Group windows"
          value={$appSettings.windowGrouping}
          on:change={(e) => run(() => saveAppSettings({ windowGrouping: e.detail }))}
          options={[
            { value: "tool-target", label: "One window per tool and target" },
            { value: "tool", label: "One window per tool, a tab per target" },
            { value: "target", label: "One window per target, a tab per tool" },
          ]}
        />
      </div>
      <div class="field">
        <span>After disconnect</span>
        <Select
          testid="after-disconnect"
          ariaLabel="After disconnect"
          value={$appSettings.afterDisconnect}
          on:change={(e) => run(() => saveAppSettings({ afterDisconnect: e.detail }))}
          options={[
            { value: "resume", label: "Keep windows and resume" },
            { value: "keep", label: "Keep windows, reconnect by hand" },
            { value: "close", label: "Close windows" },
          ]}
        />
      </div>
      <h3>Closing the main window</h3>
      <label class="check"><input type="checkbox" bind:checked={closeToTray} on:change={() => run(() => SetCloseToTray(closeToTray))} /> Hide to the tray instead of quitting</label>
      <label class="check"><input type="checkbox" disabled={!closeToTray} checked={$appSettings.hideToolsWithMain} on:change={(e) => run(() => saveAppSettings({ hideToolsWithMain: e.currentTarget.checked }))} /> Hide tool windows with it</label>
      <p class="note">A hidden remote desktop pauses and resumes when shown again.</p>
    {:else if page === "monitoring" && $appSettings}
      <h2>Monitoring</h2>
      <p class="lead">Default refresh intervals. Each refresh runs commands over SSH, and a tunnel can override them in its own settings.</p>
      {#each MONITORS as monitor}
        <div class="field">
          <span>{monitor.label}</span>
          <Select
            testid={`refresh-${monitor.key}`}
            ariaLabel={`${monitor.label} refresh`}
            value={String($appSettings.refresh[monitor.key])}
            on:change={(e) => setRefresh(monitor.key, e.detail)}
            options={REFRESH_CHOICES.map((seconds) => ({ value: String(seconds), label: formatInterval(seconds) }))}
          />
        </div>
      {/each}
      <p class="note">Minimized and hidden windows pause refreshing.</p>
    {:else if page === "credentials"}
      <h2>Credentials</h2>
      <p class="lead">Logins for SSH and remote desktops, saved in the system keychain and reusable across tunnels. Passwords never go to the SSH config or to exports.</p>
      {#if editingCredential}
        <form class="card" data-testid="credential-form" on:submit|preventDefault={saveCredential}>
          <h3 class="card-title">{form.id ? `Edit ${form.name}` : "New credential"}</h3>
          <div class="grid">
            <label>Name<input class="field-control" data-testid="credential-name" bind:value={form.name} /></label>
            <label>User<input class="field-control" data-testid="credential-user" bind:value={form.username} autocomplete="off" spellcheck="false" /></label>
            <label>Domain<input class="field-control" bind:value={form.domain} placeholder="Optional" autocomplete="off" spellcheck="false" /></label>
            <label>Password<input class="field-control" data-testid="credential-password" type="password" bind:value={form.password} autocomplete="off" placeholder={form.hasPassword ? "Saved, type to replace" : ""} /></label>
          </div>
          <div class="line">
            <span>Use for</span>
            <label class="check inline"><input type="checkbox" data-testid="credential-ssh" bind:checked={form.ssh} /> SSH</label>
            <label class="check inline"><input type="checkbox" data-testid="credential-rdp" bind:checked={form.rdp} /> RDP</label>
            <label class="check inline"><input type="checkbox" bind:checked={form.vnc} /> VNC</label>
          </div>
          {#if formError}<p class="error" data-testid="credential-error">{formError}</p>{/if}
          <div class="actions">
            {#if form.id}<button type="button" class="btn danger" data-testid="credential-delete" on:click={deleteCredential}>Delete</button>{/if}
            <button type="button" class="btn" data-testid="credential-cancel" on:click={closeCredential}>Cancel</button>
            <button type="submit" class="btn primary" data-testid="credential-save">Save</button>
          </div>
        </form>
      {:else}
        <div class="actions start list-actions">
          <button type="button" class="btn primary" data-testid="credential-create" on:click={createCredential}>Create credential</button>
        </div>
        {#if credentials.length}
          <table class="data-table" data-testid="credentials-table">
            <thead><tr><th>Name</th><th>User</th><th>Domain</th><th>For</th><th>Used by</th><th></th></tr></thead>
            <tbody>
              {#each credentials as c (c.id)}
                <tr class="clickable" data-testid="credential-row" on:click={() => editCredential(c)}>
                  <td>{c.name}</td>
                  <td class="dim">{c.username}</td>
                  <td class="dim">{c.domain}</td>
                  <td>{c.protocols.map((p) => p.toUpperCase()).join(", ")}</td>
                  <td class="dim">{c.usedBy} target{c.usedBy === 1 ? "" : "s"}</td>
                  <td class="row-edit"><button type="button" class="btn" data-testid="credential-edit" on:click|stopPropagation={() => editCredential(c)}>Edit</button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        {:else}
          <p class="note">No saved logins yet. A login that works in a remote desktop can be saved here too.</p>
        {/if}
      {/if}
    {:else if page === "maintenance"}
      <h2>Maintenance</h2>
      <p class="lead">Checks and repairs for what the app sets up on this system.</p>
      <section class="section" data-testid="maintenance-logs">
        <h3>Logs</h3>
        <div class="log-tools">
          <span class="segmented" data-testid="log-level">
            {#each [["all", "All"], ["warn", "Warnings"], ["error", "Errors"]] as [value, label]}
              <button type="button" class:on={logLevel === value} data-testid={`log-level-${value}`} on:click={() => (logLevel = value)}>{label}</button>
            {/each}
          </span>
          <input class="field-control log-search" placeholder="Search the log" aria-label="Search the log" data-testid="log-search" bind:value={logQuery} />
          <button type="button" class="btn" data-testid="log-refresh" on:click={loadLog}>Refresh</button>
          <button type="button" class="btn" data-testid="log-copy" disabled={!shownLog.length} on:click={copyLog}>Copy</button>
          <button type="button" class="btn" data-testid="log-folder" on:click={() => run(OpenLogFolder)}>Open folder</button>
        </div>
        <div class="log" bind:this={logEl} data-testid="log-entries">
          {#each shownLog as entry}
            <div class="log-line" data-testid="log-line" data-level={entry.level}>
              {#if entry.time}<time title={entry.time}>{entry.time.slice(5, 19).replace("T", " ")}</time>{/if}
              {#if entry.level}<b class={entry.level.toLowerCase()}>{entry.level}</b>{/if}
              <span>{entry.message}{#if entry.details}<small> {entry.details}</small>{/if}</span>
            </div>
          {:else}
            <p class="note" data-testid="log-empty">{logError || (appLog?.entries?.length ? "Nothing matches this filter." : "The log is empty.")}</p>
          {/each}
        </div>
        <p class="note">{appLog?.truncated ? "Showing the newest entries. " : ""}Logs can contain server addresses and proxy commands, so read them before you share them.</p>
      </section>

      {#if isMac}
        <section class="section" data-testid="maintenance-portless">
          <h3>Portless</h3>
          {#if portless}
            {#if portless.problem}
              <p class="error" data-testid="portless-status">{portless.message}</p>
            {:else}
              <p class="ok" data-testid="portless-status">No problems detected with Portless.</p>
              <p class="note">{portless.message}</p>
            {/if}
          {/if}
          <div class="actions start">
            <button type="button" class="btn" data-testid="portless-install" disabled={portlessBusy} on:click={() => portlessAction(InstallPortlessService)}>{portless?.installed ? "Repair" : "Set up"}</button>
            <button type="button" class="btn danger" data-testid="portless-remove" disabled={portlessBusy} on:click={removePortless}>Remove</button>
            {#if isMac}<button type="button" class="btn" data-testid="portless-settings" on:click={() => portlessAction(OpenPortlessServiceSettings)}>Open system settings</button>{/if}
            <button type="button" class="btn" data-testid="portless-refresh" disabled={portlessBusy} on:click={loadPortless}>Refresh status</button>
          </div>
          {#if portlessError}<p class="error">{portlessError}</p>{/if}
        </section>
      {/if}

      <section class="section" data-testid="maintenance-about">
        <h3>This installation</h3>
        {#if diagnostics}
          <div class="field"><span>Version</span><code data-testid="diagnostics-version">{diagnostics.version}</code></div>
          <div class="field"><span>System</span><code>{diagnostics.system}</code></div>
          <div class="field"><span>Tunnels</span><code>{diagnostics.tunnels}, {diagnostics.connected} connected</code></div>
          <div class="field"><span>SSH config</span><code>{diagnostics.sshConfig}</code></div>
          <div class="field"><span>Settings</span><code>{diagnostics.settingsFile}</code></div>
          <div class="field"><span>Log</span><code>{diagnostics.logFile}</code></div>
        {/if}
        <div class="actions start">
          <button type="button" class="btn" data-testid="diagnostics-copy" on:click={copyDiagnostics}>Copy for a bug report</button>
          <button type="button" class="btn" data-testid="reset-windows" on:click={resetWindows}>Reset window positions</button>
        </div>
        <p class="note">Reset window positions helps when a tool window opens on a screen that is no longer attached.</p>
      </section>
    {:else if page === "updates"}
      <h2>Updates</h2>
      <p class="lead">You start every update yourself.</p>
      <div class="field"><span>Version</span><code>{version}</code></div>
      <div class="field">
        <span>Channel</span>
        <span class="segmented">
          <button type="button" class:on={channel === "stable"} on:click={() => selectChannel("stable")}>Stable</button>
          <button type="button" class:on={channel === "beta"} on:click={() => selectChannel("beta")}>Beta</button>
        </span>
      </div>
      {#if channel === "beta"}<p class="note">Beta builds may be unstable.</p>{/if}
      {#if updateInfo}
        <p class="ok">Version {updateInfo.latestVersion} is available.</p>
        <button type="button" class="btn primary" disabled={installing} on:click={install}>{installing ? "Installing" : "Install and restart"}</button>
      {:else}
        <button type="button" class="btn" disabled={checking} on:click={check}>{checking ? "Checking" : "Check for updates"}</button>
      {/if}
      {#if updateError}<p class="note">{updateError}</p>{/if}
      <h3>Project</h3>
      <div class="actions start">
        <button type="button" class="btn" on:click={() => BrowserOpenURL(websiteUrl)}>Website</button>
        <button type="button" class="btn" on:click={() => BrowserOpenURL(githubUrl)}>GitHub</button>
      </div>
    {/if}
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
  .page {
    padding: 14px 16px;
    overflow-y: auto;
    font-size: 12.5px;
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
  .ok {
    color: var(--live);
  }
  .error {
    color: var(--error);
    margin: 6px 0;
  }
  .field {
    display: grid;
    grid-template-columns: 140px 1fr;
    align-items: center;
    gap: 10px;
    min-height: 34px;
    border-top: 1px solid var(--border-soft);
  }
  .field :global(.select) {
    width: 100%;
    max-width: 320px;
    justify-self: start;
  }
  .card-title {
    margin-top: 0;
  }
  .section {
    padding: 4px 12px 12px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }
  .section + .section {
    margin-top: 12px;
  }
  .section h3 {
    margin: 6px 0 8px;
    color: var(--accent);
    font-size: 13px;
  }
  .section code {
    overflow-wrap: anywhere;
  }
  .log-tools {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    margin-bottom: 8px;
  }
  .log-search {
    flex: 1;
    min-width: 120px;
  }
  .log {
    height: 260px;
    overflow-y: auto;
    padding: 6px 8px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    font: 11.5px/1.5 var(--mono);
  }
  .log-line {
    display: flex;
    gap: 8px;
    align-items: baseline;
  }
  .log-line time {
    flex: none;
    color: var(--muted);
  }
  .log-line b {
    flex: none;
    width: 38px;
    font-weight: 500;
    color: var(--muted);
  }
  .log-line b.warn {
    color: var(--warning);
  }
  .log-line b.error {
    color: var(--error);
  }
  .log-line span {
    min-width: 0;
    overflow-wrap: anywhere;
    font-weight: 400;
  }
  .log-line small {
    margin-left: 6px;
    color: var(--muted);
    font-size: 11px;
    font-weight: 400;
  }
  .list-actions {
    margin-bottom: 10px;
  }
  .row-edit {
    text-align: right;
  }
  .field .segmented {
    justify-self: start;
  }
  .check {
    display: flex;
    align-items: center;
    gap: 7px;
    min-height: 26px;
  }
  .check.inline {
    display: inline-flex;
  }
  .clickable {
    cursor: pointer;
  }
  .card {
    margin-top: 12px;
    padding: 10px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }
  .grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px 12px;
  }
  .grid label {
    display: grid;
    gap: 3px;
    font-size: 12px;
    color: var(--muted);
  }
  .grid input {
    width: 100%;
    background: var(--bg);
  }
  .line {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 8px;
    color: var(--muted);
  }
  .line .check {
    color: var(--text);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
    margin-top: 10px;
  }
  .actions.start {
    justify-content: flex-start;
  }
</style>
