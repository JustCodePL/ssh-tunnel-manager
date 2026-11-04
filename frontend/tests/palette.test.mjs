import test from "node:test";
import assert from "node:assert/strict";
import { buildActions, searchActions } from "../src/lib/palette.js";

const state = {
  tunnels: [
    {
      id: "g146",
      name: "mikrus-g146",
      group: "Mikrus",
      portForwards: [
        { localPort: 15432, remoteHost: "127.0.0.1", remotePort: 5432 },
        { localPort: 13389, remoteHost: "10.0.0.1", remotePort: 3389 },
      ],
    },
    { id: "lap", name: "laptop" },
  ],
  statusOf: (id) => (id === "g146" ? "connected" : "disconnected"),
  capabilities: { g146: { docker: true, htop: false } },
  attachOptions: (id) => (id === "g146" ? [{ protocol: "ssh" }, { protocol: "rdp" }] : [{ protocol: "ssh" }]),
};

test("actions follow the state", () => {
  const actions = buildActions(state);
  const labels = actions.map((a) => a.label);
  assert.ok(labels.includes("Disconnect mikrus-g146"));
  assert.ok(labels.includes("Connect laptop"));
  assert.ok(labels.includes("Remote desktop on mikrus-g146"));
  assert.ok(!labels.includes("Remote desktop on laptop"));
  assert.ok(labels.includes("Docker on mikrus-g146"));
  assert.ok(!labels.includes("htop on mikrus-g146"), "not detected");
  assert.ok(labels.includes("Copy 127.0.0.1:15432"));
  assert.ok(labels.includes("Remote desktop on 10.0.0.1:3389"));
  assert.ok(labels.includes("Disconnect group Mikrus"));
  assert.equal(new Set(actions.map((a) => a.id)).size, actions.length, "ids are unique");
});

test("every word must match, earlier matches first", () => {
  const actions = buildActions(state);
  const found = searchActions(actions, "term g146");
  assert.equal(found[0].label, "Terminal on mikrus-g146");
  assert.deepEqual(searchActions(actions, "no such thing"), []);
  assert.equal(searchActions(actions, "", 3).length, 3);
  assert.equal(searchActions(actions, "COPY 15432")[0].run.text, "127.0.0.1:15432");
});

test("a forward's Attach carries its target", () => {
  const run = searchActions(buildActions(state), "desktop 10.0.0.1")[0].run;
  assert.deepEqual(run.spec, { tool: "rdp", tunnelId: "g146", remoteHost: "10.0.0.1", remotePort: 3389 });
});
