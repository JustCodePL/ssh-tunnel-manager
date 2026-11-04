const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { pageForHash } = require("./changelog.js");
const { outdated } = require("../scripts/build-changelog.cjs");
const versions = require("./changelog-versions.js");

test("the news pages are built from the entries", () => {
  assert.deepEqual(outdated(), []);
});

test("an old hash link finds the release page, and anything else stays", () => {
  const links = ["https://x/changelog/v1.2.0-beta.3/", "https://x/changelog/v1.1.0/"];
  assert.equal(pageForHash(links, "#v1.1.0"), links[1]);
  assert.equal(pageForHash(links, "#v1.2.0-beta.3"), links[0]);
  assert.equal(pageForHash(links, ""), null);
  assert.equal(pageForHash(links, undefined), null);
  assert.equal(pageForHash(links, "#missing"), null);
  assert.equal(pageForHash(links, "#%E0%A4%A"), null);
});

const languages = [
  { dir: "changelog", heading: "News", newer: "Next", older: "Previous", prefix: "" },
  { dir: "pl/changelog", heading: "Aktualności", newer: "Następna", older: "Poprzednia", prefix: "pl/" },
];
const site = "https://justcodepl.github.io/ssh-tunnel-manager/";
const read = (page) => fs.readFileSync(path.join(__dirname, page), "utf8");
const articles = (html) => [...html.matchAll(/<article class="changelog-entry" id="([^"]+)"/g)].map((m) => m[1]);
const history = (html) => html.match(/<nav class="changelog-history"[\s\S]*?<\/nav>/)[0];
const pager = (html) => html.match(/<nav class="changelog-pager"[\s\S]*?<\/nav>/)[0];

for (const { dir, heading, newer, older, prefix } of languages) {
  test(`${dir} opens on the newest release under a plain heading`, () => {
    const html = read(`${dir}/index.html`);
    const intro = html.match(/<div class="section-intro">([\s\S]*?)<\/div>/)[1];
    assert.ok(intro.includes(`<h2>${heading}</h2>`));
    assert.equal((intro.match(/<p\b/g) ?? []).length, 1, "only the kicker stays above the heading");
    assert.deepEqual(articles(html), [versions[0]]);
    assert.match(html, /<script src="\.\.\/(\.\.\/)?changelog\.js[^"]*" defer>/);
  });

  versions.forEach((version, index) => {
    const page = `${dir}/${version}/index.html`;

    test(`${page} shows only its release`, () => {
      assert.deepEqual(articles(read(page)), [version]);
    });

    test(`${page} has its own title and address`, () => {
      const html = read(page);
      assert.ok(html.match(/<title>(.*?)<\/title>/)[1].startsWith(`${version}: `));
      assert.ok(html.includes(`<link rel="canonical" href="${site}${prefix}changelog/${version}/" />`));
      assert.ok(html.includes(`hreflang="en" href="${site}changelog/${version}/"`));
      assert.ok(html.includes(`hreflang="pl" href="${site}pl/changelog/${version}/"`));
    });

    test(`${page} lists every release, newest first, and marks its own`, () => {
      const nav = history(read(page));
      const links = [...nav.matchAll(/href="(?:\.\.\/)+changelog\/([^"/]+)\/"/g)].map((m) => m[1]);
      assert.deepEqual(links, versions);
      assert.deepEqual([...nav.matchAll(/href="(?:\.\.\/)+changelog\/([^"/]+)\/" aria-current="page"/g)].map((m) => m[1]), [version]);
    });

    test(`${page} links to the releases around it`, () => {
      const nav = pager(read(page));
      const newerRelease = versions[index - 1];
      const olderRelease = versions[index + 1];
      assert.equal(nav.includes(`>${newer}<`), Boolean(newerRelease));
      assert.equal(nav.includes(`>${older}<`), Boolean(olderRelease));
      if (newerRelease) assert.ok(nav.includes(`changelog/${newerRelease}/" class="changelog-pager-newer"`));
      if (olderRelease) assert.ok(nav.includes(`changelog/${olderRelease}/" class="changelog-pager-older"`));
    });

    test(`${page} has no broken local links`, () => {
      const html = read(page);
      for (const match of html.matchAll(/(?:href|src)="(\.[^"#?]*)/g)) {
        const target = path.join(__dirname, path.dirname(page), match[1]);
        assert.ok(fs.existsSync(target), `missing ${match[1]}`);
      }
    });
  });
}

test("the sitemap lists every release page in both languages", () => {
  const sitemap = read("sitemap.xml");
  for (const version of versions) {
    assert.ok(sitemap.includes(`<loc>${site}changelog/${version}/</loc>`));
    assert.ok(sitemap.includes(`<loc>${site}pl/changelog/${version}/</loc>`));
  }
});

test("the release list links to a release's own page", () => {
  assert.ok(read("app.js").includes("changelogLink.href = `changelog/${release.tag_name}/`"));
});
