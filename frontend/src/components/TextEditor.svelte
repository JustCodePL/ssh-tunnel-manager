<script lang="ts">
  import { errorText } from "../lib/runtime";
  import { onMount, onDestroy, tick, createEventDispatcher } from "svelte";
  import { EditorView, keymap } from "@codemirror/view";
  import { EditorState, Compartment } from "@codemirror/state";
  import { basicSetup } from "codemirror";
  import { SFTPReadText, SFTPWriteText } from "../../bindings/ssh-tunnel-manager/app";
  import { languageForName } from "../lib/cmLanguages";

  export let sessionId: string;
  export let path: string;
  export let name: string;

  const dispatch = createEventDispatcher<{
    close: void;
    saved: void;
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
      ".cm-content": { fontFamily: '"JetBrains Mono", monospace', caretColor: "var(--accent)" },
      ".cm-gutters": { backgroundColor: "#0a0a0a", color: "#555", border: "none" },
      ".cm-activeLine": { backgroundColor: "rgba(0,212,255,0.05)" },
      ".cm-activeLineGutter": { backgroundColor: "rgba(0,212,255,0.08)", color: "#888" },
      "&.cm-focused .cm-cursor": { borderLeftColor: "var(--accent)" },
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
      const r: any = await SFTPReadText(sessionId, curPath, force);
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
      error = errorText(e);
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
      const res: any = await SFTPWriteText(sessionId, curPath, currentDoc(), force ? 0 : modTimeMs);
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
      error = errorText(e);
    } finally {
      saving = false;
    }
  }

  async function reloadFromRemote() {
    prompt = null;
    try {
      const r: any = await SFTPReadText(sessionId, curPath, true);
      modTimeMs = r.modTime ? new Date(r.modTime).getTime() : 0;
      if (view) {
        view.dispatch({
          changes: { from: 0, to: view.state.doc.length, insert: r.content },
        });
      }
      dirty = false;
      setStatus("reloaded from server");
    } catch (e: any) {
      error = errorText(e);
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
    if (e.key === "Escape" && !prompt) {
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
<div class="ed-root">
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
            <button class="ed-btn" on:click={() => load(true)}>Open as text</button>
            <button class="ed-btn" on:click={requestClose}>Cancel</button>
          </div>
        </div>
      {/if}
      <!-- Editor host stays mounted; hidden while loading / force-gating. -->
      <div class="cm-host" class:hidden={loading || !!needsForce} bind:this={editorEl}></div>
    </div>
  </div>
</div>


{#if prompt === "conflict"}
  <div class="modal-backdrop" role="presentation">
    <div class="modal-panel danger">
      <div class="modal-header"><span class="modal-title">File changed on server</span></div>
      <div class="modal-body">
        <div class="modal-message">
          "{curName}" was modified on the server since you opened it. Overwriting will discard those remote changes.
        </div>
        <div class="modal-actions">
          <button class="modal-btn accent danger" on:click={() => { prompt = null; save(true); }}>Overwrite</button>
          <button class="modal-btn" on:click={reloadFromRemote}>Reload</button>
          <button class="modal-btn" on:click={() => (prompt = null)}>Cancel</button>
        </div>
      </div>
    </div>
  </div>
{:else if prompt === "discard"}
  <div class="modal-backdrop" role="presentation">
    <div class="modal-panel danger">
      <div class="modal-header"><span class="modal-title">Unsaved changes</span></div>
      <div class="modal-body">
        <div class="modal-message">Discard unsaved changes to "{curName}"?</div>
        <div class="modal-actions">
          <button class="modal-btn accent danger" on:click={() => { prompt = null; dispatch("close"); }}>Discard</button>
          <button class="modal-btn" on:click={() => { prompt = null; save(false); }}>Save</button>
          <button class="modal-btn" on:click={() => (prompt = null)}>Cancel</button>
        </div>
      </div>
    </div>
  </div>
{:else if prompt === "switch"}
  <div class="modal-backdrop" role="presentation">
    <div class="modal-panel danger">
      <div class="modal-header"><span class="modal-title">Unsaved changes</span></div>
      <div class="modal-body">
        <div class="modal-message">
          "{curName}" has unsaved changes. Open "{pendingName}" anyway?
        </div>
        <div class="modal-actions">
          <button class="modal-btn accent danger" on:click={doSwitch}>Discard &amp; open</button>
          <button class="modal-btn" on:click={switchSave}>Save &amp; open</button>
          <button class="modal-btn" on:click={cancelSwitch}>Cancel</button>
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  /* Docked: dissolve the root so .ed-window becomes the pane's flex child. */
  .ed-root { display: contents; }
  .ed-window { display: flex; flex-direction: column; background: var(--bg); min-width: 0; }
  .ed-root .ed-window { flex: 1; height: 100%; border-left: 1px solid var(--border); }

  /* Windowed: a centered overlay matching the file browser / terminal. Starts
     below the title bar so the main window titlebar stays grabbable. */
  .ed-root.hidden { display: none; }

  .ed-titlebar { display: flex; justify-content: space-between; align-items: center; padding: 6px 12px; border-bottom: 1px solid var(--border); background: var(--surface); }
  .ed-title { font-family: var(--ui); font-size: 12px; color: var(--accent2); display: flex; align-items: center; gap: 8px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ed-icon { color: var(--accent); }
  .ed-mode { color: var(--muted); font-size: 11.5px; }
  .titlebar-actions { display: flex; gap: 4px; align-items: center; }
  .title-btn { background: none; border: 1px solid var(--border); color: var(--muted); font-family: var(--ui); font-size: 12px; min-width: 24px; height: 24px; cursor: pointer; border-radius: 2px; line-height: 1; padding: 0 6px; }
  .title-btn:hover:not(:disabled) { color: var(--accent2); border-color: var(--accent2); }
  .title-btn:disabled { opacity: 0.35; cursor: default; }
  .title-btn.save { color: var(--accent); border-color: var(--accent); }
  .title-btn.save:hover:not(:disabled) { background: var(--accent-soft); }
  .close-btn { background: none; border: 1px solid var(--border); color: var(--muted); font-size: 16px; width: 24px; height: 24px; cursor: pointer; border-radius: 2px; line-height: 1; padding: 0; }
  .close-btn:hover { color: var(--error); border-color: var(--error); }

  .status-bar { font-family: var(--ui); font-size: 11.5px; padding: 4px 10px; border-bottom: 1px solid var(--border); }
  .status-bar.info { color: var(--accent); background: var(--accent-soft); }
  .status-bar.error { color: var(--error); background: color-mix(in srgb, var(--error) 14%, transparent); }

  .ed-body { flex: 1; position: relative; overflow: hidden; }
  .msg { font-family: var(--ui); font-size: 12px; color: var(--accent2); padding: 20px; }
  .cm-host { height: 100%; overflow: hidden; }
  .cm-host.hidden { display: none; }

  .force-panel { display: flex; flex-direction: column; gap: 10px; padding: 30px; font-family: var(--ui); }
  .force-msg { color: var(--warning); font-size: 12px; }
  .force-sub { color: var(--muted); font-size: 12px; }
  .force-buttons { display: flex; gap: 6px; margin-top: 6px; }
  .ed-btn { background: transparent; border: 1px solid var(--border); color: var(--muted); font-family: var(--ui); font-size: 11.5px; padding: 3px 8px; cursor: pointer; border-radius: 2px; }
  .ed-btn:hover { color: var(--accent2); border-color: var(--accent2); }

  .restore-pill { position: fixed; bottom: 12px; right: 220px; z-index: 1000; display: flex; align-items: center; gap: 8px; padding: 6px 10px; background: var(--bg); border: 1px solid var(--accent); border-radius: 2px; color: var(--accent); font-family: var(--ui); font-size: 12px; cursor: pointer; box-shadow: 0 0 12px var(--accent-soft); }
  .restore-pill:hover { background: var(--surface); box-shadow: 0 0 16px var(--accent-soft); }
  .pill-icon { font-size: 13px; }
  .pill-text { max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pill-close { color: var(--muted); padding: 0 4px; font-size: 14px; line-height: 1; }
  .pill-close:hover { color: var(--error); }

  .modal-backdrop { position: fixed; top: var(--titlebar-height); left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.7); z-index: 1100; display: flex; align-items: center; justify-content: center; }
  .modal-panel { background: var(--bg); border: 1px solid var(--accent2); border-radius: 2px; width: 100%; max-width: 460px; margin: 0 16px; box-shadow: 0 0 40px rgba(0,0,0,0.6); }
  .modal-panel.danger { border-color: var(--error); box-shadow: 0 0 40px color-mix(in srgb, var(--error) 14%, transparent); }
  .modal-header { padding: 10px 14px; border-bottom: 1px solid var(--border); }
  .modal-title { font-size: 12px; color: var(--error); font-family: var(--ui); letter-spacing: 0.05em; }
  .modal-body { padding: 14px; display: flex; flex-direction: column; gap: 10px; }
  .modal-message { font-size: 12px; color: var(--text); font-family: var(--ui); line-height: 1.5; }
  .modal-actions { display: flex; gap: 6px; margin-top: 6px; }
  .modal-btn { background: transparent; border: 1px solid var(--border); color: var(--muted); font-family: var(--ui); font-size: 11.5px; padding: 5px 12px; cursor: pointer; border-radius: 2px; letter-spacing: 0.05em; }
  .modal-btn:hover:not(:disabled) { color: var(--text); border-color: var(--muted); }
  .modal-btn.accent { color: var(--accent2); border-color: var(--accent2); }
  .modal-btn.accent.danger { color: var(--error); border-color: var(--error); }
  .modal-btn.accent.danger:hover:not(:disabled) { background: color-mix(in srgb, var(--error) 14%, transparent); }
</style>
