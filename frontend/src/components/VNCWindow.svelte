<script lang="ts">
  import Select from "./Select.svelte";
  import { errorText } from "../lib/runtime";
  import { onDestroy, onMount } from "svelte";
  import { BindTabSession, CloseVNC, GetCredentialBinding, GetVNCLogin, ListCredentials, OpenVNC, ReadClipboard, SetVNCFullColor, WriteClipboardText } from "../../bindings/ssh-tunnel-manager/app";
  import { loadNoVnc } from "../lib/novnc";
  import { showToast } from "../stores/toast";
  import { createVncClipboard } from "../lib/vncClipboard.js";
  import type { PortForward, TunnelConfig } from "../types";

  export let tunnel: TunnelConfig;
  // The forward to open a screen on. Without one, the host's own VNC server.
  export let forward: PortForward | null = null;
  export let tabId = "";
  export let clipboardMode = "both";
  export let visible = true;

  type View = "connecting" | "connected" | "login" | "ended";
  type Login = { username: string; password: string; credentialId: string; save: boolean };

  const target = forward
    ? { remoteHost: forward.remoteHost, remotePort: forward.remotePort }
    : { remoteHost: "", remotePort: 0 };
  const targetLabel = forward ? `${tunnel.name} → ${forward.remoteHost}:${forward.remotePort}` : tunnel.name;

  let view: View = "connecting";
  let message = "";
  let screenEl: HTMLDivElement;
  let desktopName = "";
  let fit = true;
  // macOS Screen Sharing gets 16-bit colour unless full colour is chosen for
  // the target; other servers always show what they send.
  let apple = false;
  let reducedColor = false;

  let username = "";
  let password = "";
  let needsUsername = false;
  let saveLogin = true;
  let savedLogins: { id: string; name: string; username: string; protocols: string[] }[] = [];
  let selectedLogin = "";
  $: chosenLogin = savedLogins.find((item) => item.id === selectedLogin);

  // The login of the last attempt, kept for this window's lifetime so
  // reconnecting does not ask again. Empty means the saved or no login.
  let activeLogin: Login = { username: "", password: "", credentialId: "", save: false };

  let rfb: any = null;
  let sessionId = "";
  // Identifies the current attempt. Callbacks from an older one are ignored.
  let attempt = 0;

  let currentClipboardMode = clipboardMode;
  $: currentClipboardMode = clipboardMode;
  const clipboard = createVncClipboard({
    read: async () => (await ReadClipboard(false))?.text ?? "",
    write: (text: string) => WriteClipboardText(text) as any,
    mode: () => currentClipboardMode,
  });

  function dispose() {
    attempt++;
    if (rfb) {
      try {
        rfb.disconnect();
      } catch (e) {
        console.warn("VNC disconnect failed:", e);
      }
      rfb = null;
    }
    if (sessionId) {
      CloseVNC(sessionId).catch((e) => console.warn("closing VNC session failed:", e));
      sessionId = "";
    }
  }

  function showLogin(text: string, usernameNeeded: boolean) {
    dispose();
    message = text;
    needsUsername = usernameNeeded;
    // A Mac's account is often the SSH user. Other servers want the user name
    // empty, so it is filled in only when the server asks for one.
    if (usernameNeeded && !username) username = tunnel.user ?? "";
    password = "";
    activeLogin = { username: "", password: "", credentialId: "", save: false };
    view = "login";
  }

  function end(text: string) {
    dispose();
    message = text;
    view = "ended";
  }

  async function start() {
    const login = activeLogin;
    dispose();
    const current = attempt;
    view = "connecting";
    message = "";
    try {
      // The client loads first, so a failure there leaves no session open.
      const RFB = await loadNoVnc();
      if (current !== attempt) return;
      const result = await OpenVNC(tunnel.id, target as any, login as any);
      if (current !== attempt) {
        if (result?.sessionId) CloseVNC(result.sessionId).catch(() => {});
        return;
      }
      if (!result || result.status !== "connected") {
        showLogin(result?.message ?? "", !!result?.needsUsername);
        return;
      }
      sessionId = result.sessionId ?? "";
      apple = !!result.apple;
      reducedColor = !!result.reducedColor;
      if (tabId) void BindTabSession(tabId, "vnc", sessionId);
      // Saved once is enough; a reconnect from this window reuses the login.
      if (login.save) activeLogin = { ...login, save: false };
      connect(RFB, result.proxyAddress ?? "", current);
    } catch (e) {
      if (current === attempt) end(errorText(e));
    }
  }

  function connect(RFB: any, address: string, current: number) {
    // The app authenticated already, so the client is offered no authentication.
    const client = new RFB(screenEl, address, { wsProtocols: ["binary"] });
    client.scaleViewport = fit;
    client.resizeSession = false;
    client.focusOnClick = true;
    // The server sends the pointer's shape only once it moves, and macOS hides
    // the pointer while typing: show a dot instead of nothing.
    client.showDotCursor = true;
    client.addEventListener("connect", () => {
      if (current !== attempt) return;
      view = "connected";
      client.focus();
      // A new session has none of the local clipboard yet.
      clipboard.reset();
      void sendLocalClipboard();
    });
    client.addEventListener("disconnect", (event: any) => {
      if (current !== attempt) return;
      rfb = null;
      end(event.detail?.clean ? "The session ended." : "The connection to the VNC server was lost.");
    });
    client.addEventListener("securityfailure", (event: any) => {
      if (current !== attempt) return;
      end(`The VNC server refused the session: ${event.detail?.reason ?? "unknown reason"}`);
    });
    client.addEventListener("desktopname", (event: any) => {
      desktopName = event.detail?.name ?? "";
    });
    client.addEventListener("clipboard", (event: any) => {
      clipboard.received(event.detail?.text ?? "").catch((e) => console.warn("copying VNC clipboard failed:", e));
    });
    rfb = client;
  }

  // A tab shown again may have missed a copy made meanwhile.
  $: if (visible) void sendLocalClipboard();

  async function sendLocalClipboard() {
    if (!rfb || view !== "connected" || !visible) return;
    try {
      const text = await clipboard.local();
      if (text && rfb) rfb.clipboardPasteFrom(text);
    } catch (e) {
      console.warn("reading the clipboard failed:", e);
    }
  }

  async function toggleColor() {
    try {
      await SetVNCFullColor(tunnel.id, target as any, reducedColor);
    } catch (e) {
      // The running session stays as it is.
      showToast(errorText(e), "error");
      return;
    }
    // The colour depth is set when the session starts.
    start();
  }

  function toggleFit() {
    fit = !fit;
    if (rfb) rfb.scaleViewport = fit;
  }

  function submitLogin() {
    if (chosenLogin) {
      activeLogin = { username: "", password: "", credentialId: chosenLogin.id, save: saveLogin };
    } else {
      activeLogin = { username: username.trim(), password, credentialId: "", save: saveLogin };
    }
    password = "";
    start();
  }

  function changeLogin() {
    dispose();
    message = "";
    view = "login";
  }

  onMount(async () => {
    window.addEventListener("focus", sendLocalClipboard);
    try {
      // Every login is offered, because the Mac account used for SSH usually
      // opens its screen too.
      savedLogins = ((await ListCredentials()) ?? []).filter((c: any) => c.hasPassword) as any;
      const saved = await GetVNCLogin(tunnel.id, target as any);
      username = saved.username;
      // The tunnel's SSH login is offered first only for the host itself, so it
      // does not go to a machine behind a forward unless chosen.
      selectedLogin = saved.credentialId || (forward ? "" : await GetCredentialBinding("ssh", tunnel.id, { remoteHost: "", remotePort: 0 } as any)) || "";
    } catch (e) {
      console.warn("reading stored VNC login failed:", e);
    }
    // Try the assigned login, or none, first: a server that needs a login
    // answers with the form.
    start();
  });

  onDestroy(() => {
    window.removeEventListener("focus", sendLocalClipboard);
    dispose();
  });
</script>

<div class="vnc-overlay" data-testid="vnc">
  <div class="vnc-titlebar">
    <span class="vnc-title">vnc — {targetLabel}{desktopName ? ` (${desktopName})` : ""}</span>
    <div class="vnc-actions">
      {#if view === "connected"}
        {#if apple}
          <button class="vnc-text-btn" data-testid="vnc-color" on:click={toggleColor} title={reducedColor ? "Show every color. The Mac sends about twice as much, so the screen follows input more slowly." : "Use 16-bit color. The Mac sends about half as much, so the screen follows input faster."}>{reducedColor ? "full color" : "16-bit color"}</button>
        {/if}
        <button class="vnc-text-btn" data-testid="vnc-fit" on:click={toggleFit} title={fit ? "Show the screen at its real size" : "Fit the screen to the window"}>{fit ? "actual size" : "fit to window"}</button>
        <button class="vnc-text-btn" on:click={() => rfb?.sendCtrlAltDel()} title="Send Ctrl+Alt+Del">ctrl+alt+del</button>
        <button class="vnc-text-btn" on:click={changeLogin}>change login</button>
      {/if}
    </div>
  </div>

  <div class="vnc-body">
    <div bind:this={screenEl} class="vnc-screen" class:vnc-hidden={view !== "connected"} data-testid="vnc-screen" />

    {#if view === "connecting"}
      <div class="vnc-status">connecting...</div>
    {:else if view === "login"}
      <form class="vnc-form" on:submit|preventDefault={submitLogin}>
        <div class="vnc-form-title">screen sharing login</div>
        {#if message}<div class="vnc-error" data-testid="vnc-message">{message}</div>{/if}
        {#if savedLogins.length}
          <div class="vnc-field">
            <span>saved login</span>
            <Select
              bind:value={selectedLogin}
              testid="vnc-saved-login"
              ariaLabel="Saved login"
              options={[
                { value: "", label: "type a login" },
                ...savedLogins.map((login) => ({ value: login.id, label: `${login.name} (${login.protocols.map((protocol) => protocol.toUpperCase()).join(", ")})` })),
              ]}
            />
          </div>
        {/if}
        {#if chosenLogin}
          <div class="vnc-chosen" data-testid="vnc-chosen-login">{chosenLogin.username || "password only"}</div>
        {:else}
          <label>user <input bind:value={username} data-testid="vnc-username" autocomplete="off" spellcheck="false" required={needsUsername} placeholder={needsUsername ? "account on the Mac" : "Mac account, or empty for a VNC password"} /></label>
          <label>password <input type="password" bind:value={password} data-testid="vnc-password" autocomplete="off" required /></label>
        {/if}
        <label class="vnc-check"><input type="checkbox" data-testid="vnc-save-login" bind:checked={saveLogin} /> {chosenLogin ? "use this login here next time" : "save in credentials"}</label>
        <button class="vnc-primary" type="submit">connect</button>
      </form>
    {:else if view === "ended"}
      <div class="vnc-panel">
        <div class="vnc-status" data-testid="vnc-message">{message || "The session ended."}</div>
        <div class="vnc-panel-actions">
          <button class="vnc-primary" on:click={start}>reconnect</button>
          <button class="vnc-text-btn" on:click={changeLogin}>change login</button>
        </div>
      </div>
    {/if}
  </div>
</div>

<style>
  .vnc-overlay { flex: 1; min-height: 0; display: flex; flex-direction: column; background: var(--bg); }
  .vnc-titlebar { display: flex; justify-content: space-between; align-items: center; padding: 4px 12px; border-bottom: 1px solid var(--border); background: var(--surface); }
  .vnc-title { font-size: 12px; color: var(--accent); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .vnc-actions, .vnc-panel-actions { display: flex; gap: 6px; align-items: center; }
  .vnc-body { position: relative; flex: 1; min-height: 0; overflow: hidden; display: flex; align-items: center; justify-content: center; }
  .vnc-screen { position: absolute; inset: 0; overflow: hidden; }
  .vnc-screen.vnc-hidden { visibility: hidden; }
  .vnc-status { font-size: 12px; color: var(--accent); padding: 20px; z-index: 1; }
  .vnc-error { font-size: 12px; color: var(--error); }
  .vnc-form, .vnc-panel { z-index: 1; display: flex; flex-direction: column; gap: 10px; width: 420px; max-width: 90%; padding: 20px; border: 1px solid var(--border); background: var(--surface); color: var(--text); font-size: 12px; }
  .vnc-panel .vnc-status { padding: 0; overflow-wrap: anywhere; }
  .vnc-form-title { color: var(--accent); }
  .vnc-chosen { color: var(--text); font-size: 12px; overflow-wrap: anywhere; }
  .vnc-form label { display: flex; flex-direction: column; gap: 4px; color: var(--muted); font-size: 12px; }
  .vnc-field { display: flex; flex-direction: column; gap: 4px; color: var(--muted); font-size: 12px; }
  .vnc-field :global(.select) { display: flex; }
  .vnc-form input:not([type="checkbox"]) { background: var(--bg); border: 1px solid var(--border); color: var(--text); padding: 6px 8px; font: inherit; border-radius: 2px; }
  .vnc-form input:focus { outline: none; border-color: var(--accent); }
  .vnc-form label.vnc-check { flex-direction: row; align-items: center; gap: 6px; }
  .vnc-primary, .vnc-text-btn { background: none; border: 1px solid var(--border); color: var(--muted); cursor: pointer; border-radius: 2px; font: inherit; font-size: 12px; padding: 4px 8px; }
  .vnc-primary { color: var(--accent); border-color: var(--accent); }
  .vnc-primary:hover { background: var(--accent-soft); }
  .vnc-text-btn:hover { color: var(--accent2); border-color: var(--accent2); }
</style>
