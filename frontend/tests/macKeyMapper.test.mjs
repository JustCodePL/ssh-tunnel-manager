import test from "node:test";
import assert from "node:assert/strict";
import { createMacKeyMapper } from "../src/lib/macKeyMapper.js";

const event = (type, code, key, metaKey = false) => ({ type, code, key, metaKey, shiftKey: false, altKey: false });

test("Cmd becomes Ctrl", () => {
  const map = createMacKeyMapper();
  assert.deepEqual(map(event("keydown", "MetaLeft", "Meta", true)).map((e) => [e.type, e.code, e.ctrlKey]), [["keydown", "ControlLeft", true]]);
  assert.deepEqual(map(event("keyup", "MetaRight", "Meta")).map((e) => [e.type, e.code, e.ctrlKey]), [["keyup", "ControlRight", false]]);
});

test("ordinary keys pass through unchanged", () => {
  const map = createMacKeyMapper();
  assert.equal(map(event("keydown", "KeyA", "a")), null);
  assert.equal(map(event("keyup", "KeyA", "a")), null);
  assert.equal(map(event("keydown", "AltRight", "Alt")), null);
});

test("a key whose keyup macOS swallows is released before Ctrl", () => {
  const map = createMacKeyMapper();
  map(event("keydown", "MetaLeft", "Meta", true));
  assert.equal(map(event("keydown", "KeyC", "c", true)), null);
  const released = map(event("keyup", "MetaLeft", "Meta"));
  assert.deepEqual(released.map((e) => [e.type, e.code]), [["keyup", "KeyC"], ["keyup", "ControlLeft"]]);
});

test("a key that did report keyup is not released twice", () => {
  const map = createMacKeyMapper();
  map(event("keydown", "MetaLeft", "Meta", true));
  map(event("keydown", "KeyC", "c", true));
  map(event("keyup", "KeyC", "c", true));
  assert.deepEqual(map(event("keyup", "MetaLeft", "Meta")).map((e) => e.code), ["ControlLeft"]);
});

test("nothing leaks into the next Cmd press", () => {
  const map = createMacKeyMapper();
  map(event("keydown", "MetaLeft", "Meta", true));
  map(event("keydown", "KeyV", "v", true));
  map(event("keyup", "MetaLeft", "Meta"));
  map(event("keydown", "MetaLeft", "Meta", true));
  assert.deepEqual(map(event("keyup", "MetaLeft", "Meta")).map((e) => e.code), ["ControlLeft"]);
});
