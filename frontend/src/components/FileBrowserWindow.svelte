<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from "svelte";
  import {
    OpenSFTP,
    CloseSFTP,
    SFTPListDir,
    SFTPDownload,
    SFTPDownloadDir,
    SFTPPickUploadFiles,
    SFTPUploadFiles,
    SFTPDelete,
    SFTPMkdir,
    SFTPRename,
    CancelSFTPTransfer,
  } from "../../wailsjs/go/main/App";
  import { EventsOn, EventsOff } from "../../wailsjs/runtime/runtime";
  import type { TunnelConfig } from "../types";
  import type { ssh } from "../../wailsjs/go/models";

  export let tunnel: TunnelConfig;
  const dispatch = createEventDispatcher<{ close: void }>();

  type FileEntry = ssh.FileEntry;

  let sessionId = "";
  let cwd = "";
  let entries: FileEntry[] = [];
  let loading = true;
  let error = "";
  let minimized = false;
  let statusMsg = "";
  // The file currently open in the editor, shown either as a side-by-side
  // pane (default) or popped out into a standalone window. The editor component
  // (and CodeMirror) is loaded lazily the first time it's needed so it doesn't
  // weigh on app startup.
  let editing: FileEntry | null = null;
  let editorWindowed = false;
  let editorMinimized = false;
  let TextEditorComp: typeof import("./TextEditor.svelte").default | null = null;
  let editorRef: { openFile: (path: string, name: string) => void } | null = null;

  async function openEditor(e: FileEntry) {
    if (e.isDir || e.isLink) return;
    if (!TextEditorComp) {
      TextEditorComp = (await import("./TextEditor.svelte")).default;
    }
    editorMinimized = false;
    if (!editing) {
      // First open: the editor loads this file on mount.
      editing = e;
      return;
    }
    if (editing.path === e.path) return; // already open; just un-minimized above
    // Editor already open with another file — delegate so it can guard any
    // unsaved edits before switching.
    editorRef?.openFile(e.path, e.name);
  }

  function handleEditorFileOpened(ev: CustomEvent<{ path: string; name: string }>) {
    // The switch target is always a row in the current listing, so keep the
    // parent's record pointed at that entry. The editor owns the displayed
    // name/path regardless, so a miss here is harmless.
    const found = entries.find((x) => x.path === ev.detail.path);
    if (found) editing = found;
  }

  interface ProgressState {
    transferId: string;
    kind: "upload" | "download";
    name: string;
    fileIndex: number;
    fileTotal: number;
    transferred: number;
    size: number;
    startedAt: number;
  }
  // Map keyed by transferId so multiple concurrent transfers can render rows
  // independently. Reassigned (not mutated) so Svelte tracks updates.
  let transfers: Record<string, ProgressState> = {};
  $: activeTransfers = Object.values(transfers);

  type ModalState =
    | { kind: "prompt"; title: string; label: string; placeholder?: string; initial?: string; submitLabel: string; validate?: (value: string) => string | null; onSubmit: (value: string) => void | Promise<void> }
    | { kind: "confirm"; title: string; message: string; items?: string[]; submitLabel: string; danger?: boolean; onSubmit: () => void | Promise<void> };
  let modal: ModalState | null = null;
  let modalInput = "";
  let modalInputEl: HTMLInputElement | null = null;

  function openPrompt(opts: Extract<ModalState, { kind: "prompt" }>) {
    modalInput = opts.initial ?? "";
    modal = opts;
    setTimeout(() => modalInputEl?.focus(), 0);
  }

  function openConfirm(opts: Extract<ModalState, { kind: "confirm" }>) {
    modal = opts;
  }

  function closeModal() {
    modal = null;
    modalInput = "";
  }

  $: modalError = (() => {
    if (!modal || modal.kind !== "prompt") return "";
    const v = modalInput.trim();
    if (!v) return "";
    return modal.validate?.(v) ?? "";
  })();

  async function submitModal() {
    if (!modal) return;
    const m = modal;
    if (m.kind === "prompt") {
      const value = modalInput.trim();
      if (!value || modalError) return;
      closeModal();
      await m.onSubmit(value);
    } else {
      closeModal();
      await m.onSubmit();
    }
  }

  function handleModalKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") closeModal();
    else if (e.key === "Enter" && modal?.kind === "prompt") submitModal();
  }

  onMount(async () => {
    EventsOn("sftp:progress", handleProgressEvent);
    try {
      const r = await OpenSFTP(tunnel.id);
      sessionId = r.sessionId;
      cwd = r.home || "/";
      await refresh();
    } catch (e: any) {
      error = e?.toString?.() ?? String(e);
      loading = false;
    }
  });

  onDestroy(() => {
    EventsOff("sftp:progress");
    if (sessionId) CloseSFTP(sessionId);
  });

  function handleProgressEvent(e: any) {
    if (!e || e.sessionId !== sessionId || !e.transferId) return;
    if (e.done) {
      if (transfers[e.transferId]) {
        const { [e.transferId]: _, ...rest } = transfers;
        transfers = rest;
      }
      return;
    }
    const prev = transfers[e.transferId];
    const startedAt = prev ? prev.startedAt : Date.now();
    transfers = {
      ...transfers,
      [e.transferId]: {
        transferId: e.transferId,
        kind: e.kind,
        name: e.name,
        fileIndex: e.fileIndex,
        fileTotal: e.fileTotal,
        transferred: e.transferred,
        size: e.size,
        startedAt,
      },
    };
  }

  function setStatus(msg: string) {
    statusMsg = msg;
    if (msg) setTimeout(() => (statusMsg = ""), 3000);
  }

  async function refresh() {
    if (!sessionId) return;
    loading = true;
    error = "";
    try {
      entries = await SFTPListDir(sessionId, cwd);
    } catch (e: any) {
      error = e?.toString?.() ?? String(e);
    } finally {
      loading = false;
    }
  }

  function parentPath(p: string): string {
    if (p === "/" || p === "") return "/";
    const trimmed = p.replace(/\/+$/, "");
    const idx = trimmed.lastIndexOf("/");
    if (idx <= 0) return "/";
    return trimmed.slice(0, idx);
  }

  function joinPath(dir: string, name: string): string {
    if (dir === "/") return "/" + name;
    return dir.replace(/\/+$/, "") + "/" + name;
  }

  async function navigate(p: string) {
    cwd = p || "/";
    await refresh();
  }

  async function openEntry(e: FileEntry) {
    if (e.isDir || e.isLink) {
      await navigate(e.path);
    } else {
      await openEditor(e);
    }
  }

  function handleEditorSaved() {
    void refresh();
  }

  function handleEditorClose() {
    editing = null;
    editorMinimized = false;
    editorWindowed = false;
  }

  function toggleEditorWindow() {
    editorWindowed = !editorWindowed;
    // Minimize only applies to the windowed editor; docking clears it.
    if (!editorWindowed) editorMinimized = false;
  }

  async function goUp() {
    await navigate(parentPath(cwd));
  }

  async function handleDownload(e: FileEntry) {
    try {
      if (e.isDir) {
        const localPath = await SFTPDownloadDir(sessionId, e.path);
        if (localPath) setStatus(`Downloaded → ${localPath}`);
      } else {
        const localPath = await SFTPDownload(sessionId, e.path);
        if (localPath) setStatus(`Downloaded → ${localPath}`);
      }
    } catch (err: any) {
      const msg = err?.toString?.() ?? String(err);
      if (/canceled|cancelled|context canceled/i.test(msg)) {
        setStatus("Download cancelled");
      } else {
        error = msg;
      }
    }
  }

  async function handleUpload() {
    let pick: { paths: string[]; conflicts: string[] };
    try {
      pick = await SFTPPickUploadFiles(sessionId, cwd);
    } catch (err: any) {
      error = err?.toString?.() ?? String(err);
      return;
    }
    if (!pick.paths || pick.paths.length === 0) return;
    if (pick.conflicts && pick.conflicts.length > 0) {
      openConfirm({
        kind: "confirm",
        title: "// overwrite existing files?",
        message: `${pick.conflicts.length} file${pick.conflicts.length === 1 ? "" : "s"} already exist${pick.conflicts.length === 1 ? "s" : ""} in this directory and will be overwritten:`,
        items: pick.conflicts,
        submitLabel: "[ overwrite ]",
        danger: true,
        onSubmit: () => runUpload(pick.paths),
      });
      return;
    }
    await runUpload(pick.paths);
  }

  async function runUpload(paths: string[]) {
    try {
      const count = await SFTPUploadFiles(sessionId, paths, cwd);
      if (count > 0) {
        setStatus(`Uploaded ${count} file${count === 1 ? "" : "s"}`);
        await refresh();
      }
    } catch (err: any) {
      const msg = err?.toString?.() ?? String(err);
      if (/canceled|cancelled|context canceled/i.test(msg)) {
        setStatus("Upload cancelled");
        await refresh();
      } else {
        error = msg;
      }
    }
  }

  async function handleCancelTransfer(transferId: string) {
    try {
      await CancelSFTPTransfer(transferId);
    } catch (err: any) {
      error = err?.toString?.() ?? String(err);
    }
  }

  function handleMkdir() {
    openPrompt({
      kind: "prompt",
      title: "// new directory",
      label: `Create in ${cwd}`,
      placeholder: "directory name",
      submitLabel: "[ create ]",
      validate: (v) => {
        if (/[\/\\]/.test(v)) return "Name cannot contain / or \\";
        if (entries.some((e) => e.name === v)) return `"${v}" already exists in this directory`;
        return null;
      },
      onSubmit: runMkdir,
    });
  }

  async function runMkdir(name: string) {
    try {
      await SFTPMkdir(sessionId, joinPath(cwd, name));
      setStatus(`Created ${name}`);
      await refresh();
    } catch (err: any) {
      error = err?.toString?.() ?? String(err);
    }
  }

  async function handleDelete(e: FileEntry) {
    const label = e.isDir ? `directory "${e.name}" and all its contents` : `file "${e.name}"`;
    if (!confirm(`Delete ${label}?`)) return;
    try {
      await SFTPDelete(sessionId, e.path);
      setStatus(`Deleted ${e.name}`);
      await refresh();
    } catch (err: any) {
      error = err?.toString?.() ?? String(err);
    }
  }

  async function handleRename(e: FileEntry) {
    const newName = prompt(`Rename "${e.name}" to:`, e.name);
    if (!newName || newName === e.name) return;
    try {
      await SFTPRename(sessionId, e.path, joinPath(cwd, newName.trim()));
      setStatus(`Renamed → ${newName}`);
      await refresh();
    } catch (err: any) {
      error = err?.toString?.() ?? String(err);
    }
  }

  function formatSize(n: number): string {
    if (n < 1024) return n + " B";
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + " K";
    if (n < 1024 * 1024 * 1024) return (n / (1024 * 1024)).toFixed(1) + " M";
    return (n / (1024 * 1024 * 1024)).toFixed(2) + " G";
  }

  function formatDate(d: any): string {
    if (!d) return "";
    try {
      const dt = new Date(d);
      return dt.toISOString().slice(0, 16).replace("T", " ");
    } catch {
      return "";
    }
  }

  function handleClose() {
    if (sessionId) CloseSFTP(sessionId);
    sessionId = "";
    dispatch("close");
  }

  function minimize() { minimized = true; }
  function restore() { minimized = false; }

  let pathInput = "";
  $: pathInput = cwd;

  async function handlePathSubmit(e: KeyboardEvent) {
    if (e.key === "Enter") {
      await navigate(pathInput.trim() || "/");
    }
  }
</script>

<div class="fb-overlay" class:hidden={minimized}>
  <div class="fb-window">
    <div class="fb-titlebar">
      <span class="fb-title">files — {tunnel.name} ({tunnel.user}@{tunnel.host})</span>
      <div class="titlebar-actions">
        <button class="title-btn" on:click={minimize} title="Minimize">−</button>
        <button class="close-btn" on:click={handleClose} title="Close">×</button>
      </div>
    </div>

    <div class="fb-toolbar">
      <button class="tb-btn" on:click={refresh} title="Refresh">↻ refresh</button>
      <input
        class="path-input"
        bind:value={pathInput}
        on:keydown={handlePathSubmit}
        spellcheck="false"
        autocomplete="off"
      />
      <button class="tb-btn upload" on:click={handleUpload} disabled={!sessionId} title="Upload files to current directory">⤴ upload</button>
      <button class="tb-btn" on:click={handleMkdir} disabled={!sessionId} title="Create new directory">+ mkdir</button>
    </div>

    {#if activeTransfers.length > 0}
      <div class="transfers-panel">
        {#each activeTransfers as p (p.transferId)}
          {@const pct = p.size > 0 ? Math.min(100, (p.transferred / p.size) * 100) : 0}
          {@const elapsed = Math.max(0.001, (Date.now() - p.startedAt) / 1000)}
          {@const rate = p.transferred / elapsed}
          <div class="progress-bar">
            <div class="progress-label">
              <span class="progress-kind">{p.kind === "upload" ? "⤴" : "⤵"}</span>
              <span class="progress-name">{p.name}</span>
              {#if p.fileTotal > 1}
                <span class="progress-counter">[{p.fileIndex}/{p.fileTotal}]</span>
              {/if}
              <span class="progress-pct">{pct.toFixed(0)}%</span>
              <span class="progress-rate">{formatSize(rate)}/s</span>
              <span class="progress-bytes">{formatSize(p.transferred)}{p.size > 0 ? ` / ${formatSize(p.size)}` : ""}</span>
              <button class="progress-cancel" on:click={() => handleCancelTransfer(p.transferId)} title="Cancel transfer">× cancel</button>
            </div>
            <div class="progress-track">
              <div class="progress-fill" style="width: {pct}%"></div>
            </div>
          </div>
        {/each}
      </div>
    {/if}
    {#if statusMsg}
      <div class="status-bar info">{statusMsg}</div>
    {/if}
    {#if error}
      <div class="status-bar error">{error}</div>
    {/if}

    <div class="fb-body">
      <div class="file-panel">
      {#if loading}
        <div class="msg">loading...</div>
      {:else if entries.length === 0 && (cwd === "/" || cwd === "")}
        <div class="msg dim">// empty directory</div>
      {:else}
        {#if entries.length === 0}
          <div class="msg dim">// empty directory</div>
        {/if}
        <table class="file-table">
          <thead>
            <tr>
              <th class="col-name">name</th>
              <th class="col-size">size</th>
              <th class="col-mtime">modified</th>
              <th class="col-actions"></th>
            </tr>
          </thead>
          <tbody>
            {#if cwd !== "/" && cwd !== ""}
              <tr class="dir parent-row">
                <td class="col-name">
                  <button class="name-btn" on:click={goUp} on:dblclick={goUp}>
                    <span class="icon">▴</span>..
                  </button>
                </td>
                <td class="col-size">—</td>
                <td class="col-mtime"></td>
                <td class="col-actions"></td>
              </tr>
            {/if}
            {#each entries as e (e.path)}
              <tr class:dir={e.isDir} class:link={e.isLink}>
                <td class="col-name">
                  {#if e.isDir || e.isLink}
                    <button class="name-btn" on:dblclick={() => openEntry(e)} on:click={() => openEntry(e)}>
                      <span class="icon">{e.isLink ? "↪" : "▸"}</span>{e.name}{e.isDir ? "/" : ""}
                    </button>
                  {:else}
                    <button class="name-btn" on:dblclick={() => openEntry(e)} on:click={() => openEntry(e)} title="Open in editor">
                      <span class="icon">·</span>{e.name}
                    </button>
                  {/if}
                </td>
                <td class="col-size">{e.isDir ? "—" : formatSize(e.size)}</td>
                <td class="col-mtime">{formatDate(e.modTime)}</td>
                <td class="col-actions">
                  {#if !e.isDir && !e.isLink}
                    <button class="act-btn" on:click={() => openEditor(e)} title="Edit as text">edit</button>
                  {/if}
                  {#if !e.isLink}
                    <button
                      class="act-btn"
                      on:click={() => handleDownload(e)}
                      title={e.isDir ? "Download as ZIP" : "Download"}
                    >{e.isDir ? "⤵ zip" : "⤵"}</button>
                  {/if}
                  <button class="act-btn" on:click={() => handleRename(e)} title="Rename">✎</button>
                  <button class="act-btn danger" on:click={() => handleDelete(e)} title="Delete">×</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
      </div>

      {#if editing && TextEditorComp}
        <div class="editor-pane" class:windowed={editorWindowed}>
          <svelte:component
            this={TextEditorComp}
            bind:this={editorRef}
            {sessionId}
            path={editing.path}
            name={editing.name}
            windowed={editorWindowed}
            minimized={editorMinimized}
            on:saved={handleEditorSaved}
            on:close={handleEditorClose}
            on:togglewindow={toggleEditorWindow}
            on:minimize={() => (editorMinimized = true)}
            on:restore={() => (editorMinimized = false)}
            on:fileopened={handleEditorFileOpened}
          />
        </div>
      {/if}
    </div>
  </div>
</div>

{#if modal}
  <div class="modal-backdrop" on:click|self={closeModal} on:keydown={handleModalKeydown} role="presentation">
    <div class="modal-panel" class:danger={modal.kind === "confirm" && modal.danger}>
      <div class="modal-header">
        <span class="modal-title">{modal.title}</span>
      </div>
      <div class="modal-body">
        {#if modal.kind === "prompt"}
          <div class="modal-label">{modal.label}</div>
          <input
            class="modal-input"
            class:invalid={!!modalError}
            bind:value={modalInput}
            bind:this={modalInputEl}
            on:keydown={handleModalKeydown}
            placeholder={modal.placeholder ?? ""}
            spellcheck="false"
            autocomplete="off"
          />
          {#if modalError}
            <div class="modal-validation">{modalError}</div>
          {/if}
        {:else}
          <div class="modal-message">{modal.message}</div>
          {#if modal.items && modal.items.length > 0}
            <ul class="modal-items">
              {#each modal.items as item}
                <li>{item}</li>
              {/each}
            </ul>
          {/if}
        {/if}
        <div class="modal-actions">
          <button
            class="modal-btn accent"
            class:danger={modal.kind === "confirm" && modal.danger}
            on:click={submitModal}
            disabled={modal.kind === "prompt" && (!modalInput.trim() || !!modalError)}
          >{modal.submitLabel}</button>
          <button class="modal-btn" on:click={closeModal}>[ cancel ]</button>
        </div>
      </div>
    </div>
  </div>
{/if}

{#if minimized}
  <button class="restore-pill" on:click={restore} title="Restore file browser">
    <span class="pill-icon">▢</span>
    <span class="pill-text">files — {tunnel.name}</span>
    <span class="pill-close" on:click|stopPropagation={handleClose} title="Close">×</span>
  </button>
{/if}

<style>
  .fb-overlay { position: fixed; top: 36px; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.85); z-index: 999; display: flex; align-items: center; justify-content: center; }
  .fb-overlay.hidden { display: none; }
  .fb-window { width: 90vw; height: 80vh; background: #0a0a0a; border: 1px solid #00d4ff; border-radius: 2px; display: flex; flex-direction: column; }
  .fb-titlebar { display: flex; justify-content: space-between; align-items: center; padding: 6px 12px; border-bottom: 1px solid #1a1a1a; background: #111; }
  .fb-title { font-family: "JetBrains Mono", monospace; font-size: 11px; color: #00d4ff; }
  .titlebar-actions { display: flex; gap: 4px; }
  .title-btn, .close-btn { background: none; border: 1px solid #333; color: #888; font-size: 16px; width: 24px; height: 24px; cursor: pointer; border-radius: 2px; line-height: 1; padding: 0; }
  .title-btn:hover { color: #00d4ff; border-color: #00d4ff; }
  .close-btn:hover { color: #ff4444; border-color: #ff4444; }

  .fb-toolbar { display: flex; align-items: center; gap: 6px; padding: 6px 10px; background: #0d0d0d; border-bottom: 1px solid #1a1a1a; }
  .tb-btn { background: transparent; border: 1px solid #333; color: #888; font-family: "JetBrains Mono", monospace; font-size: 10px; padding: 3px 8px; cursor: pointer; border-radius: 2px; }
  .tb-btn:hover:not(:disabled) { color: #00d4ff; border-color: #00d4ff; }
  .tb-btn:disabled { opacity: 0.35; cursor: default; }
  .tb-btn.upload { color: #00ff88; border-color: #00ff88; }
  .tb-btn.upload:hover:not(:disabled) { background: rgba(0,255,136,0.1); }

  .path-input { flex: 1; background: #050505; border: 1px solid #222; color: #00d4ff; font-family: "JetBrains Mono", monospace; font-size: 11px; padding: 3px 8px; border-radius: 2px; outline: none; min-width: 0; }
  .path-input:focus { border-color: #00d4ff; }

  .status-bar { font-family: "JetBrains Mono", monospace; font-size: 10px; padding: 4px 10px; border-bottom: 1px solid #1a1a1a; }
  .status-bar.info { color: #00ff88; background: rgba(0,255,136,0.05); }
  .status-bar.error { color: #ff4444; background: rgba(255,68,68,0.05); }

  .transfers-panel { max-height: 30vh; overflow-y: auto; border-bottom: 1px solid #1a1a1a; }
  .transfers-panel .progress-bar:last-child { border-bottom: none; }
  .progress-bar { padding: 4px 10px 6px; background: rgba(0, 212, 255, 0.05); border-bottom: 1px solid #1a1a1a; font-family: "JetBrains Mono", monospace; font-size: 10px; color: #ccc; }
  .progress-label { display: flex; align-items: center; gap: 10px; margin-bottom: 4px; }
  .progress-kind { color: #00d4ff; font-size: 12px; }
  .progress-name { color: #ccc; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; min-width: 0; }
  .progress-counter { color: #666; }
  .progress-pct { color: #00d4ff; font-weight: 600; min-width: 36px; text-align: right; }
  .progress-rate { color: #888; min-width: 70px; text-align: right; }
  .progress-bytes { color: #666; min-width: 110px; text-align: right; }
  .progress-cancel { background: transparent; border: 1px solid #ff4444; color: #ff4444; font-family: "JetBrains Mono", monospace; font-size: 9px; padding: 1px 6px; cursor: pointer; border-radius: 2px; }
  .progress-cancel:hover { background: rgba(255, 68, 68, 0.1); }
  .progress-track { height: 4px; background: #1a1a1a; border-radius: 2px; overflow: hidden; }
  .progress-fill { height: 100%; background: linear-gradient(90deg, #00d4ff, #00ff88); transition: width 100ms linear; }

  .fb-body { flex: 1; display: flex; flex-direction: row; min-height: 0; }
  .file-panel { flex: 1 1 0; overflow: auto; min-width: 0; }
  .editor-pane { flex: 0 0 55%; min-width: 0; display: flex; }
  /* When the editor is popped out into its own window, the pane must not
     reserve split space; the editor positions itself as a fixed overlay. */
  .editor-pane.windowed { display: contents; }
  .msg { font-family: "JetBrains Mono", monospace; font-size: 11px; color: #00d4ff; padding: 20px; }
  .msg.dim { color: #666; }

  .file-table { width: 100%; border-collapse: collapse; font-family: "JetBrains Mono", monospace; font-size: 11px; color: #ccc; }
  .file-table th { text-align: left; padding: 6px 10px; background: #0d0d0d; color: #666; font-weight: 400; text-transform: lowercase; border-bottom: 1px solid #1a1a1a; position: sticky; top: 0; z-index: 1; }
  .file-table td { padding: 4px 10px; border-bottom: 1px solid #111; }
  .file-table tr:hover td { background: #111; }
  .file-table tr.dir td.col-name { color: #00d4ff; }
  .file-table tr.link td.col-name { color: #ffaa00; }
  .file-table tr.parent-row td { color: #888; border-bottom: 1px solid #1a1a1a; }
  .file-table tr.parent-row td.col-name { color: #888; }

  .col-name { width: auto; }
  .col-size { width: 90px; color: #888; text-align: right; }
  .col-mtime { width: 150px; color: #888; }
  .col-actions { width: 160px; text-align: right; white-space: nowrap; }

  .name-btn { background: none; border: none; color: inherit; font: inherit; cursor: pointer; padding: 0; text-align: left; }
  .name-btn:hover { color: #00ff88; text-decoration: underline; }
  .icon { display: inline-block; width: 14px; color: #666; }
  tr.dir .icon { color: #00d4ff; }

  .act-btn { background: transparent; border: 1px solid #333; color: #888; font-family: "JetBrains Mono", monospace; font-size: 10px; padding: 1px 6px; cursor: pointer; border-radius: 2px; margin-left: 2px; }
  .act-btn:hover:not(:disabled) { color: #00d4ff; border-color: #00d4ff; }
  .act-btn.danger:hover:not(:disabled) { color: #ff4444; border-color: #ff4444; }
  .act-btn:disabled { opacity: 0.35; cursor: default; }

  .modal-backdrop { position: fixed; inset: 0; background: rgba(0,0,0,0.7); z-index: 1100; display: flex; align-items: center; justify-content: center; }
  .modal-panel { background: #0a0a0a; border: 1px solid #00d4ff; border-radius: 2px; width: 100%; max-width: 460px; margin: 0 16px; box-shadow: 0 0 40px rgba(0,0,0,0.6); }
  .modal-panel.danger { border-color: #ff4444; box-shadow: 0 0 40px rgba(255,68,68,0.2); }
  .modal-header { padding: 10px 14px; border-bottom: 1px solid #1a1a1a; }
  .modal-title { font-size: 11px; color: #00d4ff; font-family: "JetBrains Mono", monospace; letter-spacing: 0.05em; }
  .modal-panel.danger .modal-title { color: #ff4444; }
  .modal-body { padding: 14px; display: flex; flex-direction: column; gap: 10px; }
  .modal-label { font-size: 10px; color: #888; font-family: "JetBrains Mono", monospace; }
  .modal-message { font-size: 11px; color: #ccc; font-family: "JetBrains Mono", monospace; line-height: 1.5; }
  .modal-items { list-style: none; margin: 0; padding: 6px 10px; background: #050505; border: 1px solid #222; border-radius: 2px; max-height: 200px; overflow-y: auto; font-family: "JetBrains Mono", monospace; font-size: 10px; color: #ffaa00; }
  .modal-items li { padding: 2px 0; }
  .modal-input { width: 100%; background: #050505; border: 1px solid #222; color: #ccc; font-family: "JetBrains Mono", monospace; font-size: 12px; padding: 7px 10px; border-radius: 2px; outline: none; box-sizing: border-box; }
  .modal-input:focus { border-color: #00d4ff; }
  .modal-input.invalid { border-color: #ff4444; }
  .modal-input.invalid:focus { border-color: #ff4444; }
  .modal-validation { font-size: 10px; color: #ff4444; font-family: "JetBrains Mono", monospace; }
  .modal-actions { display: flex; gap: 6px; margin-top: 6px; }
  .modal-btn { background: transparent; border: 1px solid #333; color: #888; font-family: "JetBrains Mono", monospace; font-size: 10px; padding: 5px 12px; cursor: pointer; border-radius: 2px; letter-spacing: 0.05em; }
  .modal-btn:hover:not(:disabled) { color: #ccc; border-color: #888; }
  .modal-btn.accent { color: #00d4ff; border-color: #00d4ff; }
  .modal-btn.accent:hover:not(:disabled) { background: rgba(0,212,255,0.1); }
  .modal-btn.accent.danger { color: #ff4444; border-color: #ff4444; }
  .modal-btn.accent.danger:hover:not(:disabled) { background: rgba(255,68,68,0.1); }
  .modal-btn:disabled { opacity: 0.4; cursor: default; }

  .restore-pill { position: fixed; bottom: 12px; right: 220px; z-index: 1000; display: flex; align-items: center; gap: 8px; padding: 6px 10px; background: #0a0a0a; border: 1px solid #00d4ff; border-radius: 2px; color: #00d4ff; font-family: "JetBrains Mono", monospace; font-size: 11px; cursor: pointer; box-shadow: 0 0 12px rgba(0, 212, 255, 0.3); }
  .restore-pill:hover { background: #111; box-shadow: 0 0 16px rgba(0, 212, 255, 0.5); }
  .pill-icon { font-size: 13px; }
  .pill-text { max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pill-close { color: #888; padding: 0 4px; font-size: 14px; line-height: 1; }
  .pill-close:hover { color: #ff4444; }
</style>
