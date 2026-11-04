// Old links to a release used a hash on the news page, and published release
// notes still have them, so the news page sends those to the release's own page.
(function (root, factory) {
  const api = factory();

  if (typeof module === "object" && module.exports) {
    module.exports = api;
    return;
  }

  if (root.document) api.start(root);
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  // The release page for a hash such as #v1.2.0, if that release is listed.
  function pageForHash(links, hash) {
    let version;
    try {
      version = decodeURIComponent(String(hash || "").replace(/^#/, ""));
    } catch {
      return null;
    }
    if (!version) return null;
    return links.find((link) => link.endsWith(`/${version}/`)) ?? null;
  }

  function start(window) {
    const anchors = [...window.document.querySelectorAll(".changelog-history a")];
    const page = pageForHash(anchors.map((anchor) => anchor.href), window.location.hash);
    if (page) {
      window.location.replace(page);
      return;
    }
    // Keep the open release in view in a long history, without moving the page.
    const history = window.document.querySelector(".changelog-history");
    const current = history?.querySelector("[aria-current]");
    if (current && current.offsetTop - history.offsetTop > history.clientHeight / 2) {
      history.scrollTop = current.offsetTop - history.offsetTop - history.clientHeight / 3;
    }
  }

  return { pageForHash, start };
});
