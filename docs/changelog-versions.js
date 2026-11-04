// Releases with an entry on the changelog page. The release list links to them.
(function (root, versions) {
  if (typeof module === "object" && module.exports) {
    module.exports = versions;
    return;
  }

  root.CHANGELOG_VERSIONS = versions;
})(typeof globalThis !== "undefined" ? globalThis : this, [
  "v1.2.0-beta.4",
  "v1.2.0-beta.3",
  "v1.2.0-beta.2",
  "v1.2.0-beta.1",
  "v1.1.0",
  "v1.0.36",
  "v1.0.31",
  "v1.0.30",
  "v1.0.29",
  "v1.0.28",
  "v1.0.27",
  "v1.0.26",
  "v1.0.25",
  "v1.0.24",
  "v1.0.23",
  "v1.0.22",
  "v1.0.21",
  "v1.0.20",
  "v1.0.19",
  "v1.0.18",
  "v1.0.17",
  "v1.0.16",
  "v1.0.15",
  "v1.0.14",
  "v1.0.13",
  "v1.0.12",
  "v1.0.11",
  "v1.0.10",
  "v1.0.9",
  "v1.0.8",
  "v1.0.7",
  "v1.0.6",
  "v1.0.5",
  "v1.0.4",
  "v1.0.3",
  "v1.0.2",
  "v1.0.1",
  "v1.0.0",
]);
