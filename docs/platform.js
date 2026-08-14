(function (root, factory) {
  const api = factory();

  if (typeof module === "object" && module.exports) {
    module.exports = api;
    return;
  }

  root.PlatformDetection = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  function detectPlatform(environment = {}) {
    const userAgentPlatform = environment.userAgentData?.platform || "";
    const legacyPlatform = environment.platform || "";
    const userAgent = environment.userAgent || "";
    const source = `${userAgentPlatform} ${legacyPlatform} ${userAgent}`.toLowerCase();

    const isIPadInDesktopMode =
      legacyPlatform.toLowerCase() === "macintel" && Number(environment.maxTouchPoints) > 1;
    const isMobileOrChromeOS = /android|iphone|ipad|ipod|cros|chrome os/.test(source);

    if (isIPadInDesktopMode || isMobileOrChromeOS) return "other";
    if (/windows|win32|win64/.test(source)) return "windows";
    if (/macintosh|mac os|macintel/.test(source)) return "mac";
    if (source.includes("linux")) return "linux";
    return "other";
  }

  return { detectPlatform };
});
