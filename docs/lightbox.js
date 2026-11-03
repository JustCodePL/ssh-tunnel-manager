// Opens changelog screenshots in a lightbox. Without JavaScript the links open
// the image directly.
(function () {
  const polish = document.documentElement.lang.toLowerCase().startsWith("pl");
  const copy = polish
    ? { open: "Otwórz w nowej karcie", close: "Zamknij" }
    : { open: "Open in new tab", close: "Close" };

  const dialog = document.createElement("dialog");
  dialog.className = "lightbox";

  const image = document.createElement("img");

  const openLink = document.createElement("a");
  openLink.className = "lightbox-open";
  openLink.target = "_blank";
  openLink.rel = "noreferrer";
  openLink.textContent = `${copy.open} ↗`;

  const closeButton = document.createElement("button");
  closeButton.className = "lightbox-close";
  closeButton.type = "button";
  closeButton.textContent = copy.close;
  closeButton.addEventListener("click", () => dialog.close());

  const actions = document.createElement("div");
  actions.className = "lightbox-actions";
  actions.append(openLink, closeButton);

  dialog.append(actions, image);
  // A click on the backdrop lands on the dialog element itself.
  dialog.addEventListener("click", (event) => {
    if (event.target === dialog) dialog.close();
  });
  document.body.append(dialog);

  document.querySelectorAll(".changelog-entry .feature-detail-shots a").forEach((link) => {
    link.addEventListener("click", (event) => {
      if (event.metaKey || event.ctrlKey || event.shiftKey) return;
      event.preventDefault();
      image.src = link.href;
      image.alt = link.querySelector("img").alt;
      openLink.href = link.href;
      dialog.showModal();
    });
  });
})();
