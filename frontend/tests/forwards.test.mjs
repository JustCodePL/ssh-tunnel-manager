import test from "node:test";
import assert from "node:assert/strict";
import { attachExpands, forwardAddress, forwardAttach, forwardTags, toolForProtocol } from "../src/lib/forwards.js";

test("a forward's local address", () => {
  assert.equal(forwardAddress({ localPort: 15432 }), "127.0.0.1:15432");
  assert.equal(forwardAddress({ portless: true, domain: "db", remotePort: 5432 }), "db.ssh-local:5432");
  assert.equal(forwardAddress({ portless: true, domain: "web", remotePort: 8080, exposePort: 80 }), "http://web.ssh-local");
  assert.equal(forwardAddress({ portless: true, domain: "web", remotePort: 8443, exposePort: 443 }), "https://web.ssh-local");
});

test("Attach on a forward follows its type, also when portless", () => {
  assert.equal(forwardAttach({ remotePort: 3389 }).tool, "rdp");
  assert.equal(forwardAttach({ remotePort: 3389, portless: true, domain: "x" }).tool, "rdp");
  assert.equal(forwardAttach({ remotePort: 22 }).tool, "terminal");
  assert.equal(forwardAttach({ remotePort: 5432 }), null);
  assert.equal(forwardAttach({ remotePort: 8080, protocol: "rdp" }).tool, "rdp");
  assert.equal(forwardAttach({ remotePort: 3389, protocol: "none" }), null);
});

test("tags name portless and the type", () => {
  assert.deepEqual(forwardTags({ remotePort: 3389, portless: true }), ["portless", "RDP"]);
  assert.deepEqual(forwardTags({ remotePort: 22 }), ["SSH"]);
  assert.deepEqual(forwardTags({ remotePort: 80 }), []);
});

test("Attach expands only with more than one protocol", () => {
  assert.equal(attachExpands([]), false);
  assert.equal(attachExpands([{ protocol: "ssh", label: "Terminal" }]), false);
  assert.equal(attachExpands([{ protocol: "ssh", label: "Terminal" }, { protocol: "rdp", label: "RDP" }]), true);
  assert.equal(toolForProtocol("rdp"), "rdp");
  assert.equal(toolForProtocol("ssh"), "terminal");
});
