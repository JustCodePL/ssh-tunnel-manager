// Maps macOS Cmd to Ctrl for a remote desktop, so Cmd+C copies on the remote
// host. It also works around a macOS browser quirk: while Cmd is held, the keyup
// of other keys is never delivered. Without compensation those keys would stay
// pressed on the remote host.

/** @type {Record<string, string>} */
const META_TO_CONTROL = { MetaLeft: "ControlLeft", MetaRight: "ControlRight", OSLeft: "ControlLeft", OSRight: "ControlRight" };
const MODIFIERS = new Set(["ShiftLeft", "ShiftRight", "ControlLeft", "ControlRight", "AltLeft", "AltRight", ...Object.keys(META_TO_CONTROL)]);

/**
 * @typedef {{ type: "keydown" | "keyup", code: string, key: string, ctrlKey: boolean, shiftKey: boolean, altKey: boolean }} MappedKey
 */

/**
 * Creates a stateful mapper for one session.
 * @returns {(evt: { type: string, code: string, key: string, metaKey: boolean, shiftKey: boolean, altKey: boolean }) => MappedKey[] | null}
 *   null to leave the event unchanged, otherwise the events to send instead.
 */
export function createMacKeyMapper() {
  /** @type {Map<string, string>} */
  const heldWithCmd = new Map();

  return (evt) => {
    const control = META_TO_CONTROL[evt.code];
    if (control) {
      const down = evt.type === "keydown";
      const mapped = { type: /** @type {"keydown" | "keyup"} */ (evt.type), code: control, key: "Control", ctrlKey: down, shiftKey: evt.shiftKey, altKey: evt.altKey };
      if (down) return [mapped];
      const released = [...heldWithCmd].map(([code, key]) => ({ type: /** @type {"keyup"} */ ("keyup"), code, key, ctrlKey: true, shiftKey: evt.shiftKey, altKey: evt.altKey }));
      heldWithCmd.clear();
      return [...released, mapped];
    }

    if (MODIFIERS.has(evt.code)) return null;

    if (evt.type === "keydown" && evt.metaKey) {
      heldWithCmd.set(evt.code, evt.key);
    } else if (evt.type === "keyup") {
      heldWithCmd.delete(evt.code);
    }
    return null;
  };
}
