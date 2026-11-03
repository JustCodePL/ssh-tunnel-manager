// Opens the RDP view on the connected host and logs in, pausing on each state
// so the capture script gets a frame of it. Hides real addresses and users.
(function () {
  const login = "@LOGIN@";
  const password = "@PASSWORD@";
  const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

  async function until(find) {
    for (;;) {
      const found = find();
      if (found) return found;
      await wait(250);
    }
  }

  const buttonNamed = (name) =>
    [...document.querySelectorAll("button")].find((b) => b.textContent.trim() === name && !b.disabled);

  function setValue(input, value) {
    input.value = value;
    input.dispatchEvent(new Event("input", { bubbles: true }));
  }

  setInterval(() => {
    document.querySelectorAll(".card-host, .rdp-title, .forward-tag").forEach((el) => {
      const masked = el.textContent.replace(/\S+@[^\s:]+/, "artur@lexar").replace(/\d+\.\d+\.\d+\.\d+/g, "lexar");
      if (masked !== el.textContent) el.textContent = masked;
    });
  }, 50);

  // The window may be covered while capturing, which would pause the session.
  Object.defineProperty(document, "hidden", { get: () => false });
  Object.defineProperty(document, "visibilityState", { get: () => "visible" });

  (async () => {
    (await until(() => buttonNamed("rdp"))) && (await wait(4000));
    buttonNamed("rdp").click();

    const form = await until(() => document.querySelector(".rdp-form"));
    const [user, , pass] = form.querySelectorAll("input:not([type=checkbox])");
    setValue(user, login);
    const remember = form.querySelector("input[type=checkbox]");
    remember.checked = false;
    remember.dispatchEvent(new Event("change", { bubbles: true }));
    await wait(4000);

    setValue(pass, password);
    form.querySelector("button[type=submit]").click();
  })();
})();
