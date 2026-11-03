<script lang="ts">
  import { createEventDispatcher, onDestroy, onMount } from "svelte";
  import { CloseRDP, GetRDPLogin, OpenRDP, SaveRDPLogin, TrustRDPCertificate } from "../../wailsjs/go/main/App";
  import { EventsOn } from "../../wailsjs/runtime/runtime";
  import { loadIronRdp } from "../lib/ironrdp";
  import { createMacKeyMapper } from "../lib/macKeyMapper.js";
  import type { TunnelConfig } from "../types";

  export let tunnel: TunnelConfig;
  const dispatch = createEventDispatcher<{ close: void }>();

  type View = "loading" | "login" | "connecting" | "connected" | "paused" | "ended" | "certificate";

  // Values of IronErrorKind in the vendored client.
  const ERROR_WRONG_PASSWORD = 1;
  const ERROR_LOGON_FAILURE = 2;
  const ERROR_ACCESS_DENIED = 3;
  const ERROR_NEGOTIATION = 6;

  let view: View = "loading";
  let message = "";
  let screenEl: HTMLDivElement;

  let username = tunnel.user ?? "";
  let domain = "";
  let password = "";
  let remember = true;

  let certificateChange: { expected: string; actual: string } | null = null;

  // Kept for the lifetime of this window, so a session paused by hiding the
  // window can resume without asking again.
  let activeLogin: { username: string; domain: string; password: string } | null = null;
  let saveAfterConnect = false;

  let element: HTMLElement | null = null;
  let interaction: any = null;
  let sessionId = "";
  // Identifies the current attempt. Callbacks from an older one are ignored.
  let attempt = 0;

  const isMac = navigator.userAgent.includes("Mac");
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
    return { kind: -1, text: String(e?.message ?? e) };
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
      const backend = await loadIronRdp();
      if (current !== attempt) return;
      const session = await OpenRDP(tunnel.id, login as any);
      if (current !== attempt) {
        CloseRDP(session.id).catch(() => {});
        return;
      }
      sessionId = session.id;
      activeLogin = { username: session.username, domain: session.domain, password: session.password };
      const created = document.createElement("iron-remote-desktop");
      created.setAttribute("scale", "fit");
      created.setAttribute("flexcenter", "true");
      // The component reads its module when it mounts, so set it first.
      (created as any).module = backend;
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
        SaveRDPLogin(tunnel.id, activeLogin as any).catch((e) => console.error("saving RDP login failed:", e));
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
    activeLogin = { username: username.trim(), domain: domain.trim(), password };
    saveAfterConnect = remember;
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
      await TrustRDPCertificate(tunnel.id, certificateChange.actual);
      certificateChange = null;
      start();
    } catch (e: any) {
      message = String(e?.message ?? e);
      view = "ended";
    }
  }

  function close() {
    dispose();
    activeLogin = null;
    dispatch("close");
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
    if (change.tunnelId === tunnel.id) {
      certificateChange = { expected: change.expected, actual: change.actual };
    }
  });

  onMount(async () => {
    document.addEventListener("visibilitychange", onVisibilityChange);
    window.addEventListener("keydown", onKeyCapture, true);
    window.addEventListener("keyup", onKeyCapture, true);
    try {
      const saved = await GetRDPLogin(tunnel.id);
      if (saved.username) username = saved.username;
      domain = saved.domain ?? "";
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

<div class="rdp-overlay">
  <div class="rdp-titlebar">
    <span class="rdp-title">rdp — {tunnel.name}</span>
    <div class="rdp-actions">
      {#if view === "connected"}
        <button class="rdp-text-btn" on:click={() => interaction?.ctrlAltDel()} title="Send Ctrl+Alt+Del">ctrl+alt+del</button>
        <button class="rdp-text-btn" on:click={changeLogin}>change login</button>
      {/if}
      <button class="rdp-close" on:click={close} title="Close">×</button>
    </div>
  </div>

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
        <label>user <input bind:value={username} autocomplete="off" spellcheck="false" required /></label>
        <label>domain <input bind:value={domain} autocomplete="off" spellcheck="false" placeholder="optional" /></label>
        <label>password <input type="password" bind:value={password} autocomplete="off" required /></label>
        <label class="rdp-check"><input type="checkbox" bind:checked={remember} /> remember in keychain</label>
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
  .rdp-overlay { position: fixed; top: 36px; left: 0; right: 0; bottom: 0; z-index: 1000; display: flex; flex-direction: column; background: #0a0a0a; font-family: "JetBrains Mono", monospace; }
  .rdp-titlebar { display: flex; justify-content: space-between; align-items: center; padding: 4px 12px; border-bottom: 1px solid #1a1a1a; background: #111; }
  .rdp-title { font-size: 11px; color: #00ff88; }
  .rdp-actions, .rdp-panel-actions { display: flex; gap: 6px; align-items: center; }
  .rdp-body { position: relative; flex: 1; min-height: 0; overflow: hidden; display: flex; align-items: center; justify-content: center; }
  .rdp-screen { position: absolute; inset: 0; overflow: hidden; }
  .rdp-screen.rdp-hidden { visibility: hidden; }
  .rdp-status { font-size: 12px; color: #00ff88; padding: 20px; z-index: 1; }
  .rdp-error { font-size: 12px; color: #ff4444; }
  .rdp-form, .rdp-panel { z-index: 1; display: flex; flex-direction: column; gap: 10px; width: 420px; max-width: 90%; padding: 20px; border: 1px solid #1a1a1a; background: #111; color: #ccc; font-size: 12px; }
  .rdp-form-title { color: #00ff88; }
  .rdp-form label { display: flex; flex-direction: column; gap: 4px; color: #888; font-size: 11px; }
  .rdp-form input:not([type="checkbox"]) { background: #0a0a0a; border: 1px solid #333; color: #eee; padding: 6px 8px; font: inherit; border-radius: 2px; }
  .rdp-form input:focus { outline: none; border-color: #00ff88; }
  .rdp-form label.rdp-check { flex-direction: row; align-items: center; gap: 6px; }
  .rdp-panel p { margin: 0; color: #888; }
  .rdp-fingerprint { font-size: 10px; color: #888; word-break: break-all; }
  .rdp-primary, .rdp-danger, .rdp-text-btn, .rdp-close { background: none; border: 1px solid #333; color: #888; cursor: pointer; border-radius: 2px; font: inherit; font-size: 11px; padding: 4px 8px; }
  .rdp-primary { color: #00ff88; border-color: #00ff88; }
  .rdp-primary:hover { background: rgba(0, 255, 136, 0.1); }
  .rdp-danger { color: #ff4444; border-color: #ff4444; }
  .rdp-text-btn:hover { color: #00d4ff; border-color: #00d4ff; }
  .rdp-close { font-size: 16px; width: 24px; height: 24px; line-height: 1; padding: 0; }
  .rdp-close:hover { color: #ff4444; border-color: #ff4444; }
</style>
