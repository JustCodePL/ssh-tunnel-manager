#!/usr/bin/env node
// Builds the news pages from scripts/changelog: one page per release in each
// language, and the news page itself, which shows the newest release.
//
//   node scripts/build-changelog.cjs           writes the pages under docs/
//   node scripts/build-changelog.cjs --check   fails if they are out of date
//
// docs/changelog-versions.js lists the releases, newest first. An entry is
// scripts/changelog/entries/<language>/<version>.html. Links in entries and page
// templates use @ROOT@ for the site root and @HOME@ for the language's home.
const fs = require("node:fs");
const path = require("node:path");

const repo = path.join(__dirname, "..");
const docs = path.join(repo, "docs");
const source = path.join(__dirname, "changelog");
const site = "https://justcodepl.github.io/ssh-tunnel-manager/";
const versions = require(path.join(docs, "changelog-versions.js"));

const languages = {
  en: {
    dir: "changelog",
    depth: 1,
    news: "News",
    newer: "Next",
    older: "Previous",
    history: "Release history",
    pager: "Older and newer releases",
    description: "What changed in each SSH Tunnel Manager release and how to use it.",
  },
  pl: {
    dir: "pl/changelog",
    depth: 2,
    news: "Aktualności",
    newer: "Następna",
    older: "Poprzednia",
    history: "Historia aktualizacji",
    pager: "Starsze i nowsze wydania",
    description: "Co się zmieniło w każdym wydaniu SSH Tunnel Manager i jak z tego korzystać.",
  },
};

function readEntry(language, version) {
  const html = fs.readFileSync(path.join(source, "entries", language, `${version}.html`), "utf8").trimEnd();
  return {
    version,
    html,
    title: plainText(html.match(/<h3>([\s\S]*?)<\/h3>/)[1]),
    intro: plainText(html.match(/<\/h3>\s*<p>([\s\S]*?)<\/p>/)?.[1] ?? ""),
    badge: html.match(/<span class="release-badge[^"]*">[^<]*<\/span>/)[0],
    date: html.match(/<time datetime="([^"]+)"/)[1],
  };
}

function plainText(html) {
  return html.replace(/<[^>]+>/g, "").replace(/\s+/g, " ").replace(/"/g, "&quot;").trim();
}

function historyLink(entry, current, home) {
  const here = entry.version === current ? ' aria-current="page"' : "";
  return `<a href="${home}/changelog/${entry.version}/"${here}><span class="changelog-history-version">${entry.version}</span> ${entry.badge} <span class="changelog-history-title">${entry.title}</span></a>`;
}

function pagerLink(version, label, className, home) {
  if (!version) return `<span class="${className}"></span>`;
  return `<a href="${home}/changelog/${version}/" class="${className}"><span class="changelog-pager-label">${label}</span><span class="changelog-pager-version">${version}</span></a>`;
}

// One page: the news page when slug is empty, otherwise the page of a release.
function renderPage(language, entries, index, slug) {
  const options = languages[language];
  const entry = entries[index];
  const up = options.depth + (slug ? 1 : 0);
  const root = Array(up).fill("..").join("/");
  const home = language === "en" ? root : Array(up - 1).fill("..").join("/");
  const pagePath = slug ? `changelog/${slug}/` : "changelog/";
  const layout = `<div class="changelog-layout">
          <nav class="changelog-history" aria-label="${options.history}">
            ${entries.map((other) => historyLink(other, entry.version, home)).join("\n            ")}
          </nav>
          <div class="changelog-content">
        ${entry.html}
            <nav class="changelog-pager" aria-label="${options.pager}">
              ${pagerLink(entries[index - 1]?.version, options.newer, "changelog-pager-newer", home)}
              ${pagerLink(entries[index + 1]?.version, options.older, "changelog-pager-older", home)}
            </nav>
          </div>
        </div>`;
  return fs
    .readFileSync(path.join(source, `page.${language}.html`), "utf8")
    .replaceAll("@TITLE@", slug ? `${entry.version}: ${entry.title} · ${options.news} · SSH Tunnel Manager` : `${options.news} · SSH Tunnel Manager`)
    .replaceAll("@DESCRIPTION@", slug ? entry.intro || entry.title : options.description)
    .replaceAll("@LAYOUT@", layout)
    .replaceAll("@PATH@", pagePath)
    .replaceAll("@ROOT@", root)
    .replaceAll("@HOME@", home);
}

function sitemapBlock(entries) {
  const urls = entries.map(({ version, date }) => {
    const en = `${site}changelog/${version}/`;
    const pl = `${site}pl/changelog/${version}/`;
    return [en, pl]
      .map(
        (loc) => `  <url>
    <loc>${loc}</loc>
    <lastmod>${date}</lastmod>
    <xhtml:link rel="alternate" hreflang="en" href="${en}" />
    <xhtml:link rel="alternate" hreflang="pl" href="${pl}" />
    <xhtml:link rel="alternate" hreflang="x-default" href="${en}" />
  </url>`,
      )
      .join("\n");
  });
  return `  <!-- changelog:start -->\n${urls.join("\n")}\n  <!-- changelog:end -->`;
}

// Every generated file, by its path under docs/.
function render() {
  const files = new Map();
  for (const language of Object.keys(languages)) {
    const entries = versions.map((version) => readEntry(language, version));
    const { dir } = languages[language];
    files.set(`${dir}/index.html`, renderPage(language, entries, 0, ""));
    entries.forEach((entry, index) => {
      files.set(`${dir}/${entry.version}/index.html`, renderPage(language, entries, index, entry.version));
    });
  }
  const sitemap = fs.readFileSync(path.join(docs, "sitemap.xml"), "utf8");
  const block = sitemapBlock(versions.map((version) => readEntry("en", version)));
  files.set("sitemap.xml", sitemap.replace(/ {2}<!-- changelog:start -->[\s\S]*<!-- changelog:end -->/, block));
  return files;
}

function outdated() {
  const stale = [];
  for (const [file, content] of render()) {
    const target = path.join(docs, file);
    if (!fs.existsSync(target) || fs.readFileSync(target, "utf8") !== content) stale.push(file);
  }
  return stale;
}

if (require.main === module) {
  if (process.argv.includes("--check")) {
    const stale = outdated();
    if (stale.length > 0) {
      console.error(`Out of date, run node scripts/build-changelog.cjs:\n${stale.join("\n")}`);
      process.exit(1);
    }
  } else {
    for (const [file, content] of render()) {
      const target = path.join(docs, file);
      fs.mkdirSync(path.dirname(target), { recursive: true });
      fs.writeFileSync(target, content);
    }
  }
}

module.exports = { outdated };
