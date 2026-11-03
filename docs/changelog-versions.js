// Releases with an entry on the changelog page. The release list links to them.
(function (root, versions) {
  if (typeof module === "object" && module.exports) {
    module.exports = versions;
    return;
  }

  root.CHANGELOG_VERSIONS = versions;
})(typeof globalThis !== "undefined" ? globalThis : this, ["v1.2.0-beta.2", "v1.2.0-beta.1"]);
