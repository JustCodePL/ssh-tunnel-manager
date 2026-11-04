import test from "node:test";
import assert from "node:assert/strict";
import { breadcrumbs, scaleBar, sortContainers, sortProcesses } from "../src/lib/toolTables.js";
import { tabTitle } from "../src/lib/tabs.js";

const processes = [
  { command: "b", cpu: 1, mem: 9, pid: 3 },
  { command: "a", cpu: 5, mem: 2, pid: 2 },
  { command: "c", cpu: 1, mem: 2, pid: 1 },
];

test("processes sort by each column without changing the input", () => {
  assert.deepEqual(sortProcesses(processes, "cpu").map((p) => p.command), ["a", "b", "c"]);
  assert.deepEqual(sortProcesses(processes, "mem").map((p) => p.command), ["b", "a", "c"]);
  assert.deepEqual(sortProcesses(processes, "name").map((p) => p.command), ["a", "b", "c"]);
  assert.deepEqual(sortProcesses(processes, "pid").map((p) => p.pid), [1, 2, 3]);
  assert.equal(processes[0].command, "b");
});

test("bars scale to the busiest row", () => {
  assert.equal(scaleBar(5, 10), 50);
  assert.equal(scaleBar(20, 10), 100);
  assert.equal(scaleBar(1, 0), 0);
});

test("containers sort by usage text", () => {
  const list = [
    { name: "a", cpuPercent: "1.0%", memPercent: "9%" },
    { name: "b", cpuPercent: "5.5%", memPercent: "" },
    { name: "c" },
  ];
  assert.deepEqual(sortContainers(list, "cpu").map((c) => c.name), ["b", "a", "c"]);
  assert.deepEqual(sortContainers(list, "mem").map((c) => c.name), ["a", "b", "c"]);
  assert.deepEqual(sortContainers(list, "name").map((c) => c.name), ["a", "b", "c"]);
});

test("breadcrumbs walk from the root", () => {
  assert.deepEqual(breadcrumbs("/"), [{ label: "/", path: "/" }]);
  assert.deepEqual(breadcrumbs("/srv/app").map((c) => c.path), ["/", "/srv", "/srv/app"]);
});

test("tab titles name the tool and target", () => {
  assert.equal(tabTitle({ tool: "settings", tunnelId: "" }), "Settings");
  assert.equal(tabTitle({ tool: "docker", tunnelId: "prod" }), "Docker: prod");
  assert.equal(tabTitle({ tool: "rdp", tunnelId: "nas", remoteHost: "10.0.0.1", remotePort: 3389 }), "Remote desktop: nas to 10.0.0.1:3389");
  assert.equal(tabTitle({ tool: "editor", tunnelId: "nas", path: "/etc/hosts" }), "hosts");
});
