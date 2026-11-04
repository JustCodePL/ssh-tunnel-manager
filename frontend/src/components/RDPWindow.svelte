<script lang="ts">
  import { isMac } from "../lib/platform";
  import Select from "./Select.svelte";
  import { errorText } from "../lib/runtime";
  import { onDestroy, onMount } from "svelte";
  import { BindCredential, BindTabSession, CloseRDP, GetCredentialBinding, GetRDPLogin, ListCredentials, OpenRDP, ReadClipboard, SaveRDPLogin, SaveRemoteFile, TrustRDPCertificate, WriteClipboardImage, WriteClipboardText } from "../../bindings/ssh-tunnel-manager/app";
  import { EventsOn } from "../lib/runtime";
  import { loadIronRdp } from "../lib/ironrdp";
  import { createMacKeyMapper } from "../lib/macKeyMapper.js";
  import { bytesToBase64, createClipboardShim, installClipboardShim } from "../lib/clipboardBridge.js";
  import { showToast } from "../stores/toast";
  import type { PortForward, TunnelConfig } from "../types";

  export let tunnel: TunnelConfig;
  // The forward to open a desktop on. Without one, the host's own RDP server.
  export let forward: PortForward | null = null;
  export let tabId = "";
  export let clipboardMode = "both";

  type View = "loading" | "login" | "connecting" | "connected" | "paused" | "ended" | "certificate";

  // Values of IronErrorKind in the vendored client.
  const ERROR_WRONG_PASSWORD = 1;
  const ERROR_LOGON_FAILURE = 2;
  const ERROR_ACCESS_DENIED = 3;
  const ERROR_RDCLEANPATH = 4;
  const ERROR_NEGOTIATION = 6;

  const target = forward
    ? { remoteHost: forward.remoteHost, remotePort: forward.remotePort }
    : { remoteHost: "", remotePort: 0 };
  const targetLabel = forward ? `${tunnel.name} → ${forward.remoteHost}:${forward.remotePort}` : tunnel.name;
  let stateKey = "";

  let view: View = "loading";
  let rdpModule: any = null;
  let message = "";
  let screenEl: HTMLDivElement;

  let username = tunnel.user ?? "";
  let domain = "";
  let password = "";
  let saveLogin = true;

  let certificateChange: { expected: string; actual: string } | null = null;

  // Kept for the lifetime of this window, so a session paused by hiding the
  // window can resume without asking again.
  let activeLogin: { username: string; domain: string; password: string; credentialId?: string } | null = null;
  let saveAfterConnect = false;
  // A saved login chosen in the form, assigned to this target once it works.
  let assignAfterConnect = "";

  let element: HTMLElement | null = null;
  let interaction: any = null;
  let sessionId = "";
  // Identifies the current attempt. Callbacks from an older one are ignored.
  let attempt = 0;

  // The client reads navigator.clipboard, so this window gets the backend's
  // version, which follows the tunnel's clipboard setting as it changes.
  let currentClipboardMode = clipboardMode;
  $: currentClipboardMode = clipboardMode;
  installClipboardShim(
    createClipboardShim({
      read: (includeImage: boolean) => ReadClipboard(includeImage) as any,
      writeText: (text: string) => WriteClipboardText(text) as any,
      writeImage: (png: string) => WriteClipboardImage(png) as any,
      mode: () => currentClipboardMode,
    }),
  );

  // Files copied on the remote desktop, waiting for the user to save them.
  let remoteFiles: { info: any; index: number }[] = [];
  let fileProvider: any = null;
  let filePicker: HTMLInputElement;
  let savedLogins: { id: string; name: string; username: string; domain: string; protocols: string[] }[] = [];
  let selectedLogin = "";
  $: chosenLogin = savedLogins.find((item) => item.id === selectedLogin);

  async function setupFileTransfer(ui: any, module: any) {
    if (currentClipboardMode === "off" || !module?.RdpFileTransferProvider) return;
    fileProvider = new module.RdpFileTransferProvider({ chunkSize: 64 * 1024 });
    ui.enableFileTransfer(fileProvider);
    fileProvider.on("files-available", (files: any[]) => {
      remoteFiles = files.map((info, index) => ({ info, index }));
    });
    fileProvider.on("format-list-response", (accepted: boolean) => {
      if (accepted && pendingPaste) {
        pendingPaste = false;
        interaction?.ctrlV();
      }
    });
    fileProvider.on("error", (error: any) => {
      showToast(`File transfer failed: ${error?.message ?? error}`, "error");
    });
  }

  let pendingPaste = false;

  function sendFiles(event: Event) {
    const input = event.target as HTMLInputElement;
    const files = Array.from(input.files ?? []);
    input.value = "";
    if (!files.length || !fileProvider) return;
    pendingPaste = true;
    const upload = fileProvider.uploadFiles(files);
    Promise.resolve(upload?.completion).catch((e: any) => {
      pendingPaste = false;
      showToast(`Could not send the files: ${errorText(e)}`, "error");
    });
    showToast(`Sending ${files.length} file${files.length > 1 ? "s" : ""}. Paste to place them.`, "success");
  }

  async function saveRemoteFile(entry: { info: any; index: number }) {
    try {
      const { completion } = fileProvider.downloadFile(entry.info, entry.index);
      const blob: Blob = await completion;
      const encoded = bytesToBase64(new Uint8Array(await blob.arrayBuffer()));
      const path = await SaveRemoteFile(entry.info.name, encoded);
      if (path) {
        showToast(`Saved ${path}`, "success");
        remoteFiles = remoteFiles.filter((file) => file !== entry);
      }
    } catch (e: any) {
      showToast(`Could not save ${entry.info?.name}: ${e?.message ?? e}`, "error");
    }
  }

  let mapMacKey = createMacKeyMapper();

  function dispose() {
    attempt++;
    try {
      interaction?.shutdown();
    } catch (e) {
      console.warn("RDP shutdown failed:", e);
    }
    interaction = null;
    element?.remove();
    element = null;
    if (sessionId) {
      CloseRDP(sessionId).catch((e) => console.warn("closing RDP session failed:", e));
      sessionId = "";
    }
    mapMacKey = createMacKeyMapper();
  }

  function describeError(e: any): { kind: number; text: string } {
    if (typeof e?.kind === "function") {
      let detail = "";
      try {
        detail = e.backtrace();
      } catch {
        detail = "";
      }
      return { kind: e.kind(), text: detail.split("\n")[0] || "connection failed" };
    }
    return { kind: -1, text: errorText(e) };
  }

  function handleFailure(e: any) {
    const { kind, text } = describeError(e);
    console.error("RDP connection failed:", text);
    dispose();
    if (certificateChange) {
      view = "certificate";
      return;
    }
    if (kind === ERROR_WRONG_PASSWORD || kind === ERROR_LOGON_FAILURE) {
      message = "The server rejected this login.";
      activeLogin = null;
      password = "";
      view = "login";
      return;
    }
    if (kind === ERROR_ACCESS_DENIED) {
      message = "This account has no remote desktop access.";
    } else if (kind === ERROR_NEGOTIATION) {
      message = "The server does not accept the security settings this client offers.";
    } else if (kind === ERROR_RDCLEANPATH) {
      message = `Could not connect to the RDP server of ${targetLabel}.`;
    } else {
      message = text;
    }
    view = "ended";
  }

  async function start() {
    if (!activeLogin) return;
    const login = activeLogin;
    dispose();
    const current = attempt;
    view = "connecting";
    message = "";
    certificateChange = null;
    try {
      const { backend, exports: rdpExports } = await loadIronRdp();
      if (current !== attempt) return;
      const session: any = await OpenRDP(tunnel.id, target as any, login as any);
      if (current !== attempt) {
        CloseRDP(session.id).catch(() => {});
        return;
      }
      sessionId = session.id;
      if (tabId) void BindTabSession(tabId, "rdp", sessionId);
      stateKey = session.stateKey;
      activeLogin = { username: session.username, domain: session.domain, password: session.password };
      const created = document.createElement("iron-remote-desktop");
      created.setAttribute("scale", "fit");
      created.setAttribute("flexcenter", "true");
      // The component reads its module when it mounts, so set it first.
      (created as any).module = backend;
      rdpModule = rdpExports;
      created.addEventListener("ready", (event: any) => {
        if (current === attempt) onReady(event.detail.irgUserInteraction, session, current);
      });
      element = created;
      screenEl.appendChild(created);
    } catch (e) {
      if (current === attempt) handleFailure(e);
    }
  }

  async function onReady(ui: any, session: any, current: number) {
    interaction = ui;
    void setupFileTransfer(ui, rdpModule);
    const config = ui
      .configBuilder()
      .withUsername(session.username)
      .withPassword(session.password)
      .withServerDomain(session.domain)
      .withDestination(session.destination)
      .withProxyAddress(session.proxyAddress)
      .withAuthToken(session.token)
      .withDesktopSize({ width: Math.max(screenEl.clientWidth, 640), height: Math.max(screenEl.clientHeight, 480) })
      .build();
    try {
      const info = await ui.connect(config);
      if (current !== attempt) return;
      view = "connected";
      ui.setVisibility(true);
      element?.focus();
      if (saveAfterConnect && activeLogin) {
        saveAfterConnect = false;
        SaveRDPLogin(tunnel.id, target as any, activeLogin as any).catch((e) => console.error("saving RDP login failed:", e));
      }
      if (assignAfterConnect) {
        const id = assignAfterConnect;
        assignAfterConnect = "";
        BindCredential("rdp", tunnel.id, target as any, id).catch((e) => console.error("assigning RDP login failed:", e));
      }
      info
        .run()
        .then((termination: any) => {
          if (current !== attempt) return;
          dispose();
          message = termination?.reason?.() ?? "The session ended.";
          view = "ended";
        })
        .catch((e: any) => {
          if (current === attempt) handleFailure(e);
        });
    } catch (e) {
      if (current === attempt) handleFailure(e);
    }
  }

  function submitLogin() {
    const chosen = savedLogins.find((item) => item.id === selectedLogin);
    if (chosen) {
      // An empty password makes the backend read the chosen login.
      activeLogin = { username: chosen.username, domain: chosen.domain, password: "", credentialId: chosen.id };
      saveAfterConnect = false;
      assignAfterConnect = saveLogin ? chosen.id : "";
    } else {
      activeLogin = { username: username.trim(), domain: domain.trim(), password };
      saveAfterConnect = saveLogin;
      assignAfterConnect = "";
    }
    password = "";
    start();
  }

  function changeLogin() {
    dispose();
    activeLogin = null;
    message = "";
    view = "login";
  }

  async function trustCertificate() {
    if (!certificateChange) return;
    try {
      await TrustRDPCertificate(tunnel.id, target as any, certificateChange.actual);
      certificateChange = null;
      start();
    } catch (e: any) {
      message = errorText(e);
      view = "ended";
    }
  }

  // A hidden webview stops running timers and rendering, which freezes the
  // session. Disconnect cleanly and resume when the window is visible again.
  function onVisibilityChange() {
    if (document.hidden) {
      if (view === "connected" || view === "connecting") {
        dispose();
        view = "paused";
      }
    } else if (view === "paused") {
      start();
    }
  }

  function onKeyCapture(evt: KeyboardEvent) {
    if (!isMac || view !== "connected" || !evt.isTrusted || document.activeElement !== element) return;
    const mapped = mapMacKey(evt as any);
    if (!mapped) return;
    evt.stopImmediatePropagation();
    evt.preventDefault();
    for (const key of mapped) {
      window.dispatchEvent(new KeyboardEvent(key.type, { ...key, bubbles: true, cancelable: true }));
    }
  }

  const offCertificateChanged = EventsOn("rdp:certificate-changed", (change: any) => {
    if (change.stateKey === stateKey) {
      certificateChange = { expected: change.expected, actual: change.actual };
    }
  });

  onMount(async () => {
    document.addEventListener("visibilitychange", onVisibilityChange);
    window.addEventListener("keydown", onKeyCapture, true);
    window.addEventListener("keyup", onKeyCapture, true);
    try {
      // Every login is offered, because one saved for SSH often opens the same
      // machine's desktop too.
      savedLogins = ((await ListCredentials()) ?? []).filter((c: any) => c.hasPassword) as any;
      const saved = await GetRDPLogin(tunnel.id, target as any);
      if (saved.username) username = saved.username;
      domain = saved.domain ?? "";
      selectedLogin = saved.credentialId || (await GetCredentialBinding("ssh", tunnel.id, target as any)) || "";
      if (saved.hasPassword) {
        // An empty password makes the backend use the stored login.
        activeLogin = { username, domain, password: "" };
        start();
        return;
      }
    } catch (e) {
      console.warn("reading stored RDP login failed:", e);
    }
    view = "login";
  });

  onDestroy(() => {
    document.removeEventListener("visibilitychange", onVisibilityChange);
    window.removeEventListener("keydown", onKeyCapture, true);
    window.removeEventListener("keyup", onKeyCapture, true);
    offCertificateChanged();
    dispose();
  });
</script>

<div class="rdp-overlay" data-testid="rdp">
  <div class="rdp-titlebar">
    <span class="rdp-title">rdp — {targetLabel}</span>
    <div class="rdp-actions">
      {#if view === "connected"}
        <button class="rdp-text-btn" on:click={() => interaction?.ctrlAltDel()} title="Send Ctrl+Alt+Del">ctrl+alt+del</button>
        <button class="rdp-text-btn" on:click={changeLogin}>change login</button>
        {#if fileProvider}
          <button class="rdp-text-btn" data-testid="rdp-send-files" on:click={() => filePicker.click()}>send files</button>
          <input bind:this={filePicker} type="file" multiple hidden on:change={sendFiles} />
        {/if}
      {/if}
    </div>
  </div>

  {#if remoteFiles.length}
    <div class="rdp-files" data-testid="rdp-remote-files">
      <span>Copied on the remote desktop:</span>
      {#each remoteFiles as entry}
        <button class="rdp-text-btn" on:click={() => saveRemoteFile(entry)}>save {entry.info.name}</button>
      {/each}
    </div>
  {/if}

  <div class="rdp-body">
    <div bind:this={screenEl} class="rdp-screen" class:rdp-hidden={view !== "connected"} />

    {#if view === "loading" || view === "connecting"}
      <div class="rdp-status">connecting...</div>
    {:else if view === "paused"}
      <div class="rdp-status">paused while the window is hidden</div>
    {:else if view === "login"}
      <form class="rdp-form" on:submit|preventDefault={submitLogin}>
        <div class="rdp-form-title">remote desktop login</div>
        {#if message}<div class="rdp-error">{message}</div>{/if}
        {#if savedLogins.length}
          <div class="rdp-field">
            <span>saved login</span>
            <Select
              bind:value={selectedLogin}
              testid="rdp-saved-login"
              ariaLabel="Saved login"
              options={[
                { value: "", label: "type a login" },
                ...savedLogins.map((login) => ({ value: login.id, label: `${login.name} (${login.protocols.map((protocol) => protocol.toUpperCase()).join(", ")})` })),
              ]}
            />
          </div>
        {/if}
        {#if chosenLogin}
          <div class="rdp-chosen" data-testid="rdp-chosen-login">{chosenLogin.domain ? `${chosenLogin.domain}\\` : ""}{chosenLogin.username}</div>
        {:else}
          <label>user <input bind:value={username} autocomplete="off" spellcheck="false" required /></label>
          <label>domain <input bind:value={domain} autocomplete="off" spellcheck="false" placeholder="optional" /></label>
          <label>password <input type="password" bind:value={password} autocomplete="off" required /></label>
        {/if}
        <label class="rdp-check"><input type="checkbox" data-testid="rdp-save-login" bind:checked={saveLogin} /> {chosenLogin ? "use this login here next time" : "save in credentials"}</label>
        <button class="rdp-primary" type="submit">connect</button>
      </form>
    {:else if view === "ended"}
      <div class="rdp-panel">
        <div class="rdp-status">{message || "The session ended."}</div>
        <div class="rdp-panel-actions">
          {#if activeLogin}<button class="rdp-primary" on:click={start}>reconnect</button>{/if}
          <button class="rdp-text-btn" on:click={changeLogin}>change login</button>
        </div>
      </div>
    {:else if view === "certificate" && certificateChange}
      <div class="rdp-panel">
        <div class="rdp-error">The certificate of this RDP server has changed.</div>
        <p>This is expected if the server was reinstalled. Otherwise, someone may be intercepting the connection.</p>
        <div class="rdp-fingerprint">trusted: {certificateChange.expected}</div>
        <div class="rdp-fingerprint">received: {certificateChange.actual}</div>
        <div class="rdp-panel-actions">
          <button class="rdp-danger" on:click={trustCertificate}>trust new certificate</button>
          <button class="rdp-text-btn" on:click={close}>close</button>
        </div>
      </div>
    {/if}
  </div>
</div>

<style>
  .rdp-files { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; padding: 5px 10px; font-size: 12px; color: var(--muted); border-bottom: 1px solid var(--border); }

  .rdp-overlay { flex: 1; min-height: 0; display: flex; flex-direction: column; background: var(--bg); }
  .rdp-titlebar { display: flex; justify-content: space-between; align-items: center; padding: 4px 12px; border-bottom: 1px solid var(--border); background: var(--surface); }
  .rdp-title { font-size: 12px; color: var(--accent); }
  .rdp-actions, .rdp-panel-actions { display: flex; gap: 6px; align-items: center; }
  .rdp-body { position: relative; flex: 1; min-height: 0; overflow: hidden; display: flex; align-items: center; justify-content: center; }
  .rdp-screen { position: absolute; inset: 0; overflow: hidden; }
  .rdp-screen.rdp-hidden { visibility: hidden; }
  .rdp-status { font-size: 12px; color: var(--accent); padding: 20px; z-index: 1; }
  .rdp-error { font-size: 12px; color: var(--error); }
  .rdp-form, .rdp-panel { z-index: 1; display: flex; flex-direction: column; gap: 10px; width: 420px; max-width: 90%; padding: 20px; border: 1px solid var(--border); background: var(--surface); color: var(--text); font-size: 12px; }
  .rdp-form-title { color: var(--accent); }
  .rdp-chosen { color: var(--text); font-size: 12px; overflow-wrap: anywhere; }
  .rdp-form label { display: flex; flex-direction: column; gap: 4px; color: var(--muted); font-size: 12px; }
  .rdp-field { display: flex; flex-direction: column; gap: 4px; color: var(--muted); font-size: 12px; }
  .rdp-field :global(.select) { display: flex; }
  .rdp-form input:not([type="checkbox"]) { background: var(--bg); border: 1px solid var(--border); color: var(--text); padding: 6px 8px; font: inherit; border-radius: 2px; }
  .rdp-form input:focus { outline: none; border-color: var(--accent); }
  .rdp-form label.rdp-check { flex-direction: row; align-items: center; gap: 6px; }
  .rdp-panel p { margin: 0; color: var(--muted); }
  .rdp-fingerprint { font-size: 11.5px; color: var(--muted); word-break: break-all; }
  .rdp-primary, .rdp-danger, .rdp-text-btn, .rdp-close { background: none; border: 1px solid var(--border); color: var(--muted); cursor: pointer; border-radius: 2px; font: inherit; font-size: 12px; padding: 4px 8px; }
  .rdp-primary { color: var(--accent); border-color: var(--accent); }
  .rdp-primary:hover { background: var(--accent-soft); }
  .rdp-danger { color: var(--error); border-color: var(--error); }
  .rdp-text-btn:hover { color: var(--accent2); border-color: var(--accent2); }
  .rdp-close { font-size: 16px; width: 24px; height: 24px; line-height: 1; padding: 0; }
  .rdp-close:hover { color: var(--error); border-color: var(--error); }
</style>
