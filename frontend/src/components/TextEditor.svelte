<script lang="ts">
  import { onMount, onDestroy, tick, createEventDispatcher } from "svelte";
  import { EditorView, keymap } from "@codemirror/view";
  import { EditorState, Compartment } from "@codemirror/state";
  import { basicSetup } from "codemirror";
  import { SFTPReadText, SFTPWriteText } from "../../wailsjs/go/main/App";
  import { languageForName } from "../lib/cmLanguages";

  export let sessionId: string;
  export let path: string;
  export let name: string;
  // windowed: detached into a standalone overlay window (like the file
  // browser / terminal); otherwise docked as a side-by-side pane.
  export let windowed = false;
  export let minimized = false;

  const dispatch = createEventDispatcher<{
    close: void;
    saved: void;
    togglewindow: void;
    minimize: void;
    restore: void;
    fileopened: { path: string; name: string };
  }>();

  // The file actually loaded in the editor. Kept internally (rather than read
  // straight from the props) so switching files goes through openFile() and
  // can guard unsaved changes; the props only seed the first load.
  let curPath = path;
  let curName = name;
  // A file the user asked to switch to while unsaved edits are pending.
  let pendingPath: string | null = null;
  let pendingName = "";

  let loading = true;
  let error = "";
  let saving = false;
  let dirty = false;
  let statusMsg = "";
  let mode = "";
  let modTimeMs = 0;
  // Set when the backend refused to open the file as text without force.
  let needsForce: { reason: "binary" | "tooLarge"; size: number } | null = null;
  // Reuse one modal slot for the conflict, discard, and switch prompts.
  let prompt: "conflict" | "discard" | "switch" | null = null;

  let editorEl: HTMLDivElement;
  let view: EditorView | null = null;
  const langComp = new Compartment();

  const theme = EditorView.theme(
    {
      "&": { backgroundColor: "#050505", color: "#ccc", height: "100%", fontSize: "12px" },
      ".cm-content": { fontFamily: '"JetBrains Mono", monospace', caretColor: "#00ff88" },
      ".cm-gutters": { backgroundColor: "#0a0a0a", color: "#555", border: "none" },
      ".cm-activeLine": { backgroundColor: "rgba(0,212,255,0.05)" },
      ".cm-activeLineGutter": { backgroundColor: "rgba(0,212,255,0.08)", color: "#888" },
      "&.cm-focused .cm-cursor": { borderLeftColor: "#00ff88" },
      ".cm-selectionBackground, &.cm-focused .cm-selectionBackground, ::selection": {
        backgroundColor: "rgba(0,212,255,0.25)",
      },
      ".cm-scroller": { fontFamily: '"JetBrains Mono", monospace' },
    },
    { dark: true },
  );

  onMount(() => {
    void load(false);
  });

  onDestroy(() => {
    view?.destroy();
    view = null;
  });

  function setStatus(msg: string) {
    statusMsg = msg;
    if (msg) setTimeout(() => (statusMsg = ""), 3000);
  }

  async function load(force: boolean) {
    loading = true;
    error = "";
    try {
      const r = await SFTPReadText(sessionId, curPath, force);
      if (!force && (r.binary || r.tooLarge)) {
        needsForce = { reason: r.binary ? "binary" : "tooLarge", size: r.size };
        loading = false;
        return;
      }
      needsForce = null;
      modTimeMs = r.modTime ? new Date(r.modTime).getTime() : 0;
      mode = r.mode;
      loading = false;
      await tick();
      initEditor(r.content);
    } catch (e: any) {
      error = e?.toString?.() ?? String(e);
      loading = false;
    }
  }

  function initEditor(content: string) {
    view?.destroy();
    const state = EditorState.create({
      doc: content,
      extensions: [
        basicSetup,
        theme,
        langComp.of([]),
        keymap.of([
          {
            key: "Mod-s",
            preventDefault: true,
            run: () => {
              void save(false);
              return true;
            },
          },
        ]),
        EditorView.updateListener.of((u) => {
          if (u.docChanged) dirty = true;
        }),
      ],
    });
    view = new EditorView({ state, parent: editorEl });
    void languageForName(curName).then((ext) => {
      if (ext && view) view.dispatch({ effects: langComp.reconfigure(ext) });
    });
  }

  // openFile is called by the parent when the user picks another file while
  // this editor is already open. Unsaved edits trigger a confirm prompt first.
  export function openFile(p: string, n: string) {
    if (p === curPath) return;
    pendingPath = p;
    pendingName = n;
    if (dirty) {
      prompt = "switch";
    } else {
      void doSwitch();
    }
  }

  async function doSwitch() {
    if (!pendingPath) return;
    prompt = null;
    curPath = pendingPath;
    curName = pendingName;
    pendingPath = null;
    dirty = false;
    statusMsg = "";
    dispatch("fileopened", { path: curPath, name: curName });
    await load(false);
  }

  // From the switch prompt: save the current file, then switch only if the
  // save actually succeeded (no conflict / error left pending).
  async function switchSave() {
    prompt = null;
    await save(false);
    if (prompt === null && !dirty && pendingPath) void doSwitch();
  }

  function cancelSwitch() {
    prompt = null;
    pendingPath = null;
    pendingName = "";
  }

  function currentDoc(): string {
    return view ? view.state.doc.toString() : "";
  }

  async function save(force: boolean) {
    if (!view || saving) return;
    saving = true;
    error = "";
    try {
      const res = await SFTPWriteText(sessionId, curPath, currentDoc(), force ? 0 : modTimeMs);
      if (res.conflict) {
        prompt = "conflict";
        saving = false;
        return;
      }
      modTimeMs = res.modTime ? new Date(res.modTime).getTime() : Date.now();
      dirty = false;
      setStatus("saved");
      dispatch("saved");
    } catch (e: any) {
      error = e?.toString?.() ?? String(e);
    } finally {
      saving = false;
    }
  }

  async function reloadFromRemote() {
    prompt = null;
    try {
      const r = await SFTPReadText(sessionId, curPath, true);
      modTimeMs = r.modTime ? new Date(r.modTime).getTime() : 0;
      if (view) {
        view.dispatch({
          changes: { from: 0, to: view.state.doc.length, insert: r.content },
        });
      }
      dirty = false;
      setStatus("reloaded from server");
    } catch (e: any) {
      error = e?.toString?.() ?? String(e);
    }
  }

  function requestClose() {
    if (dirty) {
      prompt = "discard";
      return;
    }
    dispatch("close");
  }

  function handleKeydown(e: KeyboardEvent) {
    // Esc dismisses the editor only when it's a standalone window (modal-like);
    // a docked pane stays put so Esc inside the editor isn't surprising.
    if (e.key === "Escape" && windowed && !prompt && !minimized) {
      e.preventDefault();
      requestClose();
    }
  }
</script>

<svelte:window on:keydown={handleKeydown} />

<!-- One stable DOM subtree: in docked mode .ed-root is display:contents so
     .ed-window flows into the parent pane; in windowed mode .ed-root becomes a
     centered overlay. The CodeMirror host stays mounted across the switch so
     edits survive popping the editor in and out. -->
<div class="ed-root" class:windowed class:hidden={minimized}>
  <div class="ed-window">
    <div class="ed-titlebar">
      <span class="ed-title">
        <span class="ed-icon">✎</span>{curName}{dirty ? " *" : ""}
        {#if mode}<span class="ed-mode">{mode}</span>{/if}
      </span>
      <div class="titlebar-actions">
        <button class="title-btn save" on:click={() => save(false)} disabled={saving || loading || !view} title="Save (Ctrl+S)">
          {saving ? "…" : "save"}
        </button>
        <button class="title-btn" on:click={() => dispatch("togglewindow")} title={windowed ? "Dock to file browser" : "Open in window"}>
          {windowed ? "⇲" : "⇱"}
        </button>
        {#if windowed}
          <button class="title-btn" on:click={() => dispatch("minimize")} title="Minimize">−</button>
        {/if}
        <button class="close-btn" on:click={requestClose} title="Close (Esc)">×</button>
      </div>
    </div>

    {#if statusMsg}
      <div class="status-bar info">{statusMsg}</div>
    {/if}
    {#if error}
      <div class="status-bar error">{error}</div>
    {/if}

    <div class="ed-body">
      {#if loading}
        <div class="msg">loading…</div>
      {:else if needsForce}
        <div class="force-panel">
          <div class="force-msg">
            {#if needsForce.reason === "binary"}
              // this file looks binary (contains non-text bytes).
            {:else}
              // this file is large ({(needsForce.size / (1024 * 1024)).toFixed(1)} MB).
            {/if}
          </div>
          <div class="force-sub">Open it as text anyway?</div>
          <div class="force-buttons">
            <button class="ed-btn" on:click={() => load(true)}>[ open as text ]</button>
            <button class="ed-btn" on:click={requestClose}>[ cancel ]</button>
          </div>
        </div>
      {/if}
      <!-- Editor host stays mounted; hidden while loading / force-gating. -->
      <div class="cm-host" class:hidden={loading || !!needsForce} bind:this={editorEl}></div>
    </div>
  </div>
</div>

{#if windowed && minimized}
  <button class="restore-pill" on:click={() => dispatch("restore")} title="Restore editor">
    <span class="pill-icon">✎</span>
    <span class="pill-text">edit — {curName}{dirty ? " *" : ""}</span>
    <span class="pill-close" on:click|stopPropagation={requestClose} title="Close">×</span>
  </button>
{/if}

{#if prompt === "conflict"}
  <div class="modal-backdrop" role="presentation">
    <div class="modal-panel danger">
      <div class="modal-header"><span class="modal-title">// file changed on server</span></div>
      <div class="modal-body">
        <div class="modal-message">
          "{curName}" was modified on the server since you opened it. Overwriting will discard those remote changes.
        </div>
        <div class="modal-actions">
          <button class="modal-btn accent danger" on:click={() => { prompt = null; save(true); }}>[ overwrite ]</button>
          <button class="modal-btn" on:click={reloadFromRemote}>[ reload ]</button>
          <button class="modal-btn" on:click={() => (prompt = null)}>[ cancel ]</button>
        </div>
      </div>
    </div>
  </div>
{:else if prompt === "discard"}
  <div class="modal-backdrop" role="presentation">
    <div class="modal-panel danger">
      <div class="modal-header"><span class="modal-title">// unsaved changes</span></div>
      <div class="modal-body">
        <div class="modal-message">Discard unsaved changes to "{curName}"?</div>
        <div class="modal-actions">
          <button class="modal-btn accent danger" on:click={() => { prompt = null; dispatch("close"); }}>[ discard ]</button>
          <button class="modal-btn" on:click={() => { prompt = null; save(false); }}>[ save ]</button>
          <button class="modal-btn" on:click={() => (prompt = null)}>[ cancel ]</button>
        </div>
      </div>
    </div>
  </div>
{:else if prompt === "switch"}
  <div class="modal-backdrop" role="presentation">
    <div class="modal-panel danger">
      <div class="modal-header"><span class="modal-title">// unsaved changes</span></div>
      <div class="modal-body">
        <div class="modal-message">
          "{curName}" has unsaved changes. Open "{pendingName}" anyway?
        </div>
        <div class="modal-actions">
          <button class="modal-btn accent danger" on:click={doSwitch}>[ discard &amp; open ]</button>
          <button class="modal-btn" on:click={switchSave}>[ save &amp; open ]</button>
          <button class="modal-btn" on:click={cancelSwitch}>[ cancel ]</button>
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  /* Docked: dissolve the root so .ed-window becomes the pane's flex child. */
  .ed-root { display: contents; }
  .ed-window { display: flex; flex-direction: column; background: #0a0a0a; min-width: 0; }
  .ed-root:not(.windowed) .ed-window { flex: 1; height: 100%; border-left: 1px solid #1a1a1a; }

  /* Windowed: a centered overlay matching the file browser / terminal. Starts
     at top:36px so the main window titlebar stays grabbable. */
  .ed-root.windowed { position: fixed; top: 36px; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.85); z-index: 1000; display: flex; align-items: center; justify-content: center; }
  .ed-root.windowed .ed-window { width: 86vw; height: 82vh; border: 1px solid #00d4ff; border-radius: 2px; box-shadow: 0 0 40px rgba(0,0,0,0.6); }
  .ed-root.hidden { display: none; }

  .ed-titlebar { display: flex; justify-content: space-between; align-items: center; padding: 6px 12px; border-bottom: 1px solid #1a1a1a; background: #111; }
  .ed-title { font-family: "JetBrains Mono", monospace; font-size: 11px; color: #00d4ff; display: flex; align-items: center; gap: 8px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ed-icon { color: #00ff88; }
  .ed-mode { color: #555; font-size: 10px; }
  .titlebar-actions { display: flex; gap: 4px; align-items: center; }
  .title-btn { background: none; border: 1px solid #333; color: #888; font-family: "JetBrains Mono", monospace; font-size: 11px; min-width: 24px; height: 24px; cursor: pointer; border-radius: 2px; line-height: 1; padding: 0 6px; }
  .title-btn:hover:not(:disabled) { color: #00d4ff; border-color: #00d4ff; }
  .title-btn:disabled { opacity: 0.35; cursor: default; }
  .title-btn.save { color: #00ff88; border-color: #00ff88; }
  .title-btn.save:hover:not(:disabled) { background: rgba(0,255,136,0.1); }
  .close-btn { background: none; border: 1px solid #333; color: #888; font-size: 16px; width: 24px; height: 24px; cursor: pointer; border-radius: 2px; line-height: 1; padding: 0; }
  .close-btn:hover { color: #ff4444; border-color: #ff4444; }

  .status-bar { font-family: "JetBrains Mono", monospace; font-size: 10px; padding: 4px 10px; border-bottom: 1px solid #1a1a1a; }
  .status-bar.info { color: #00ff88; background: rgba(0,255,136,0.05); }
  .status-bar.error { color: #ff4444; background: rgba(255,68,68,0.05); }

  .ed-body { flex: 1; position: relative; overflow: hidden; }
  .msg { font-family: "JetBrains Mono", monospace; font-size: 11px; color: #00d4ff; padding: 20px; }
  .cm-host { height: 100%; overflow: hidden; }
  .cm-host.hidden { display: none; }

  .force-panel { display: flex; flex-direction: column; gap: 10px; padding: 30px; font-family: "JetBrains Mono", monospace; }
  .force-msg { color: #ffaa00; font-size: 12px; }
  .force-sub { color: #888; font-size: 11px; }
  .force-buttons { display: flex; gap: 6px; margin-top: 6px; }
  .ed-btn { background: transparent; border: 1px solid #333; color: #888; font-family: "JetBrains Mono", monospace; font-size: 10px; padding: 3px 8px; cursor: pointer; border-radius: 2px; }
  .ed-btn:hover { color: #00d4ff; border-color: #00d4ff; }

  .restore-pill { position: fixed; bottom: 12px; right: 220px; z-index: 1000; display: flex; align-items: center; gap: 8px; padding: 6px 10px; background: #0a0a0a; border: 1px solid #00ff88; border-radius: 2px; color: #00ff88; font-family: "JetBrains Mono", monospace; font-size: 11px; cursor: pointer; box-shadow: 0 0 12px rgba(0, 255, 136, 0.3); }
  .restore-pill:hover { background: #111; box-shadow: 0 0 16px rgba(0, 255, 136, 0.5); }
  .pill-icon { font-size: 13px; }
  .pill-text { max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pill-close { color: #888; padding: 0 4px; font-size: 14px; line-height: 1; }
  .pill-close:hover { color: #ff4444; }

  .modal-backdrop { position: fixed; top: 36px; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.7); z-index: 1100; display: flex; align-items: center; justify-content: center; }
  .modal-panel { background: #0a0a0a; border: 1px solid #00d4ff; border-radius: 2px; width: 100%; max-width: 460px; margin: 0 16px; box-shadow: 0 0 40px rgba(0,0,0,0.6); }
  .modal-panel.danger { border-color: #ff4444; box-shadow: 0 0 40px rgba(255,68,68,0.2); }
  .modal-header { padding: 10px 14px; border-bottom: 1px solid #1a1a1a; }
  .modal-title { font-size: 11px; color: #ff4444; font-family: "JetBrains Mono", monospace; letter-spacing: 0.05em; }
  .modal-body { padding: 14px; display: flex; flex-direction: column; gap: 10px; }
  .modal-message { font-size: 11px; color: #ccc; font-family: "JetBrains Mono", monospace; line-height: 1.5; }
  .modal-actions { display: flex; gap: 6px; margin-top: 6px; }
  .modal-btn { background: transparent; border: 1px solid #333; color: #888; font-family: "JetBrains Mono", monospace; font-size: 10px; padding: 5px 12px; cursor: pointer; border-radius: 2px; letter-spacing: 0.05em; }
  .modal-btn:hover:not(:disabled) { color: #ccc; border-color: #888; }
  .modal-btn.accent { color: #00d4ff; border-color: #00d4ff; }
  .modal-btn.accent.danger { color: #ff4444; border-color: #ff4444; }
  .modal-btn.accent.danger:hover:not(:disabled) { background: rgba(255,68,68,0.1); }
</style>
