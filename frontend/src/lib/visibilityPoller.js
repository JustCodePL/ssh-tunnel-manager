/**
 * @typedef {object} VisibilitySource
 * @property {boolean} hidden
 * @property {(type: "visibilitychange", listener: () => void) => void} addEventListener
 * @property {(type: "visibilitychange", listener: () => void) => void} removeEventListener
 */

/**
 * @typedef {object} VisibilityPollerOptions
 * @property {VisibilitySource} [documentRef]
 * @property {(callback: () => void, delayMs: number) => ReturnType<typeof setTimeout>} [setTimer]
 * @property {(timer: ReturnType<typeof setTimeout>) => void} [clearTimer]
 */

/**
 * Run a poll immediately while the document is visible, then schedule the next
 * run only after the current one completes. Hidden documents keep no timer and
 * refresh once as soon as they become visible again.
 *
 * @param {() => void | Promise<void>} poll
 * @param {number} intervalMs
 * @param {VisibilityPollerOptions} [options]
 */
export function createVisibilityPoller(poll, intervalMs, options = {}) {
  const documentRef = options.documentRef ?? document;
  const setTimer = options.setTimer ?? ((callback, delayMs) => setTimeout(callback, delayMs));
  const clearTimer = options.clearTimer ?? ((timer) => clearTimeout(timer));

  /** @type {ReturnType<typeof setTimeout> | null} */
  let timer = null;
  let stopped = true;
  let enabled = true;
  let running = false;
  let rerunRequested = false;

  function isActive() {
    return !stopped && enabled && !documentRef.hidden;
  }

  function clearScheduled() {
    if (timer === null) return;
    clearTimer(timer);
    timer = null;
  }

  function scheduleNext() {
    if (!isActive() || running || timer !== null) return;
    timer = setTimer(() => {
      timer = null;
      void run();
    }, intervalMs);
  }

  async function run() {
    if (!isActive()) return;
    if (running) {
      rerunRequested = true;
      return;
    }

    clearScheduled();
    running = true;
    try {
      await poll();
    } finally {
      running = false;
      if (!isActive()) {
        rerunRequested = false;
        return;
      }
      if (rerunRequested) {
        rerunRequested = false;
        void run();
        return;
      }
      scheduleNext();
    }
  }

  function handleVisibilityChange() {
    clearScheduled();
    if (documentRef.hidden) {
      rerunRequested = false;
      return;
    }
    void run();
  }

  return {
    start() {
      if (!stopped) return;
      stopped = false;
      documentRef.addEventListener("visibilitychange", handleVisibilityChange);
      void run();
    },

    stop() {
      if (stopped) return;
      stopped = true;
      rerunRequested = false;
      clearScheduled();
      documentRef.removeEventListener("visibilitychange", handleVisibilityChange);
    },

    /** @param {boolean} value */
    setEnabled(value) {
      if (enabled === value) return;
      enabled = value;
      clearScheduled();
      if (!enabled) {
        rerunRequested = false;
        return;
      }
      void run();
    },
  };
}
