<script lang="ts">
  import type { TunnelConfig, TunnelStatus } from "../types";
  import { connectTunnel, disconnectTunnel, deleteTunnel } from "../stores/tunnels";
  import StatusBadge from "./StatusBadge.svelte";
  import { createEventDispatcher } from "svelte";

  export let tunnel: TunnelConfig;
  export let status: TunnelStatus = "disconnected";

  const dispatch = createEventDispatcher<{ edit: TunnelConfig; logs: TunnelConfig }>();

  let actionLoading = false;

  async function handleConnect() {
    actionLoading = true;
    try {
      await connectTunnel(tunnel.id);
    } catch (e) {
      console.error("connect failed:", e);
    } finally {
      actionLoading = false;
    }
  }

  async function handleDisconnect() {
    actionLoading = true;
    try {
      await disconnectTunnel(tunnel.id);
    } catch (e) {
      console.error("disconnect failed:", e);
    } finally {
      actionLoading = false;
    }
  }

  async function handleDelete() {
    if (!confirm(`Delete tunnel "${tunnel.name}"?`)) return;
    await deleteTunnel(tunnel.id);
  }

  function handleEdit() {
    dispatch("edit", tunnel);
  }

  function handleLogs() {
    dispatch("logs", tunnel);
  }

  $: isActive = status === "connected" || status === "connecting" || status === "reconnecting";
</script>

<div class="group bg-zinc-50 dark:bg-zinc-800 rounded-lg p-4 border border-zinc-200 dark:border-zinc-700 hover:border-blue-400 dark:hover:border-blue-500 hover:bg-white dark:hover:bg-zinc-750 hover:shadow-sm transition-all">
  <div class="flex items-start justify-between gap-3">
    <div class="min-w-0 flex-1">
      <div class="flex items-center gap-2 mb-1">
        <h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 truncate">{tunnel.name}</h3>
        <StatusBadge {status} />
      </div>
      <p class="text-xs text-zinc-500 dark:text-zinc-400 group-hover:text-zinc-700 dark:group-hover:text-zinc-200 truncate font-mono transition-colors">
        {tunnel.user}@{tunnel.host}:{tunnel.port}
      </p>
      {#if tunnel.portForwards && tunnel.portForwards.length > 0}
        <div class="mt-2 flex flex-wrap gap-1.5">
          {#each tunnel.portForwards as pf}
            <span class="text-[11px] bg-zinc-200 dark:bg-zinc-700 text-zinc-600 dark:text-zinc-300 px-1.5 py-0.5 rounded font-mono">
              {pf.localPort} → {pf.remoteHost}:{pf.remotePort}
            </span>
          {/each}
        </div>
      {/if}
    </div>

    <div class="flex items-center gap-1.5 shrink-0">
      {#if isActive}
        <button
          class="px-2.5 py-1 text-xs rounded bg-red-600 hover:bg-red-500 text-white disabled:opacity-50"
          on:click={handleDisconnect}
          disabled={actionLoading}
        >
          Disconnect
        </button>
      {:else}
        <button
          class="px-2.5 py-1 text-xs rounded bg-emerald-600 hover:bg-emerald-500 text-white disabled:opacity-50"
          on:click={handleConnect}
          disabled={actionLoading}
        >
          Connect
        </button>
      {/if}
      <button
        class="px-2 py-1 text-xs rounded bg-zinc-200 dark:bg-zinc-700 hover:bg-zinc-300 dark:hover:bg-zinc-600 text-zinc-600 dark:text-zinc-300"
        on:click={handleLogs}
        title="View connection logs"
      >
        Logs
      </button>
      <button
        class="px-2 py-1 text-xs rounded bg-zinc-200 dark:bg-zinc-700 hover:bg-zinc-300 dark:hover:bg-zinc-600 text-zinc-600 dark:text-zinc-300"
        on:click={handleEdit}
      >
        Edit
      </button>
      <button
        class="px-2 py-1 text-xs rounded bg-zinc-200 dark:bg-zinc-700 hover:bg-zinc-300 dark:hover:bg-zinc-600 text-zinc-500 dark:text-zinc-400 hover:text-red-500 dark:hover:text-red-400"
        on:click={handleDelete}
      >
        Del
      </button>
    </div>
  </div>
</div>
