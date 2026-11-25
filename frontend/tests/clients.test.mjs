import test from "node:test";
import assert from "node:assert/strict";
import { clientRows, desktopPort, filterRows, groupRows, webRows, NO_GROUP } from "../src/lib/clients.js";

const tunnels = [
  {
    id: "jump", name: "jump", group: "Office", pinned: false,
    portForwards: [
      { remoteHost: "10.0.0.1", remotePort: 3389, localPort: 13389, description: "" },
      { remoteHost: "10.0.0.2", remotePort: 5432, localPort: 15432, description: "db" },
      { remoteHost: "10.0.0.3", remotePort: 8080, localPort: 18080, description: "Panel", portless: true, domain: "panel", exposePort: 80 },
      { remoteHost: "10.0.0.4", remotePort: 8081, localPort: 18081, description: "", portless: true, domain: "api" },
    ],
  },
  { id: "solo", name: "solo", group: "", pinned: true, portForwards: [] },
];
const desktops = [
  { id: "rd:laptop", name: "laptop", address: "192.168.1.5", port: 3389, protocol: "rdp", group: "Office", pinned: true },
  { id: "rd:mac", name: "mac", address: "mac.lan", port: 5900, protocol: "vnc", group: "" },
];
const attach = { jump: [{ protocol: "ssh", label: "Terminal" }], solo: [{ protocol: "ssh", label: "Terminal" }, { protocol: "vnc", label: "Screen sharing (VNC)" }] };

test("the Clients tab lists desktops, attachable forwards and host sessions", () => {
  const rows = clientRows(tunnels, desktops, attach);
  assert.deepEqual(rows.map((r) => r.key), ["rd:laptop", "rd:mac", "jump|ssh", "jump#0", "solo|ssh", "solo|vnc"]);
  const forward = rows.find((r) => r.kind === "forward");
  assert.equal(forward.name, "jump → 10.0.0.1:3389");
  assert.equal(forward.route, "jump");
  assert.equal(forward.group, "Office");
  assert.deepEqual(forward.spec, { tool: "rdp", tunnelId: "jump", remoteHost: "10.0.0.1", remotePort: 3389 });
  assert.deepEqual(rows[1].spec, { tool: "vnc", tunnelId: "rd:mac" });
  assert.equal(rows[1].route, "");
  assert.deepEqual(rows.find((r) => r.key === "solo|vnc").spec, { tool: "vnc", tunnelId: "solo" });
});

test("groups are alphabetical with the ungrouped last, pinned rows first", () => {
  const groups = groupRows(clientRows(tunnels, desktops, attach));
  assert.deepEqual(groups.map((g) => g.label), ["Office", NO_GROUP]);
  assert.deepEqual(groups[0].rows.map((r) => r.key), ["rd:laptop", "jump|ssh", "jump#0"]);
  assert.deepEqual(groups[1].rows.map((r) => r.key), ["solo|ssh", "solo|vnc", "rd:mac"]);
});

test("search matches names, addresses and routes", () => {
  const rows = clientRows(tunnels, desktops, attach);
  assert.deepEqual(filterRows(rows, "192.168").map((r) => r.key), ["rd:laptop"]);
  assert.deepEqual(filterRows(rows, "JUMP").map((r) => r.key), ["jump|ssh", "jump#0"]);
  assert.equal(filterRows(rows, " ").length, rows.length);
});

test("the Web tab lists portless forwards only", () => {
  const rows = webRows(tunnels);
  assert.deepEqual(rows.map((r) => [r.name, r.url, r.route]), [
    ["Panel", "http://panel.ssh-local", "jump"],
    ["api.ssh-local:8081", "http://api.ssh-local:8081", "jump"],
  ]);
});

test("a desktop's port defaults by protocol", () => {
  assert.equal(desktopPort("", "rdp"), 3389);
  assert.equal(desktopPort("", "vnc"), 5900);
  assert.equal(desktopPort("5901", "vnc"), 5901);
  assert.equal(desktopPort("99999", "rdp"), 3389);
});

test("a tab of a remote desktop is titled by its name", async () => {
  const { tabTitle } = await import("../src/lib/tabs.js");
  assert.equal(tabTitle({ tool: "rdp", tunnelId: "rd:work laptop" }), "Remote desktop: work laptop");
  assert.equal(tabTitle({ tool: "rdp", tunnelId: "jump", remoteHost: "10.0.0.1", remotePort: 3389 }), "Remote desktop: jump to 10.0.0.1:3389");
});
