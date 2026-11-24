/**
 * Keeps the text clipboard in step with a VNC session, in the directions the
 * tunnel's clipboard setting allows. VNC carries plain text only.
 *
 * @param {object} backend
 * @param {() => Promise<string>} backend.read reads the local clipboard's text
 * @param {(text: string) => Promise<void>} backend.write puts text on the local clipboard
 * @param {() => string} backend.mode "both", "to", "from" or "off"
 */
export function createVncClipboard(backend) {
  // The last text known on both sides, so a copy is not echoed back.
  let shared = "";

  const allowsSend = () => ["both", "to"].includes(backend.mode());
  const allowsReceive = () => ["both", "from"].includes(backend.mode());

  return {
    /** Forgets what the server has, for a new session. */
    reset() {
      shared = "";
    },

    /**
     * Text copied on the remote screen.
     * @param {string} text
     */
    async received(text) {
      if (!allowsReceive() || !text || text === shared) return;
      shared = text;
      await backend.write(text);
    },

    /**
     * Reads the local clipboard when the viewer gets focus.
     * @returns {Promise<string | null>} text to send to the server, if it changed
     */
    async local() {
      if (!allowsSend()) return null;
      const text = await backend.read();
      if (!text || text === shared) return null;
      shared = text;
      return text;
    },
  };
}
