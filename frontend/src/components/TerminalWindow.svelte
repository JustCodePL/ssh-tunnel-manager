<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { Terminal } from "@xterm/xterm";
  import { FitAddon } from "@xterm/addon-fit";
  import { OpenTerminal, TerminalWrite, CloseTerminal, ResizeTerminal } from "../../wailsjs/go/main/App";
  import { EventsOn, EventsOff, ClipboardGetText, ClipboardSetText } from "../../wailsjs/runtime/runtime";
  import type { TunnelConfig } from "../types";
  import { createEventDispatcher } from "svelte";

  export let tunnel: TunnelConfig;
  const dispatch = createEventDispatcher<{ close: void }>();

  let termEl: HTMLDivElement;
  let term: Terminal;
  let fitAddon: FitAddon;
  let sessionId: string = "";
  let loading = true;
  let error = "";
  let minimized = false;

  onMount(async () => {
    term = new Terminal({
      theme: { background: "#0a0a0a", foreground: "#00ff88", cursor: "#00ff88" },
      fontFamily: "JetBrains Mono, monospace",
      fontSize: 13,
    });
    fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(termEl);
    fitAddon.fit();

    EventsOn("terminal:output", (e: any) => {
      if (e.sessionId === sessionId) term.write(e.data);
    });
    EventsOn("terminal:closed", (e: any) => {
      if (e.sessionId === sessionId) dispatch("close");
    });

    term.onData((data) => { if (sessionId) TerminalWrite(sessionId, data); });
    term.onResize(({ cols, rows }) => { if (sessionId) ResizeTerminal(sessionId, cols, rows); });

    term.attachCustomKeyEventHandler((ev) => {
      if (ev.type !== "keydown") return true;
      const mod = ev.ctrlKey || ev.metaKey;
      if (!mod) return true;
      const key = ev.key.toLowerCase();
      // Ctrl/Cmd+Shift+C → always copy. Ctrl/Cmd+C → copy if selection, else let SIGINT through.
      if (key === "c" && (ev.shiftKey || term.hasSelection())) {
        const sel = term.getSelection();
        if (sel) {
          ClipboardSetText(sel);
          term.clearSelection();
        }
        return false;
      }
      // Ctrl/Cmd+V or Ctrl/Cmd+Shift+V → paste
      if (key === "v") {
        ClipboardGetText().then((text) => {
          if (text && sessionId) TerminalWrite(sessionId, text);
        });
        return false;
      }
      return true;
    });

    try {
      sessionId = await OpenTerminal(tunnel.id);
      loading = false;
      // Fit again now that we have a session, to send correct initial size
      setTimeout(() => {
        fitAddon.fit();
      }, 50);
    } catch (e: any) {
      error = e.toString();
      loading = false;
    }
  });

  onDestroy(() => {
    EventsOff("terminal:output");
    EventsOff("terminal:closed");
    if (sessionId) CloseTerminal(sessionId);
    term?.dispose();
  });

  function handleClose() {
    if (sessionId) CloseTerminal(sessionId);
    dispatch("close");
  }

  function minimize() {
    minimized = true;
  }

  function restore() {
    minimized = false;
    setTimeout(() => {
      fitAddon?.fit();
      term?.focus();
    }, 50);
  }

  async function handleContextMenu(ev: MouseEvent) {
    ev.preventDefault();
    if (!term) return;
    const sel = term.getSelection();
    if (sel) {
      await ClipboardSetText(sel);
      term.clearSelection();
    } else if (sessionId) {
      const text = await ClipboardGetText();
      if (text) TerminalWrite(sessionId, text);
    }
  }
</script>

<div class="terminal-overlay" class:hidden={minimized}>
  <div class="terminal-window">
    <div class="terminal-titlebar">
      <span class="terminal-title">terminal — {tunnel.name} ({tunnel.user}@{tunnel.host})</span>
      <div class="titlebar-actions">
        <button class="title-btn minimize-btn" on:click={minimize} title="Minimize">−</button>
        <button class="close-btn" on:click={handleClose} title="Close">×</button>
      </div>
    </div>
    <div class="terminal-body">
      {#if loading}
        <div class="status-msg">connecting...</div>
      {/if}
      {#if error}
        <div class="status-msg error">{error}</div>
      {/if}
      <div bind:this={termEl} class="xterm-container" class:hidden={loading || !!error} on:contextmenu={handleContextMenu} />
    </div>
  </div>
</div>

{#if minimized}
  <button class="restore-pill" on:click={restore} title="Restore terminal">
    <span class="pill-icon">▢</span>
    <span class="pill-text">terminal — {tunnel.name}</span>
    <span class="pill-close" on:click|stopPropagation={handleClose} title="Close">×</span>
  </button>
{/if}

<style>
  .terminal-overlay { position: fixed; top: 36px; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.85); z-index: 1000; display: flex; align-items: center; justify-content: center; }
  .terminal-overlay.hidden { display: none; }
  .terminal-window { width: 90vw; height: 80vh; background: #0a0a0a; border: 1px solid #00ff88; border-radius: 2px; display: flex; flex-direction: column; }
  .terminal-titlebar { display: flex; justify-content: space-between; align-items: center; padding: 6px 12px; border-bottom: 1px solid #1a1a1a; background: #111; }
  .terminal-title { font-family: "JetBrains Mono", monospace; font-size: 11px; color: #00ff88; }
  .titlebar-actions { display: flex; gap: 4px; }
  .title-btn, .close-btn { background: none; border: 1px solid #333; color: #888; font-size: 16px; width: 24px; height: 24px; cursor: pointer; border-radius: 2px; line-height: 1; padding: 0; }
  .minimize-btn:hover { color: #00d4ff; border-color: #00d4ff; }
  .close-btn:hover { color: #ff4444; border-color: #ff4444; }
  .restore-pill { position: fixed; bottom: 12px; right: 12px; z-index: 1000; display: flex; align-items: center; gap: 8px; padding: 6px 10px; background: #0a0a0a; border: 1px solid #00ff88; border-radius: 2px; color: #00ff88; font-family: "JetBrains Mono", monospace; font-size: 11px; cursor: pointer; box-shadow: 0 0 12px rgba(0, 255, 136, 0.3); }
  .restore-pill:hover { background: #111; box-shadow: 0 0 16px rgba(0, 255, 136, 0.5); }
  .pill-icon { font-size: 13px; }
  .pill-text { max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pill-close { color: #888; padding: 0 4px; font-size: 14px; line-height: 1; }
  .pill-close:hover { color: #ff4444; }
  .terminal-body { flex: 1; overflow: hidden; padding: 8px; }
  .xterm-container { width: 100%; height: 100%; }
  .xterm-container.hidden { display: none; }
  .status-msg { font-family: "JetBrains Mono", monospace; font-size: 12px; color: #00ff88; padding: 20px; }
  .status-msg.error { color: #ff4444; }
</style>
