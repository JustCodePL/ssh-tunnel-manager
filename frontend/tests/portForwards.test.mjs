import test from "node:test";
import assert from "node:assert/strict";

import { preparePortForwardsForSave } from "../src/lib/portForwards.js";

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
