// Opens Settings, then switches to Graphite, to show the title bar in both themes.
(function () {
  function click(testid, delay) {
    setTimeout(function () {
      var element = document.querySelector('[data-testid="' + testid + '"]');
      if (element) element.click();
    }, delay);
  }
  if (new URLSearchParams(location.search).has("window")) {
    click("settings-appearance", 6000);
    click("theme-graphite", 12000);
  } else {
    click("open-settings", 8000);
  }
})();
