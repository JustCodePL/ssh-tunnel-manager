// Shared formatting helpers for human-readable sizes.

/** formatBytes renders a byte count as a compact human-readable string. */
export function formatBytes(n: number): string {
  if (!isFinite(n) || n <= 0) return "0 B";
  if (n < 1024) return n + " B";
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + " K";
  if (n < 1024 * 1024 * 1024) return (n / (1024 * 1024)).toFixed(1) + " M";
  if (n < 1024 * 1024 * 1024 * 1024) return (n / (1024 * 1024 * 1024)).toFixed(1) + " G";
  return (n / (1024 * 1024 * 1024 * 1024)).toFixed(2) + " T";
}
