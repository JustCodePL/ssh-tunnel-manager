import test from "node:test";
import assert from "node:assert/strict";
import { base64ToBytes, bytesToBase64, createClipboardShim, installClipboardShim } from "../src/lib/clipboardBridge.js";

function setup(overrides = {}) {
  const calls = { reads: [], text: [], image: [] };
  let time = 0;
  let mode = overrides.mode ?? "both";
  const shim = createClipboardShim({
    read: async (includeImage) => {
      calls.reads.push(includeImage);
      return overrides.content ?? { text: "local text", png: includeImage ? "iVBORw0KGgo=" : undefined };
    },
    writeText: async (text) => calls.text.push(text),
    writeImage: async (png) => calls.image.push(png),
    mode: () => mode,
    now: () => time,
    textIntervalMs: 500,
    imageIntervalMs: 3000,
  });
  return { shim, calls, advance: (ms) => (time += ms), setMode: (value) => (mode = value) };
}

function remoteItem(type, blob) {
  return { types: [type], getType: async () => blob };
}

test("reading returns text and picture from the backend", async () => {
  const { shim } = setup();
  const [item] = await shim.read();
  assert.deepEqual(item.types, ["text/plain", "image/png"]);
  assert.equal(await (await item.getType("text/plain")).text(), "local text");
  const png = new Uint8Array(await (await item.getType("image/png")).arrayBuffer());
  assert.equal(png[1], "P".charCodeAt(0));
  await assert.rejects(item.getType("text/html"));
});

test("the monitor's fast polling does not reach the backend every time", async () => {
  const { shim, calls, advance } = setup();
  await shim.read();
  await shim.read();
  advance(100);
  await shim.read();
  assert.equal(calls.reads.length, 1);
  advance(500);
  await shim.read();
  assert.deepEqual(calls.reads, [true, false], "pictures are read less often than text");
  advance(3000);
  await shim.read();
  assert.deepEqual(calls.reads, [true, false, true]);
});

test("an empty clipboard has no items", async () => {
  const { shim } = setup({ content: { text: "" } });
  assert.deepEqual(await shim.read(), []);
});

test("the direction setting limits both ways", async () => {
  for (const [mode, sends, receives] of [
    ["both", true, true],
    ["to", true, false],
    ["from", false, true],
    ["off", false, false],
  ]) {
    const { shim, calls } = setup({ mode });
    const items = await shim.read();
    assert.equal(items.length > 0, sends, `${mode} read`);
    await shim.write([remoteItem("text/plain", new Blob(["remote"]))]);
    assert.deepEqual(calls.text, receives ? ["remote"] : [], `${mode} write`);
  }
});

test("a change of the setting applies at once", async () => {
  const { shim, calls, setMode } = setup();
  setMode("off");
  await shim.write([remoteItem("text/plain", new Blob(["hidden"]))]);
  assert.deepEqual(calls.text, []);
});

test("writes from the remote go to the backend once", async () => {
  const { shim, calls } = setup();
  const item = remoteItem("text/plain", new Blob(["same"]));
  await shim.write([item]);
  await shim.write([item]);
  assert.deepEqual(calls.text, ["same"]);
  const picture = remoteItem("image/png", new Blob([new Uint8Array([1, 2, 3])]));
  await shim.write([picture]);
  assert.deepEqual(calls.image, ["AQID"]);
});

test("base64 helpers round-trip large buffers", () => {
  const bytes = new Uint8Array(200000).map((_, i) => i % 251);
  assert.deepEqual(base64ToBytes(bytesToBase64(bytes)), bytes);
});

test("installing replaces the page API and allows permission checks", async () => {
  const nav = { permissions: { query: async () => ({ state: "prompt" }) } };
  const win = { isSecureContext: false };
  const shim = { marker: true };
  installClipboardShim(shim, nav, win);
  assert.equal(nav.clipboard, shim);
  assert.equal(win.isSecureContext, true);
  assert.equal((await nav.permissions.query({ name: "clipboard-read" })).state, "granted");
  assert.equal((await nav.permissions.query({ name: "camera" })).state, "prompt");
});
