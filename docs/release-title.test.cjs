const assert = require("node:assert/strict");
const test = require("node:test");

const { titleForRelease } = require("./release-title.js");

const copy = {
  betaRelease: "Beta release",
  latestStable: "Latest stable release",
  stableRelease: "Stable release",
};

test("uses a custom title for a beta release", () => {
  const release = {
    name: "Safer updates and clearer port forwarding",
    tag_name: "v1.0.32-beta.2",
    prerelease: true,
  };

  assert.equal(titleForRelease(release, null, copy), release.name);
});

test("uses the beta fallback when the release name only repeats the tag", () => {
  const release = {
    name: "v1.0.32-beta.2",
    tag_name: "v1.0.32-beta.2",
    prerelease: true,
  };

  assert.equal(titleForRelease(release, null, copy), "Beta release");
});

test("uses a custom title for a stable release", () => {
  const release = {
    name: "Portless reliability update",
    tag_name: "v1.0.32",
    prerelease: false,
  };

  assert.equal(titleForRelease(release, release, copy), release.name);
});

test("keeps localized stable fallbacks for releases without custom titles", () => {
  const latest = { name: "v1.0.32", tag_name: "v1.0.32", prerelease: false };
  const older = { name: "v1.0.31", tag_name: "v1.0.31", prerelease: false };

  assert.equal(titleForRelease(latest, latest, copy), "Latest stable release");
  assert.equal(titleForRelease(older, latest, copy), "Stable release");
});
