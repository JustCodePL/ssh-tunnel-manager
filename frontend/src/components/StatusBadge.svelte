<script lang="ts">
  import type { TunnelStatus } from "../types";

  export let status: TunnelStatus = "disconnected";

  const colors: Record<TunnelStatus, string> = {
    disconnected: "bg-zinc-500",
    connecting: "bg-yellow-500",
    connected: "bg-green-500",
    reconnecting: "bg-orange-500",
    error: "bg-red-500",
  };

  const labels: Record<TunnelStatus, string> = {
    disconnected: "Disconnected",
    connecting: "Connecting…",
    connected: "Connected",
    reconnecting: "Reconnecting…",
    error: "Error",
  };

  $: dotClass = colors[status] || colors.disconnected;
  $: label = labels[status] || "Unknown";
</script>

<span class="inline-flex items-center gap-1.5 text-xs font-medium">
  <span class="h-2 w-2 rounded-full {dotClass}" />
  {label}
</span>
