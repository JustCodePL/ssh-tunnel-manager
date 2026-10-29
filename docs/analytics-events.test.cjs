const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

const { downloadEventAttributes, applyAttributes } = require("./analytics-events.js");

function readPage(page) {
  return fs.readFileSync(path.join(__dirname, page), "utf8");
}

// Returns the opening tag of every <a> element whose attributes match the predicate.
function anchors(html, predicate) {
  return [...html.matchAll(/<a\b[^>]*>/gs)].map((match) => match[0]).filter(predicate);
}

for (const page of ["index.html", "pl/index.html"]) {
  test(`${page} tracks the hero download button`, () => {
    const [hero] = anchors(readPage(page), (tag) => tag.includes("download-auto"));
    assert.ok(hero);
    assert.match(hero, /data-umami-event="download"/);
    assert.match(hero, /data-umami-event-location="hero"/);
  });

  test(`${page} tracks every platform card download with its file name`, () => {
    const links = anchors(readPage(page), (tag) => tag.includes("data-asset="));
    assert.equal(links.length, 5);

    for (const link of links) {
      const asset = link.match(/data-asset="([^"]+)"/)[1];
      assert.match(link, /data-umami-event="download"/);
      assert.match(link, /data-umami-event-location="platform-card"/);
      assert.ok(link.includes(`data-umami-event-file="${asset}"`), `missing file for ${asset}`);
    }
  });

  test(`${page} tracks GitHub links in the header, hero, contact and footer`, () => {
    const links = anchors(readPage(page), (tag) => tag.includes('data-umami-event="github-click"'));
    const locations = links.map((tag) => tag.match(/data-umami-event-location="([^"]+)"/)[1]);

    for (const location of ["header", "hero", "contact", "footer"]) {
      assert.ok(locations.includes(location), `missing github-click at ${location}`);
    }
  });
}

for (const page of ["compare/index.html", "pl/compare/index.html"]) {
  test(`${page} tracks GitHub links`, () => {
    const links = anchors(readPage(page), (tag) => tag.includes('data-umami-event="github-click"'));
    assert.ok(links.length >= 3);
  });
}

for (const page of ["index.html", "pl/index.html", "compare/index.html", "pl/compare/index.html"]) {
  test(`${page} tracks the Features link in the header and footer`, () => {
    const links = anchors(readPage(page), (tag) => tag.includes('data-umami-event="features-nav"'));
    const locations = links.map((tag) => tag.match(/data-umami-event-location="([^"]+)"/)[1]).sort();
    assert.deepEqual(locations, ["footer", "header"]);
    for (const link of links) assert.match(link, /href="(\.\.\/)?features\/"/);
  });
}

const FEATURE_SECTIONS = [
  "ssh-config",
  "local-forwarding",
  "proxyjump",
  "tray",
  "reconnect-startup",
  "groups-pin",
  "resources",
  "docker",
  "terminal-files",
  "logs-htop",
  "portless",
  "auth",
  "updates",
];

for (const page of ["features/index.html", "pl/features/index.html"]) {
  test(`${page} has every feature section in order`, () => {
    const ids = [...readPage(page).matchAll(/<section class="feature-detail[^"]*" id="([^"]+)"/g)].map((m) => m[1]);
    assert.deepEqual(ids, FEATURE_SECTIONS);
  });

  test(`${page} shows a screenshot or example for every feature`, () => {
    const sections = readPage(page).split('<section class="feature-detail').slice(1);
    assert.equal(sections.length, FEATURE_SECTIONS.length);
    for (const section of sections) {
      const id = section.match(/id="([^"]+)"/)[1];
      assert.ok(/<img\b|<pre class="feature-code"/.test(section), `${id} has no screenshot`);
    }
  });

  test(`${page} gives every screenshot alt text and dimensions that exist on disk`, () => {
    const html = readPage(page);
    const images = [...html.matchAll(/<img\b[^>]*>/g)].map((m) => m[0]).filter((tag) => tag.includes("screenshots/"));
    assert.ok(images.length > 0);
    for (const tag of images) {
      assert.match(tag, /alt="[^"]{10,}"/);
      assert.match(tag, /width="\d+" height="\d+"/);
      const src = tag.match(/src="([^"]+)"/)[1];
      assert.ok(fs.existsSync(path.join(__dirname, path.dirname(page), src)), `missing ${src}`);
    }
  });

  test(`${page} tracks the GitHub call to action`, () => {
    const links = anchors(readPage(page), (tag) => tag.includes('data-umami-event-location="features-cta"'));
    assert.equal(links.length, 1);
    assert.match(links[0], /data-umami-event="github-click"/);
  });
}

test("builds release list download attributes with file and version", () => {
  assert.deepEqual(
    downloadEventAttributes({
      location: "release-list",
      file: "ssh-tunnel-manager-darwin-arm64.dmg",
      version: "v1.0.36",
    }),
    {
      "data-umami-event": "download",
      "data-umami-event-location": "release-list",
      "data-umami-event-file": "ssh-tunnel-manager-darwin-arm64.dmg",
      "data-umami-event-version": "v1.0.36",
    },
  );
});

test("omits attributes without a value", () => {
  assert.deepEqual(downloadEventAttributes({ location: "hero", file: undefined, version: "v1.0.36", platform: "mac" }), {
    "data-umami-event": "download",
    "data-umami-event-location": "hero",
    "data-umami-event-version": "v1.0.36",
    "data-umami-event-platform": "mac",
  });
});

test("applies attributes to an element", () => {
  const set = {};
  applyAttributes({ setAttribute: (name, value) => (set[name] = value) }, { "data-umami-event": "download" });
  assert.deepEqual(set, { "data-umami-event": "download" });
});
