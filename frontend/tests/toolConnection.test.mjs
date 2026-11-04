import test from "node:test";
import assert from "node:assert/strict";
import { formatInterval, needsTunnel, shouldRestart, toolState } from "../src/lib/toolConnection.js";

test("tools that open their own connection are always live", () => {
  assert.equal(toolState("disconnected", false, false, "resume"), "live");
});

test("a tool waits for its first connection", () => {
  assert.equal(toolState("disconnected", true, false, "resume"), "connecting");
  assert.equal(toolState("connecting", true, false, "resume"), "connecting");
  assert.equal(toolState("connected", true, false, "resume"), "live");
  assert.equal(toolState("error", true, false, "resume"), "lost");
});

test("a connected tool reports losing the connection", () => {
  assert.equal(toolState("reconnecting", true, true, "resume"), "reconnecting");
  assert.equal(toolState("connecting", true, true, "resume"), "reconnecting");
  assert.equal(toolState("disconnected", true, true, "resume"), "lost");
  assert.equal(toolState("error", true, true, "keep"), "lost");
});

test("only resume mode restarts by itself, and only after a loss", () => {
  assert.equal(shouldRestart("reconnecting", "live", "resume"), true);
  assert.equal(shouldRestart("lost", "live", "resume"), true);
  assert.equal(shouldRestart("reconnecting", "live", "keep"), false);
  assert.equal(shouldRestart("connecting", "live", "resume"), false, "the first connection is no restart");
  assert.equal(shouldRestart("live", "live", "resume"), false);
  assert.equal(shouldRestart("live", "lost", "resume"), false);
});

test("which tools need the tunnel's connection", () => {
  assert.equal(needsTunnel({ tool: "terminal" }), false);
  assert.equal(needsTunnel({ tool: "terminal", remoteHost: "h" }), true);
  assert.equal(needsTunnel({ tool: "files" }), false);
  assert.equal(needsTunnel({ tool: "editor", remoteHost: "h" }), true);
  assert.equal(needsTunnel({ tool: "docker" }), true);
  assert.equal(needsTunnel({ tool: "rdp" }), true);
  assert.equal(needsTunnel({ tool: "logs" }), false);
  assert.equal(needsTunnel({ tool: "tunnel-settings" }), false);
});

test("intervals read naturally", () => {
  assert.equal(formatInterval(0), "Manual only");
  assert.equal(formatInterval(3), "3 s");
  assert.equal(formatInterval(60), "1 min");
});
