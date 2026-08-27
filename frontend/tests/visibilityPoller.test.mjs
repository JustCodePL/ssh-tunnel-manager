import test from "node:test";
import assert from "node:assert/strict";

import { createVisibilityPoller } from "../src/lib/visibilityPoller.js";

class FakeDocument {
  constructor(hidden = false) {
    this.hidden = hidden;
    this.listeners = new Set();
  }

  addEventListener(_type, listener) {
    this.listeners.add(listener);
  }

  removeEventListener(_type, listener) {
    this.listeners.delete(listener);
  }

  setHidden(hidden) {
    this.hidden = hidden;
    for (const listener of this.listeners) listener();
  }
}

class FakeClock {
  constructor() {
    this.nextID = 1;
    this.timers = new Map();
  }

  setTimer(callback) {
    const id = this.nextID++;
    this.timers.set(id, callback);
    return id;
  }

  clearTimer(id) {
    this.timers.delete(id);
  }

  runNext() {
    const entry = this.timers.entries().next().value;
    if (!entry) return false;
    const [id, callback] = entry;
    this.timers.delete(id);
    callback();
    return true;
  }
}

function flushPromises() {
  return new Promise((resolve) => setImmediate(resolve));
}

test("does not poll while hidden and refreshes immediately when visible", async () => {
  const documentRef = new FakeDocument(true);
  const clock = new FakeClock();
  let calls = 0;
  const poller = createVisibilityPoller(
    () => {
      calls += 1;
    },
    3000,
    {
      documentRef,
      setTimer: clock.setTimer.bind(clock),
      clearTimer: clock.clearTimer.bind(clock),
    },
  );

  poller.start();
  await flushPromises();
  assert.equal(calls, 0);
  assert.equal(clock.timers.size, 0);

  documentRef.setHidden(false);
  await flushPromises();
  assert.equal(calls, 1);
  assert.equal(clock.timers.size, 1);

  documentRef.setHidden(true);
  assert.equal(clock.timers.size, 0);

  documentRef.setHidden(false);
  await flushPromises();
  assert.equal(calls, 2);

  poller.stop();
  assert.equal(clock.timers.size, 0);
  assert.equal(documentRef.listeners.size, 0);
});

test("schedules the next poll only after the current poll completes", async () => {
  const documentRef = new FakeDocument(false);
  const clock = new FakeClock();
  const resolvers = [];
  let running = 0;
  let maxRunning = 0;
  let calls = 0;
  const poller = createVisibilityPoller(
    () => {
      calls += 1;
      running += 1;
      maxRunning = Math.max(maxRunning, running);
      return new Promise((resolve) => {
        resolvers.push(() => {
          running -= 1;
          resolve();
        });
      });
    },
    3000,
    {
      documentRef,
      setTimer: clock.setTimer.bind(clock),
      clearTimer: clock.clearTimer.bind(clock),
    },
  );

  poller.start();
  assert.equal(calls, 1);
  assert.equal(clock.timers.size, 0);

  resolvers.shift()();
  await flushPromises();
  assert.equal(clock.timers.size, 1);

  assert.equal(clock.runNext(), true);
  assert.equal(calls, 2);
  assert.equal(clock.timers.size, 0);
  assert.equal(maxRunning, 1);

  resolvers.shift()();
  await flushPromises();
  assert.equal(clock.timers.size, 1);
  poller.stop();
});

test("pauses and immediately resumes an enabled poller", async () => {
  const documentRef = new FakeDocument(false);
  const clock = new FakeClock();
  let calls = 0;
  const poller = createVisibilityPoller(
    () => {
      calls += 1;
    },
    5000,
    {
      documentRef,
      setTimer: clock.setTimer.bind(clock),
      clearTimer: clock.clearTimer.bind(clock),
    },
  );

  poller.start();
  await flushPromises();
  assert.equal(calls, 1);
  assert.equal(clock.timers.size, 1);

  poller.setEnabled(false);
  assert.equal(clock.timers.size, 0);

  poller.setEnabled(true);
  await flushPromises();
  assert.equal(calls, 2);
  assert.equal(clock.timers.size, 1);
  poller.stop();
});
