(function (root, factory) {
  const api = factory();

  if (typeof module === "object" && module.exports) {
    module.exports = api;
    return;
  }

  root.AnalyticsEvents = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  // Umami reads data-umami-event* attributes from the clicked element, so
  // links created at runtime only need the attributes set before a click.
  function downloadEventAttributes({ location, file, version, platform }) {
    const attributes = {
      "data-umami-event": "download",
      "data-umami-event-location": location,
    };
    if (file) attributes["data-umami-event-file"] = file;
    if (version) attributes["data-umami-event-version"] = version;
    if (platform) attributes["data-umami-event-platform"] = platform;
    return attributes;
  }

  function applyAttributes(element, attributes) {
    Object.entries(attributes).forEach(([name, value]) => element.setAttribute(name, value));
  }

  return { downloadEventAttributes, applyAttributes };
});
