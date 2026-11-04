<script lang="ts">
  import { errorText } from "../lib/runtime";
  import { onMount, onDestroy } from "svelte";
  import {
    OpenForwardSFTP,
    OpenSFTP, BindTabSession,
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
  } from "../../bindings/ssh-tunnel-manager/app";
  import { createEventScope } from "../lib/runtime";
  import { breadcrumbs } from "../lib/toolTables.js";
  import { openTool } from "../lib/openTool";
  import { askConfirm } from "../stores/confirm";
  import Icon from "./Icon.svelte";
  import type { TunnelConfig, PortForward } from "../types";
  import type * as ssh from "../../bindings/ssh-tunnel-manager/internal/ssh/models";
  const events = createEventScope();
  let filter = "";
  $: visibleEntries = entries.filter((e) => !filter.trim() || e.name.toLowerCase().includes(filter.trim().toLowerCase()));
  $: crumbs = breadcrumbs(cwd || "/");
  let editingPath = false;

  export let tunnel: TunnelConfig;
  export let tabId = "";
  // The SSH forward to browse. Without one, the host itself.
  export let forward: PortForward | null = null;
  $: targetLabel = forward ? `${tunnel.name} → ${forward.sshUser || tunnel.user}@${forward.remoteHost}:${forward.remotePort}` : `${tunnel.name} (${tunnel.user}@${tunnel.host})`;

  type FileEntry = ssh.FileEntry;

  let sessionId = "";
  let cwd = "";
  let entries: FileEntry[] = [];
  let loading = true;
  let error = "";
  let statusMsg = "";
  // The file currently open in the editor, shown either as a side-by-side
  // pane (default) or popped out into a standalone window. The editor component
  // (and CodeMirror) is loaded lazily the first time it's needed so it doesn't

  function openEditor(e: FileEntry) {
    if (e.isDir || e.isLink) return;
    void openTool({
      tool: "editor",
      tunnelId: tunnel.id,
      remoteHost: forward?.remoteHost,
      remotePort: forward?.remotePort,
      path: e.path,
    });
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
    events.on("sftp:progress", handleProgressEvent);
    events.on("files:changed", () => void refresh());
    try {
      const r: any = forward
        ? await OpenForwardSFTP(tunnel.id, { remoteHost: forward.remoteHost, remotePort: forward.remotePort } as any)
        : await OpenSFTP(tunnel.id);
      sessionId = r.sessionId;
      if (tabId) void BindTabSession(tabId, "sftp", sessionId);
      cwd = r.home || "/";
      await refresh();
    } catch (e: any) {
      error = errorText(e);
      loading = false;
    }
  });

  onDestroy(() => {
    events.off();
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
      error = errorText(e);
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
      const msg = errorText(err);
      if (/canceled|cancelled|context canceled/i.test(msg)) {
        setStatus("Download cancelled");
      } else {
        error = msg;
      }
    }
  }

  async function handleUpload() {
    let pick: any;
    try {
      pick = await SFTPPickUploadFiles(sessionId, cwd);
    } catch (err: any) {
      error = errorText(err);
      return;
    }
    if (!pick.paths || pick.paths.length === 0) return;
    if (pick.conflicts && pick.conflicts.length > 0) {
      openConfirm({
        kind: "confirm",
        title: "Overwrite existing files?",
        message: `${pick.conflicts.length} file${pick.conflicts.length === 1 ? "" : "s"} already exist${pick.conflicts.length === 1 ? "s" : ""} in this directory and will be overwritten:`,
        items: pick.conflicts,
        submitLabel: "Overwrite",
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
      const msg = errorText(err);
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
      error = errorText(err);
    }
  }

  function handleMkdir() {
    openPrompt({
      kind: "prompt",
      title: "New directory",
      label: `Create in ${cwd}`,
      placeholder: "directory name",
      submitLabel: "Create",
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
      error = errorText(err);
    }
  }

  async function handleDelete(e: FileEntry) {
    const label = e.isDir ? `directory "${e.name}" and all its contents` : `file "${e.name}"`;
    if (!(await askConfirm({ title: "Delete", message: `Delete ${label}?`, confirmLabel: "Delete", danger: true }))) return;
    try {
      await SFTPDelete(sessionId, e.path);
      setStatus(`Deleted ${e.name}`);
      await refresh();
    } catch (err: any) {
      error = errorText(err);
    }
  }

  function handleRename(e: FileEntry) {
    openPrompt({
      kind: "prompt",
      title: "Rename",
      label: `Rename "${e.name}" to`,
      initial: e.name,
      submitLabel: "Rename",
      validate: (v) => {
        if (/[\/\\]/.test(v)) return "Name cannot contain / or \\";
        if (v !== e.name && entries.some((x) => x.name === v)) return `"${v}" already exists in this directory`;
        return null;
      },
      onSubmit: (name: string) => runRename(e, name),
    });
  }

  async function runRename(e: FileEntry, newName: string) {
    if (!newName || newName === e.name) return;
    try {
      await SFTPRename(sessionId, e.path, joinPath(cwd, newName.trim()));
      setStatus(`Renamed → ${newName}`);
      await refresh();
    } catch (err: any) {
      error = errorText(err);
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



  let pathInput = "";
  $: pathInput = cwd;

  async function handlePathSubmit(e: KeyboardEvent) {
    if (e.key === "Enter") {
      await navigate(pathInput.trim() || "/");
    }
  }
</script>

<div class="tool-root" data-testid="files">
  <div class="tool-bar">
    <button type="button" class="icon-btn" aria-label="Refresh" on:click={refresh}><Icon name="refresh" /></button>
    <button type="button" class="icon-btn" aria-label="Parent folder" disabled={cwd === "/" || !cwd} on:click={goUp}><Icon name="up" /></button>
    {#if editingPath}
      <!-- svelte-ignore a11y-autofocus -->
      <input
        class="field-control grow path"
        autofocus
        bind:value={pathInput}
        spellcheck="false"
        autocomplete="off"
        aria-label="Path"
        on:blur={() => (editingPath = false)}
        on:keydown={(e) => {
          if (e.key === "Escape") editingPath = false;
          else void handlePathSubmit(e);
        }}
      />
    {:else}
      <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
      <div class="crumbs grow" title="Click empty space to type a path" on:click|self={() => (editingPath = true)}>
        {#each crumbs as crumb, i}
          {#if i > 1}<span class="sep">/</span>{/if}
          <button type="button" on:click={() => navigate(crumb.path)}>{crumb.label}</button>
        {/each}
      </div>
    {/if}
    <input class="field-control filter" placeholder="Filter" aria-label="Filter files" bind:value={filter} />
    <button type="button" class="btn primary" disabled={!sessionId} title="Upload files to this folder" on:click={handleUpload}><Icon name="upload" />Upload</button>
    <button type="button" class="btn" disabled={!sessionId} on:click={handleMkdir}>New folder</button>
  </div>

  {#if statusMsg}<div class="notice">{statusMsg}</div>{/if}
  {#if error}<div class="notice error">{error}</div>{/if}

  <div class="tool-body">
    {#if loading}
      <p class="tool-message">Loading</p>
    {:else if !visibleEntries.length && !error}
      <p class="tool-message">{filter ? "No files match this filter." : "This folder is empty."}</p>
    {/if}
    {#if !loading && visibleEntries.length}
      <table class="data-table" data-testid="files-table">
        <thead>
          <tr><th class="col-name">Name</th><th class="num">Size</th><th>Modified</th><th></th></tr>
        </thead>
        <tbody>
          {#each visibleEntries as e (e.path)}
            <tr class:dir={e.isDir} data-testid="file-row" data-name={e.name}>
              <td class="col-name">
                <button type="button" class="entry" title={e.isDir || e.isLink ? "Open" : "Open in the editor"} on:click={() => (e.isDir || e.isLink ? openEntry(e) : openEditor(e))}>
                  <Icon name={e.isDir ? "files" : "file"} />{e.name}{#if e.isLink}<small>link</small>{/if}
                </button>
              </td>
              <td class="num">{e.isDir ? "" : formatSize(e.size)}</td>
              <td class="dim">{formatDate(e.modTime)}</td>
              <td>
                <div class="actions">
                  {#if !e.isDir && !e.isLink}
                    <button type="button" class="btn small" on:click={() => openEditor(e)}>Edit</button>
                  {/if}
                  {#if !e.isLink}
                    <button type="button" class="icon-btn" title={e.isDir ? "Download as ZIP" : "Download"} aria-label={`Download ${e.name}`} on:click={() => handleDownload(e)}><Icon name="download" /></button>
                  {/if}
                  <button type="button" class="icon-btn" title="Rename" aria-label={`Rename ${e.name}`} on:click={() => handleRename(e)}><Icon name="pen" /></button>
                  <button type="button" class="icon-btn" title="Delete" aria-label={`Delete ${e.name}`} on:click={() => handleDelete(e)}><Icon name="trash" /></button>
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>

  {#each activeTransfers as p (p.transferId)}
    {@const pct = p.size > 0 ? Math.min(100, (p.transferred / p.size) * 100) : 0}
    {@const elapsed = Math.max(0.001, (Date.now() - p.startedAt) / 1000)}
    <div class="transfer" data-testid="transfer">
      <i class="transfer-bar" style={`width:${pct}%`}></i>
      <b>{p.kind === "upload" ? "Uploading" : "Downloading"} {p.name}</b>
      {#if p.fileTotal > 1}<span>{p.fileIndex} of {p.fileTotal}</span>{/if}
      <span>{pct.toFixed(0)}%</span>
      <span>{formatSize(p.transferred / elapsed)}/s</span>
      <span>{formatSize(p.transferred)}{p.size > 0 ? ` / ${formatSize(p.size)}` : ""}</span>
      <button type="button" class="btn small" on:click={() => handleCancelTransfer(p.transferId)}>Cancel</button>
    </div>
  {/each}
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
            class="btn primary"
            class:danger={modal.kind === "confirm" && modal.danger}
            on:click={submitModal}
            disabled={modal.kind === "prompt" && (!modalInput.trim() || !!modalError)}
          >{modal.submitLabel}</button>
          <button class="btn" on:click={closeModal}>Cancel</button>
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  .path {
    font-family: var(--mono);
    font-size: 12px;
  }
  .crumbs {
    display: flex;
    align-items: center;
    flex-wrap: nowrap;
    overflow: hidden;
    padding: 4px 8px;
    font: 12px var(--mono);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    cursor: text;
  }
  .crumbs button {
    padding: 0 2px;
    white-space: nowrap;
    cursor: pointer;
  }
  .crumbs button:hover {
    color: var(--accent2);
  }
  .sep {
    color: var(--muted);
    padding: 0 2px;
  }
  .filter {
    width: 120px;
  }
  .notice {
    padding: 5px 10px;
    font-size: 12px;
    color: var(--muted);
    border-bottom: 1px solid var(--border);
  }
  .notice.error {
    color: var(--error);
  }
  .col-name {
    width: 46%;
  }
  .entry {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    text-align: left;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .entry :global(svg) {
    color: var(--muted);
    flex: none;
  }
  .dir .entry :global(svg) {
    color: var(--accent2);
  }
  .entry small {
    color: var(--muted);
    font-size: 12px;
  }
  th.num {
    text-align: right;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: 2px;
    opacity: 0;
  }
  tr:hover .actions,
  .actions:focus-within {
    opacity: 1;
  }
  .actions .icon-btn {
    width: 22px;
    height: 22px;
  }
  .transfer {
    position: relative;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 7px 10px;
    font-size: 12px;
    color: var(--muted);
    background: var(--surface);
    border-top: 1px solid var(--border);
    flex-shrink: 0;
  }
  .transfer b {
    font-weight: 600;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .transfer button {
    margin-left: auto;
  }
  .transfer-bar {
    position: absolute;
    top: -1px;
    left: 0;
    height: 2px;
    background: var(--accent);
  }
  .modal-backdrop {
    position: fixed;
    inset: 0;
    z-index: 70;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.6);
  }
  .modal-panel {
    width: min(420px, calc(100vw - 32px));
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }
  .modal-panel.danger {
    border-color: var(--error);
  }
  .modal-header {
    padding: 10px 14px;
    font-weight: 600;
    border-bottom: 1px solid var(--border);
  }
  .modal-body {
    padding: 12px 14px;
  }
  .modal-label {
    margin-bottom: 6px;
    color: var(--muted);
    font-size: 12px;
  }
  .modal-input {
    width: 100%;
    padding: 5px 8px;
    color: var(--text);
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .modal-input.invalid {
    border-color: var(--error);
  }
  .modal-validation {
    margin-top: 4px;
    font-size: 12px;
    color: var(--error);
  }
  .modal-message {
    word-break: break-word;
  }
  .modal-items {
    margin: 8px 0 0;
    padding-left: 18px;
    font-family: var(--mono);
    font-size: 12px;
  }
  .modal-actions {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
    margin-top: 12px;
  }
</style>
