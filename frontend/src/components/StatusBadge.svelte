<script lang="ts">
  import type { TunnelStatus } from "../types";

  export let status: TunnelStatus = "disconnected";

  const styles: Record<TunnelStatus, { color: string; glow: string; label: string }> = {
    disconnected: { color: "#555555", glow: "none", label: "offline" },
    connecting:   { color: "#00d4ff", glow: "0 0 6px #00d4ff", label: "connecting" },
    connected:    { color: "#00ff88", glow: "0 0 6px #00ff88", label: "connected" },
    reconnecting: { color: "#ffaa00", glow: "0 0 6px #ffaa00", label: "reconnecting" },
    error:        { color: "#ff4444", glow: "0 0 6px #ff4444", label: "error" },
  };

  $: s = styles[status] ?? styles.disconnected;
  $: isAnimated = status === "connecting" || status === "reconnecting";
</script>

<span class="badge" class:animated={isAnimated}>
  <span class="dot" style="color: {s.color}; text-shadow: {s.glow};">●</span>
  <span class="label" style="color: {s.color};">{s.label}</span>
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: 10px;
    font-family: 'JetBrains Mono', monospace;
    letter-spacing: 0.05em;
  }

  .dot {
    font-size: 8px;
    line-height: 1;
  }

  .label {
    text-transform: uppercase;
  }

  .animated .dot {
    animation: pulse 1.2s ease-in-out infinite;
  }

  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.3; }
  }
</style>
