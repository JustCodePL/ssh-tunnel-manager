<script lang="ts">
  import type { TunnelConfig, PortForward } from "../types";
  import { addTunnel, updateTunnel } from "../stores/tunnels";
  import { createEventDispatcher } from "svelte";

  export let tunnel: TunnelConfig | null = null;

  const dispatch = createEventDispatcher<{ close: void }>();

  let name = tunnel?.name ?? "";
  let host = tunnel?.host ?? "";
  let port = tunnel?.port ?? 22;
  let user = tunnel?.user ?? "";
  let keyPath = tunnel?.keyPath ?? "";
  let group = tunnel?.group ?? "";
  let autoConnect = tunnel?.autoConnect ?? false;
  let tunnelColor = tunnel?.color ?? "";
  let proxyCommand = tunnel?.proxyCommand ?? "";
  let proxyJump = tunnel?.proxyJump ?? "";

  const presetColors = [
    { name: "red", hex: "#ef4444" },
    { name: "orange", hex: "#f97316" },
    { name: "yellow", hex: "#eab308" },
    { name: "green", hex: "#22c55e" },
    { name: "cyan", hex: "#06b6d4" },
    { name: "blue", hex: "#3b82f6" },
    { name: "purple", hex: "#a855f7" },
    { name: "pink", hex: "#ec4899" },
  ];
  let portForwards: PortForward[] = tunnel?.portForwards?.length
    ? [...tunnel.portForwards]
    : [];
  let showAdvanced = !!(tunnel?.proxyCommand || tunnel?.proxyJump);

  let saving = false;
  let error = "";

  $: isEdit = tunnel !== null;

  function addPortForward() {
    portForwards = [...portForwards, { localPort: 0, remoteHost: "127.0.0.1", remotePort: 0, description: "" }];
  }

  function removePortForward(index: number) {
    portForwards = portForwards.filter((_, i) => i !== index);
  }

  async function handleSubmit() {
    error = "";
    const trimmedName = name.trim();
    if (!trimmedName) {
      error = "Name is required.";
      return;
    }
    if (/\s/.test(trimmedName)) {
      error = "Name cannot contain spaces (used as SSH Host alias).";
      return;
    }
    const validForwards = portForwards.filter(
      (pf) => pf.localPort > 0 && pf.remotePort > 0
    );

    saving = true;
    try {
      const cfg: TunnelConfig = {
        id: tunnel?.id ?? trimmedName,
        name: trimmedName,
        host: host.trim(),
        port,
        user: user.trim(),
        keyPath: keyPath.trim(),
        portForwards: validForwards,
        proxyCommand: proxyCommand.trim() || undefined,
        proxyJump: proxyJump.trim() || undefined,
        color: tunnelColor || undefined,
        group: group.trim(),
        autoConnect,
      };
      if (isEdit) {
        await updateTunnel(cfg);
      } else {
        await addTunnel(cfg);
      }
      dispatch("close");
    } catch (e: any) {
      error = e?.message ?? String(e);
    } finally {
      saving = false;
    }
  }
</script>

<div class="bg-zinc-50 dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 rounded-lg p-5">
  <h2 class="text-base font-semibold mb-4">
    {isEdit ? "Edit Tunnel" : "Add Tunnel"}
  </h2>

  {#if error}
    <div class="mb-3 text-sm text-red-400 bg-red-900/30 border border-red-800 rounded px-3 py-2">
      {error}
    </div>
  {/if}

  <form on:submit|preventDefault={handleSubmit} class="space-y-3">
    <div class="grid grid-cols-2 gap-3">
      <label class="block">
        <span class="text-xs text-zinc-500 dark:text-zinc-400">Name</span>
        <input
          type="text"
          bind:value={name}
          class="mt-1 block w-full rounded bg-white dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-600 text-sm px-2.5 py-1.5 text-zinc-900 dark:text-zinc-100 focus:border-blue-500 focus:outline-none"
          placeholder="Production DB"
        />
      </label>
      <label class="block">
        <span class="text-xs text-zinc-500 dark:text-zinc-400">Group</span>
        <input
          type="text"
          bind:value={group}
          class="mt-1 block w-full rounded bg-white dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-600 text-sm px-2.5 py-1.5 text-zinc-900 dark:text-zinc-100 focus:border-blue-500 focus:outline-none"
          placeholder="production"
        />
      </label>
    </div>

    <div>
      <span class="text-xs text-zinc-500 dark:text-zinc-400">Color</span>
      <div class="flex items-center gap-1.5 mt-1">
        <button
          type="button"
          class="w-6 h-6 rounded-full border-2 {tunnelColor === '' ? 'border-blue-500' : 'border-zinc-600'} bg-zinc-700"
          title="No color"
          on:click={() => (tunnelColor = "")}
        >
          {#if tunnelColor === ""}
            <span class="block w-full h-full flex items-center justify-center text-zinc-400 text-xs leading-none">&times;</span>
          {/if}
        </button>
        {#each presetColors as c}
          <button
            type="button"
            class="w-6 h-6 rounded-full border-2 {tunnelColor === c.hex ? 'border-white' : 'border-zinc-600'}"
            style="background-color: {c.hex}"
            title={c.name}
            on:click={() => (tunnelColor = c.hex)}
          />
        {/each}
      </div>
    </div>

    <div class="grid grid-cols-3 gap-3">
      <label class="block col-span-2">
        <span class="text-xs text-zinc-500 dark:text-zinc-400">Host</span>
        <input
          type="text"
          bind:value={host}
          class="mt-1 block w-full rounded bg-white dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-600 text-sm px-2.5 py-1.5 text-zinc-900 dark:text-zinc-100 focus:border-blue-500 focus:outline-none"
          placeholder="bastion.example.com"
        />
      </label>
      <label class="block">
        <span class="text-xs text-zinc-500 dark:text-zinc-400">Port</span>
        <input
          type="number"
          bind:value={port}
          class="mt-1 block w-full rounded bg-white dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-600 text-sm px-2.5 py-1.5 text-zinc-900 dark:text-zinc-100 focus:border-blue-500 focus:outline-none"
          min="1"
          max="65535"
        />
      </label>
    </div>

    <div class="grid grid-cols-2 gap-3">
      <label class="block">
        <span class="text-xs text-zinc-500 dark:text-zinc-400">User</span>
        <input
          type="text"
          bind:value={user}
          class="mt-1 block w-full rounded bg-white dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-600 text-sm px-2.5 py-1.5 text-zinc-900 dark:text-zinc-100 focus:border-blue-500 focus:outline-none"
          placeholder="deploy"
        />
      </label>
      <label class="block">
        <span class="text-xs text-zinc-500 dark:text-zinc-400">SSH Key Path</span>
        <input
          type="text"
          bind:value={keyPath}
          class="mt-1 block w-full rounded bg-white dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-600 text-sm px-2.5 py-1.5 text-zinc-900 dark:text-zinc-100 focus:border-blue-500 focus:outline-none"
          placeholder="~/.ssh/id_ed25519"
        />
      </label>
    </div>

    <div>
      <div class="flex items-center justify-between mb-2">
        <span class="text-xs text-zinc-500 dark:text-zinc-400">Port Forwards</span>
        <button
          type="button"
          class="text-xs text-blue-400 hover:text-blue-300"
          on:click={addPortForward}
        >
          + Add
        </button>
      </div>
      {#each portForwards as pf, i}
        <div class="mb-2 space-y-1">
          <div class="flex items-center gap-2">
            <input
              type="number"
              bind:value={pf.localPort}
              class="w-24 rounded bg-white dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-600 text-sm px-2 py-1.5 text-zinc-900 dark:text-zinc-100 focus:border-blue-500 focus:outline-none"
              placeholder="Local"
              min="1"
              max="65535"
            />
            <span class="text-zinc-500 text-xs">→</span>
            <input
              type="text"
              bind:value={pf.remoteHost}
              class="flex-1 rounded bg-white dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-600 text-sm px-2 py-1.5 text-zinc-900 dark:text-zinc-100 focus:border-blue-500 focus:outline-none"
              placeholder="127.0.0.1"
            />
            <span class="text-zinc-500 text-xs">:</span>
            <input
              type="number"
              bind:value={pf.remotePort}
              class="w-24 rounded bg-white dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-600 text-sm px-2 py-1.5 text-zinc-900 dark:text-zinc-100 focus:border-blue-500 focus:outline-none"
              placeholder="Remote"
              min="1"
              max="65535"
            />
            <button
              type="button"
              class="text-zinc-500 hover:text-red-400 text-sm px-1"
              on:click={() => removePortForward(i)}
            >
              ×
            </button>
          </div>
          <input
            type="text"
            bind:value={pf.description}
            class="w-full rounded bg-white dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-600 text-sm px-2 py-1 text-zinc-500 dark:text-zinc-400 focus:border-blue-500 focus:outline-none"
            placeholder="Description (e.g., web server, database)"
          />
        </div>
      {/each}
    </div>

    <div>
      <button
        type="button"
        class="text-xs text-zinc-500 dark:text-zinc-400 hover:text-zinc-300 mb-2"
        on:click={() => (showAdvanced = !showAdvanced)}
      >
        {showAdvanced ? "▾" : "▸"} Advanced (Proxy)
      </button>
      {#if showAdvanced}
        <div class="space-y-2">
          <label class="block">
            <span class="text-xs text-zinc-500 dark:text-zinc-400">ProxyCommand</span>
            <input
              type="text"
              bind:value={proxyCommand}
              class="mt-1 block w-full rounded bg-white dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-600 text-sm px-2.5 py-1.5 text-zinc-900 dark:text-zinc-100 focus:border-blue-500 focus:outline-none font-mono"
              placeholder="aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p"
            />
          </label>
          <label class="block">
            <span class="text-xs text-zinc-500 dark:text-zinc-400">ProxyJump</span>
            <input
              type="text"
              bind:value={proxyJump}
              class="mt-1 block w-full rounded bg-white dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-600 text-sm px-2.5 py-1.5 text-zinc-900 dark:text-zinc-100 focus:border-blue-500 focus:outline-none"
              placeholder="bastion.example.com"
            />
          </label>
          <p class="text-xs text-zinc-500">
            ProxyCommand tokens: %h (host), %p (port), %r (user). Only set one of ProxyCommand or ProxyJump.
          </p>
        </div>
      {/if}
    </div>

    <label class="flex items-center gap-2 cursor-pointer">
      <input type="checkbox" bind:checked={autoConnect} class="rounded" />
      <span class="text-xs text-zinc-500 dark:text-zinc-400">Auto-connect on startup</span>
    </label>

    <div class="flex justify-end gap-2 pt-2">
      <button
        type="button"
        class="px-3 py-1.5 text-sm rounded bg-zinc-200 dark:bg-zinc-700 hover:bg-zinc-300 dark:hover:bg-zinc-600 text-zinc-700 dark:text-zinc-300"
        on:click={() => dispatch("close")}
      >
        Cancel
      </button>
      <button
        type="submit"
        class="px-3 py-1.5 text-sm rounded bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50"
        disabled={saving}
      >
        {saving ? "Saving…" : isEdit ? "Update" : "Add"}
      </button>
    </div>
  </form>
</div>
