/**
 * Stands in for navigator.clipboard in a remote desktop window. IronRDP's client
 * talks to that API, but a webview only allows it from a user gesture, so its
 * background monitoring cannot work there. This version uses the backend instead
 * and applies the direction chosen in the tunnel's settings.
 *
 * @param {object} backend
 * @param {(includeImage: boolean) => Promise<{text?: string, png?: string}>} backend.read
 * @param {(text: string) => Promise<void>} backend.writeText
 * @param {(base64: string) => Promise<void>} backend.writeImage
 * @param {() => string} backend.mode "both", "to", "from" or "off"
 * @param {() => number} [backend.now]
 * @param {number} [backend.textIntervalMs] how often text is really read
 * @param {number} [backend.imageIntervalMs] how often pictures are really read
 */
export function createClipboardShim(backend) {
  const now = backend.now ?? (() => Date.now());
  const textInterval = backend.textIntervalMs ?? 500;
  const imageInterval = backend.imageIntervalMs ?? 3000;
  let lastText = "";
  let lastImage = "";
  let lastTextRead = -Infinity;
  let lastImageRead = -Infinity;
  let cached = /** @type {{text?: string, png?: string}} */ ({});
  let reading = null;

  const allowsSend = () => ["both", "to"].includes(backend.mode());
  const allowsReceive = () => ["both", "from"].includes(backend.mode());

  function toItems(content) {
    const types = [];
    if (content.text) types.push("text/plain");
    if (content.png) types.push("image/png");
    if (!types.length) return [];
    return [
      {
        types,
        async getType(type) {
          if (type === "text/plain") return new Blob([content.text], { type });
          if (type === "image/png") return new Blob([base64ToBytes(content.png)], { type });
          throw new Error(`no ${type} on the clipboard`);
        },
      },
    ];
  }

  return {
    async read() {
      if (!allowsSend()) return [];
      const time = now();
      if (time - lastTextRead < textInterval) return toItems(cached);
      if (reading) return toItems(cached);
      lastTextRead = time;
      const includeImage = time - lastImageRead >= imageInterval;
      if (includeImage) lastImageRead = time;
      reading = backend
        .read(includeImage)
        .then((content) => {
          cached = { text: content.text ?? "", png: includeImage ? content.png ?? "" : cached.png };
        })
        .finally(() => {
          reading = null;
        });
      await reading;
      return toItems(cached);
    },
    async write(items) {
      if (!allowsReceive()) return;
      for (const item of items) {
        if (item.types.includes("image/png")) {
          const png = bytesToBase64(new Uint8Array(await (await item.getType("image/png")).arrayBuffer()));
          if (png !== lastImage) {
            lastImage = png;
            await backend.writeImage(png);
            cached = { ...cached, png };
          }
        }
        if (item.types.includes("text/plain")) {
          const text = await (await item.getType("text/plain")).text();
          if (text !== lastText) {
            lastText = text;
            await backend.writeText(text);
            cached = { ...cached, text };
          }
        }
      }
    },
    async readText() {
      return allowsSend() ? (await backend.read(false)).text ?? "" : "";
    },
    async writeText(text) {
      if (allowsReceive()) await backend.writeText(text);
    },
  };
}

/** @param {string} encoded @returns {Uint8Array} */
export function base64ToBytes(encoded) {
  const raw = atob(encoded);
  const bytes = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i);
  return bytes;
}

/** @param {Uint8Array} bytes @returns {string} */
export function bytesToBase64(bytes) {
  let binary = "";
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(binary);
}

/**
 * Replaces the page's clipboard API. Call before the RDP client starts, because
 * it checks the API once while connecting.
 * @param {ReturnType<typeof createClipboardShim>} shim
 * @param {any} [nav]
 * @param {any} [win]
 */
export function installClipboardShim(shim, nav = navigator, win = window) {
  Object.defineProperty(nav, "clipboard", { configurable: true, value: shim });
  if (!win.isSecureContext) {
    Object.defineProperty(win, "isSecureContext", { configurable: true, value: true });
  }
  const permissions = nav.permissions;
  if (permissions) {
    const query = permissions.query?.bind(permissions);
    permissions.query = (descriptor) =>
      descriptor?.name === "clipboard-read" || descriptor?.name === "clipboard-write"
        ? Promise.resolve({ state: "granted" })
        : query(descriptor);
  }
}
