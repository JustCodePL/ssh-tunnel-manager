import test from "node:test";
import assert from "node:assert/strict";

import { domainWithSuffix, domainWithoutSuffix, normalizePortlessSuffix, preparePortForwardsForSave, effectiveForwardProtocol, inferForwardProtocol } from "../src/lib/portForwards.js";

test("keeps a portless forward whose local port is automatic", () => {
  const forwards = preparePortForwardsForSave([
    {
      localPort: 0,
      remoteHost: "127.0.0.1",
      remotePort: 6379,
      description: "cache",
      portless: true,
      domain: "redis.dev",
    },
  ]);

  assert.equal(forwards.length, 1);
  assert.equal(forwards[0].localPort, 0);
  assert.equal(forwards[0].remotePort, 6379);
});

test("keeps complete plain forwards and discards incomplete rows", () => {
  const forwards = preparePortForwardsForSave([
    {
      localPort: 5432,
      remoteHost: "127.0.0.1",
      remotePort: 5432,
      description: "database",
    },
    {
      localPort: 0,
      remoteHost: "127.0.0.1",
      remotePort: 0,
      description: "",
    },
    {
      localPort: 0,
      remoteHost: "127.0.0.1",
      remotePort: 8080,
      description: "missing local port",
    },
  ]);

  assert.deepEqual(forwards.map((pf) => pf.description), ["database"]);
});

test("the protocol is inferred from standard ports", () => {
  assert.equal(inferForwardProtocol(3389), "rdp");
  assert.equal(inferForwardProtocol("3389"), "rdp");
  assert.equal(inferForwardProtocol(22), "ssh");
  assert.equal(inferForwardProtocol(8080), "none");
  assert.equal(inferForwardProtocol(undefined), "none");
});

test("an explicit protocol wins over the port", () => {
  assert.equal(effectiveForwardProtocol({ remotePort: 3389, protocol: "none" }), "none");
  assert.equal(effectiveForwardProtocol({ remotePort: 8080, protocol: "rdp" }), "rdp");
  assert.equal(effectiveForwardProtocol({ remotePort: 22 }), "ssh");
});

const forwardWith = (fields) => ({ localPort: 1, remoteHost: "h", remotePort: 8080, description: "", ...fields });

test("saving keeps only a protocol that differs from the port", () => {
  const save = (fields) => preparePortForwardsForSave([forwardWith(fields)])[0];
  assert.equal(save({ remotePort: 3389, protocol: "rdp" }).protocol, undefined);
  assert.equal(save({ remotePort: 8080, protocol: "none" }).protocol, undefined);
  assert.equal(save({ remotePort: 3389, protocol: "none" }).protocol, "none");
  assert.equal(save({ remotePort: 8080, protocol: "rdp" }).protocol, "rdp");
  assert.equal(save({ remotePort: 2222, protocol: "ssh" }).protocol, "ssh");
});

test("the SSH user is kept only on SSH forwards", () => {
  const save = (fields) => preparePortForwardsForSave([forwardWith(fields)])[0];
  assert.equal(save({ remotePort: 22, sshUser: " inner " }).sshUser, "inner");
  assert.equal(save({ remotePort: 22, sshUser: "  " }).sshUser, undefined);
  assert.equal(save({ remotePort: 3389, sshUser: "inner" }).sshUser, undefined);
});

test("a portless suffix joins a forward's name and comes off again", () => {
  assert.equal(normalizePortlessSuffix(" .Dev. "), "dev");
  assert.equal(normalizePortlessSuffix(undefined), "");
  assert.equal(domainWithSuffix("Moj-Adres ", "dev"), "moj-adres.dev");
  assert.equal(domainWithoutSuffix("moj-adres.dev", "dev"), "moj-adres");
  assert.equal(domainWithoutSuffix("a.team.dev", "team.dev"), "a");
});

test("an empty portless suffix leaves domains as they are", () => {
  assert.equal(domainWithSuffix("moj-adres", ""), "moj-adres");
  assert.equal(domainWithoutSuffix("moj-adres.dev", ""), "moj-adres.dev");
});

test("a domain without the suffix keeps its whole name, and gets the suffix on save", () => {
  assert.equal(domainWithoutSuffix("traefik.justcode.ac", "dev"), "traefik.justcode.ac");
  assert.equal(domainWithSuffix("traefik.justcode.ac", "dev"), "traefik.justcode.ac.dev");
  assert.equal(domainWithoutSuffix("dev", "dev"), "dev");
  assert.equal(domainWithoutSuffix("notdev", "dev"), "notdev");
  assert.equal(domainWithSuffix("", "dev"), "", "an empty name stays empty, so validation can ask for it");
});
