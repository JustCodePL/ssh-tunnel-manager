/**
 * Decides what a tool tab shows for its tunnel's connection.
 * @param {string} status tunnel status
 * @param {boolean} needsTunnel whether the tool runs over the tunnel's connection
 * @param {boolean} wasConnected whether the tab has already been connected
 * @param {string} mode "resume", "keep" or "close"
 * @returns {"live" | "connecting" | "reconnecting" | "lost"}
 */
export function toolState(status, needsTunnel, wasConnected, mode) {
  if (!needsTunnel) return "live";
  if (status === "connected") return "live";
  if (status === "reconnecting") return "reconnecting";
  if (status === "connecting") return wasConnected ? "reconnecting" : "connecting";
  if (!wasConnected && status !== "error") return "connecting";
  return "lost";
}

/**
 * A resuming tab restarts its tool when the connection comes back. Keep mode
 * waits for the user's click.
 * @param {string} previous the tool state before the change
 * @param {string} next the tool state after the change
 * @param {string} mode
 * @returns {boolean}
 */
export function shouldRestart(previous, next, mode) {
  if (next !== "live" || previous === "live" || previous === "connecting") return false;
  return mode === "resume";
}

/** Whether the tool needs the tunnel's own connection. */
export function needsTunnel(spec) {
  switch (spec.tool) {
    case "settings":
    case "tunnel-settings":
    case "logs":
      return false;
    case "terminal":
    case "files":
    case "editor":
      return !!spec.remoteHost;
  }
  return true;
}

/** @param {number} seconds @returns {string} a refresh interval as a label */
export function formatInterval(seconds) {
  if (!seconds) return "Manual only";
  if (seconds < 60) return `${seconds} s`;
  return `${seconds / 60} min`;
}

export const REFRESH_CHOICES = [1, 2, 3, 5, 10, 30, 60, 0];
