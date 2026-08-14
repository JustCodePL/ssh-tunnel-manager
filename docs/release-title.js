(function (root, factory) {
  const api = factory();

  if (typeof module === "object" && module.exports) {
    module.exports = api;
    return;
  }

  root.ReleaseTitle = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  function titleForRelease(release, latestStableRelease, copy) {
    const releaseName = typeof release.name === "string" ? release.name.trim() : "";

    if (releaseName && releaseName !== release.tag_name) return releaseName;
    if (release.prerelease) return copy.betaRelease;
    return release === latestStableRelease ? copy.latestStable : copy.stableRelease;
  }

  return { titleForRelease };
});
