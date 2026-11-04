<script lang="ts">
  import { errorText } from "../lib/runtime";
  import { onMount, onDestroy } from "svelte";
  import { Terminal } from "@xterm/xterm";
  import { FitAddon } from "@xterm/addon-fit";
  import { PingEvents, OpenForwardTerminal, OpenTerminal, OpenCommandTerminal, BindTabSession, TerminalWrite, CloseTerminal, ResizeTerminal } from "../../bindings/ssh-tunnel-manager/app";
  import { createEventScope, ClipboardGetText, ClipboardSetText, waitForEvents } from "../lib/runtime";
  import type { PortForward, TunnelConfig } from "../types";
  import { createEventDispatcher } from "svelte";
  const events = createEventScope();

  export let tunnel: TunnelConfig;
  export let tabId = "";
  export let visible = true;
  // When set, run this command in the PTY (e.g. "htop") instead of a login
  // shell. label overrides the window/pill title prefix ("terminal" by default).
  export let command: string = "";
  export let label: string = "terminal";
  // The SSH forward to open a shell on. Without one, the host itself.
  export let forward: PortForward | null = null;
  $: targetLabel = forward ? `${tunnel.name} → ${forward.sshUser || tunnel.user}@${forward.remoteHost}:${forward.remotePort}` : `${tunnel.name} (${tunnel.user}@${tunnel.host})`;
  const dispatch = createEventDispatcher<{ ended: void }>();

  let termEl: HTMLDivElement;
  let term: Terminal;
  let fitAddon: FitAddon;
  let sessionId: string = "";
  let loading = true;
  let error = "";
  // Output that arrives before OpenTerminal has returned the session's ID.
  let early: { sessionId: string; data: string }[] = [];
  let resizeObserver: ResizeObserver | null = null;
  let fitTimer: ReturnType<typeof setTimeout> | null = null;

  function terminalTheme() {
    const css = getComputedStyle(document.documentElement);
    const graphite = document.documentElement.dataset.theme === "graphite";
    return {
      background: css.getPropertyValue("--bg").trim(),
      foreground: graphite ? css.getPropertyValue("--text").trim() : css.getPropertyValue("--accent").trim(),
      cursor: css.getPropertyValue("--accent").trim(),
    };
  }

  $: if (visible && term) {
    scheduleFit();
    term.focus();
  }

  // Each write is its own call to the backend, and calls can overtake each other,
  // which would swap fast-typed characters. One goes at a time.
  let writes: Promise<unknown> = Promise.resolve();
  function send(data: string) {
    const id = sessionId;
    writes = writes.then(() => TerminalWrite(id, data)).catch((e) => console.error("terminal write failed:", e));
  }

  function scheduleFit() {
    if (fitTimer) clearTimeout(fitTimer);
    fitTimer = setTimeout(() => {
      try { fitAddon?.fit(); } catch {}
    }, 30);
  }

  onMount(async () => {
    term = new Terminal({
      theme: terminalTheme(),
      fontFamily: "JetBrains Mono, ui-monospace, Menlo, monospace",
      fontSize: 13,
    });
    fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(termEl);
    fitAddon.fit();

    events.on("terminal:output", (e: any) => {
      if (e.sessionId === sessionId) term.write(e.data);
      else if (!sessionId) early.push(e);
    });
    events.on("terminal:closed", (e: any) => {
      if (e.sessionId === sessionId) dispatch("ended");
    });

    term.onData((data) => { if (sessionId) send(data); });
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
          if (text && sessionId) send(text);
        });
        return false;
      }
      return true;
    });

    try {
      await waitForEvents(PingEvents);
      sessionId = forward
        ? await OpenForwardTerminal(tunnel.id, { remoteHost: forward.remoteHost, remotePort: forward.remotePort } as any)
        : command
          ? await OpenCommandTerminal(tunnel.id, command)
          : await OpenTerminal(tunnel.id);
      loading = false;
      if (tabId) void BindTabSession(tabId, "terminal", sessionId);
      for (const entry of early) {
        if (entry.sessionId === sessionId) term.write(entry.data);
      }
      early = [];
      // Fit again now that we have a session, to send correct initial size
      setTimeout(() => {
        fitAddon.fit();
      }, 50);
    } catch (e: any) {
      error = errorText(e);
      loading = false;
    }

    // Auto-refit xterm when the container (or window) resizes.
    if (termEl && typeof ResizeObserver !== "undefined") {
      resizeObserver = new ResizeObserver(() => {
        if (visible) scheduleFit();
      });
      resizeObserver.observe(termEl);
    }
  });

  onDestroy(() => {
    events.off();
    if (sessionId) CloseTerminal(sessionId);
    resizeObserver?.disconnect();
    if (fitTimer) clearTimeout(fitTimer);
    term?.dispose();
  });

  async function handleContextMenu(ev: MouseEvent) {
    ev.preventDefault();
    if (!term) return;
    const sel = term.getSelection();
    if (sel) {
      await ClipboardSetText(sel);
      term.clearSelection();
    } else if (sessionId) {
      const text = await ClipboardGetText();
      if (text) send(text);
    }
  }
</script>

<div class="terminal" data-testid="terminal">
  {#if loading}<p class="tool-message">Opening a session on {tunnel.name}</p>{/if}
  {#if error}<p class="tool-message error">{error}</p>{/if}
  <!-- svelte-ignore a11y-no-static-element-interactions -->
  <div class="term-host" bind:this={termEl} on:contextmenu={handleContextMenu}></div>
</div>

<style>
  .terminal {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    padding: 6px 8px;
    background: var(--bg);
  }
  /* The fit addon measures this box and ignores its padding, so padding here
     would push the last line out of the window. */
  .term-host {
    flex: 1;
    min-height: 0;
    min-width: 0;
    overflow: hidden;
  }
</style>
