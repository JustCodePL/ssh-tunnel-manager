const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

const analyticsScripts = [
  '<script defer src="https://statystyki.justcode.ac/script.js" data-website-id="adb5be85-1f83-44a8-a215-6954adf36a93"></script>',
  '<script defer src="https://statystyki.justcode.ac/recorder.js" data-website-id="adb5be85-1f83-44a8-a215-6954adf36a93"></script>',
];

for (const page of ["index.html", "pl/index.html"]) {
  test(`${page} loads each JustCode analytics script exactly once`, () => {
    const html = fs.readFileSync(path.join(__dirname, page), "utf8");

    for (const analyticsScript of analyticsScripts) {
      assert.equal(html.split(analyticsScript).length - 1, 1);
    }
  });
}
