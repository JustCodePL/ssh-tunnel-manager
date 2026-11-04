import test from "node:test";
import assert from "node:assert/strict";
import { filterTunnels, groupTunnels, isOn, shortError, toolAvailable, toolsIn } from "../src/lib/rowLayout.js";

test("tools need what was detected on the host", () => {
  assert.equal(toolAvailable("docker", undefined), false);
  assert.equal(toolAvailable("docker", { docker: true }), true);
  assert.equal(toolAvailable("htop", { docker: true }), false);
  assert.equal(toolAvailable("disk", undefined), false, "disk waits for a first probe");
  assert.equal(toolAvailable("disk", {}), true);
  assert.equal(toolAvailable("files", undefined), true);
  assert.equal(toolAvailable("logs", undefined), true);
});

test("the row shows only row tools, the panel everything not switched off", () => {
  const row = { files: "row", logs: "panel", disk: "off", docker: "off", htop: "panel" };
  const caps = { docker: true, htop: true };
  assert.deepEqual(toolsIn(row, "row", caps).map((t) => t.key), ["files"]);
  assert.deepEqual(toolsIn(row, "panel", caps).map((t) => t.key), ["files", "logs", "htop"]);
});

test("a failed tunnel's reason is one short line", () => {
  assert.equal(shortError(undefined), "connection failed");
  assert.equal(shortError("auth failed\nmore detail"), "auth failed");
  assert.equal(shortError("x".repeat(200)).length, 80);
});

test("isOn covers everything that is not stopped", () => {
  for (const status of ["connected", "connecting", "reconnecting"]) assert.equal(isOn(status), true);
  for (const status of ["disconnected", "error", undefined]) assert.equal(isOn(status), false);
});

test("groups go pinned, named alphabetically, then unassigned", () => {
  const sections = groupTunnels([
    { id: "1", name: "z", group: "Zeta" },
    { id: "2", name: "a", group: "Alpha" },
    { id: "3", name: "u" },
    { id: "4", name: "p", group: "Alpha", pinned: true },
  ]);
  assert.deepEqual(sections.map((s) => s.label), ["Pinned", "Alpha", "Zeta", "Unassigned"]);
  assert.deepEqual(sections[1].tunnels.map((t) => t.id), ["2"], "a pinned tunnel appears once");
});

test("a group made by hand appears empty, in order, and only once", () => {
  const sections = groupTunnels([{ id: "1", name: "z", group: "Zeta" }], ["Alpha", "Zeta"]);
  assert.deepEqual(sections.map((s) => s.label), ["Alpha", "Zeta"]);
  assert.deepEqual(sections[0].tunnels, []);
  assert.equal(sections[1].tunnels.length, 1);
});

test("filters combine with the search text", () => {
  const tunnels = [
    { id: "a", name: "prod", host: "1.1.1.1", group: "G", sourceFile: "/x" },
    { id: "b", name: "stage", user: "deploy", pinned: true },
  ];
  const status = (id) => (id === "a" ? "connected" : "disconnected");
  const ids = (filter, query = "") => filterTunnels(tunnels, filter, query, status).map((t) => t.id);
  assert.deepEqual(ids("all"), ["a", "b"]);
  assert.deepEqual(ids("live"), ["a"]);
  assert.deepEqual(ids("pinned"), ["b"]);
  assert.deepEqual(ids("unassigned"), ["b"]);
  assert.deepEqual(ids("group:G"), ["a"]);
  assert.deepEqual(ids("file:/x"), ["a"]);
  assert.deepEqual(ids("all", "DEPLOY"), ["b"]);
  assert.deepEqual(ids("all", "1.1.1"), ["a"]);
  assert.deepEqual(ids("live", "deploy"), []);
});
