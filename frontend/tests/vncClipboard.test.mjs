import { test } from "node:test";
import assert from "node:assert/strict";
import { createVncClipboard } from "../src/lib/vncClipboard.js";

function setup(mode, local = "") {
  const state = { mode, local, written: [] };
  const clipboard = createVncClipboard({
    read: async () => state.local,
    write: async (text) => {
      state.written.push(text);
      state.local = text;
    },
    mode: () => state.mode,
  });
  return { state, clipboard };
}

test("text copied on the remote screen reaches the local clipboard once", async () => {
  const { state, clipboard } = setup("both");
  await clipboard.received("remote");
  await clipboard.received("remote");
  assert.deepEqual(state.written, ["remote"]);
  // It is not sent back when the viewer gets focus.
  assert.equal(await clipboard.local(), null);
});

test("a local copy is sent when the viewer gets focus", async () => {
  const { state, clipboard } = setup("both", "local");
  assert.equal(await clipboard.local(), "local");
  assert.equal(await clipboard.local(), null);
  state.local = "newer";
  assert.equal(await clipboard.local(), "newer");
  assert.equal(await clipboard.local(), null);
});

test("the tunnel's clipboard direction is respected", async () => {
  const to = setup("to", "local");
  await to.clipboard.received("remote");
  assert.deepEqual(to.state.written, []);
  assert.equal(await to.clipboard.local(), "local");

  const from = setup("from", "local");
  assert.equal(await from.clipboard.local(), null);
  await from.clipboard.received("remote");
  assert.deepEqual(from.state.written, ["remote"]);

  const off = setup("off", "local");
  assert.equal(await off.clipboard.local(), null);
  await off.clipboard.received("remote");
  assert.deepEqual(off.state.written, []);
});

test("a new session gets the local clipboard again", async () => {
  const { clipboard } = setup("both", "local");
  assert.equal(await clipboard.local(), "local");
  clipboard.reset();
  assert.equal(await clipboard.local(), "local");
});

test("the direction can change while the session runs", async () => {
  const { state, clipboard } = setup("off", "local");
  assert.equal(await clipboard.local(), null);
  state.mode = "both";
  assert.equal(await clipboard.local(), "local");
});
