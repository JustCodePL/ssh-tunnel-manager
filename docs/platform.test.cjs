const assert = require("node:assert/strict");
const test = require("node:test");

const { detectPlatform } = require("./platform.js");

test("detects Windows", () => {
  assert.equal(detectPlatform({ platform: "Win32" }), "windows");
});

test("detects macOS", () => {
  assert.equal(
    detectPlatform({
      platform: "MacIntel",
      userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
      maxTouchPoints: 0,
    }),
    "mac",
  );
});

test("detects desktop Linux", () => {
  assert.equal(detectPlatform({ platform: "Linux x86_64" }), "linux");
});

test("does not recommend macOS to an iPad in desktop mode", () => {
  assert.equal(
    detectPlatform({
      platform: "MacIntel",
      userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15)",
      maxTouchPoints: 5,
    }),
    "other",
  );
});

test("does not recommend Linux to Android or ChromeOS", () => {
  assert.equal(detectPlatform({ platform: "Linux armv8l", userAgent: "Android 16" }), "other");
  assert.equal(detectPlatform({ userAgentData: { platform: "Chrome OS" } }), "other");
});

test("returns other when the platform is unknown", () => {
  assert.equal(detectPlatform({}), "other");
});
