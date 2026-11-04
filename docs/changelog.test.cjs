const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { entryForHash, neighbours } = require("./changelog.js");

const ids = ["v3", "v2", "v1"];

test("a hash opens its entry, and anything else the newest", () => {
  assert.equal(entryForHash(ids, "#v2"), "v2");
  assert.equal(entryForHash(ids, ""), "v3");
  assert.equal(entryForHash(ids, undefined), "v3");
  assert.equal(entryForHash(ids, "#missing"), "v3");
  assert.equal(entryForHash(ids, "#%E0%A4%A"), "v3");
  assert.equal(entryForHash(["v1.2.0-beta.3"], "#v1.2.0-beta.3"), "v1.2.0-beta.3");
});

test("neighbours are the newer and older releases, and stop at the ends", () => {
  assert.deepEqual(neighbours(ids, "v2"), { newer: "v3", older: "v1" });
  assert.deepEqual(neighbours(ids, "v3"), { newer: null, older: "v2" });
  assert.deepEqual(neighbours(ids, "v1"), { newer: "v2", older: null });
  assert.deepEqual(neighbours(ids, "missing"), { newer: null, older: null });
});

const pages = [
  { page: "changelog/index.html", heading: "News", newer: "Next", older: "Previous" },
  { page: "pl/changelog/index.html", heading: "Aktualności", newer: "Następna", older: "Poprzednia" },
];

for (const { page, heading, newer, older } of pages) {
  const html = fs.readFileSync(path.join(__dirname, page), "utf8");

  test(`${page} has a plain heading without a subheading`, () => {
    const intro = html.match(/<div class="section-intro">([\s\S]*?)<\/div>/)[1];
    assert.ok(intro.includes(`<h2>${heading}</h2>`));
    assert.equal((intro.match(/<p\b/g) ?? []).length, 1, "only the kicker stays above the heading");
  });

  test(`${page} sets up one entry at a time`, () => {
    assert.match(html, /<script src="\.\.\/(\.\.\/)?changelog\.js[^"]*" defer>/);
    assert.ok(html.includes(`data-newer="${newer}"`));
    assert.ok(html.includes(`data-older="${older}"`));
    assert.ok(html.includes('<nav class="changelog-history"'));
    assert.ok(html.includes('<nav class="changelog-pager"'));
  });
}
