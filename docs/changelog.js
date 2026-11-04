// Shows one release at a time on the news page, with the history in a sidebar
// and links to the releases around it. Without JavaScript every entry is listed.
(function (root, factory) {
  const api = factory();

  if (typeof module === "object" && module.exports) {
    module.exports = api;
    return;
  }

  root.ChangelogPager = api;
  if (root.document) {
    if (root.document.readyState === "loading") {
      root.document.addEventListener("DOMContentLoaded", () => api.start(root));
    } else {
      api.start(root);
    }
  }
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  // The entry a URL hash asks for, or the newest one.
  function entryForHash(ids, hash) {
    const id = decodeHash(hash);
    return ids.includes(id) ? id : ids[0];
  }

  function decodeHash(hash) {
    try {
      return decodeURIComponent(String(hash || "").replace(/^#/, ""));
    } catch {
      return "";
    }
  }

  // ids run from newest to oldest.
  function neighbours(ids, id) {
    const index = ids.indexOf(id);
    if (index < 0) return { newer: null, older: null };
    return { newer: ids[index - 1] ?? null, older: ids[index + 1] ?? null };
  }

  function start(window) {
    const document = window.document;
    const layout = document.querySelector("[data-changelog]");
    if (!layout) return;
    const articles = [...layout.querySelectorAll(".changelog-entry")];
    if (articles.length === 0) return;
    const ids = articles.map((article) => article.id);
    const history = layout.querySelector(".changelog-history");
    const pager = layout.querySelector(".changelog-pager");

    const links = new Map();
    for (const article of articles) {
      const link = document.createElement("a");
      link.href = `#${article.id}`;
      const version = document.createElement("span");
      version.className = "changelog-history-version";
      version.textContent = article.id;
      link.append(version);
      const badge = article.querySelector(".changelog-meta .release-badge");
      if (badge) link.append(badge.cloneNode(true));
      const title = document.createElement("span");
      title.className = "changelog-history-title";
      title.textContent = article.querySelector("h3")?.textContent ?? "";
      link.append(title);
      history.append(link);
      links.set(article.id, link);
    }

    function pagerLink(id, label, className) {
      if (!id) {
        const empty = document.createElement("span");
        empty.className = className;
        return empty;
      }
      const link = document.createElement("a");
      link.href = `#${id}`;
      link.className = className;
      const name = document.createElement("span");
      name.className = "changelog-pager-label";
      name.textContent = label;
      const version = document.createElement("span");
      version.className = "changelog-pager-version";
      version.textContent = id;
      link.append(name, version);
      return link;
    }

    function show(scroll) {
      const current = entryForHash(ids, window.location.hash);
      for (const article of articles) {
        article.classList.toggle("is-current", article.id === current);
      }
      for (const [id, link] of links) {
        if (id === current) {
          link.setAttribute("aria-current", "true");
          // Only the sidebar scrolls here, the page stays where it is.
          const top = link.offsetTop - history.offsetTop;
          if (top < history.scrollTop || top + link.offsetHeight > history.scrollTop + history.clientHeight) {
            history.scrollTop = top - history.clientHeight / 3;
          }
        } else {
          link.removeAttribute("aria-current");
        }
      }
      const { newer, older } = neighbours(ids, current);
      pager.replaceChildren(
        pagerLink(newer, layout.dataset.newer, "changelog-pager-newer"),
        pagerLink(older, layout.dataset.older, "changelog-pager-older"),
      );
      if (scroll) {
        const header = document.querySelector(".site-header")?.offsetHeight ?? 0;
        window.scrollTo({ top: layout.getBoundingClientRect().top + window.scrollY - header - 16, behavior: "instant" });
      }
    }

    layout.classList.add("is-ready");
    show(Boolean(window.location.hash));
    window.addEventListener("hashchange", () => show(true));
  }

  return { entryForHash, neighbours, start };
});
