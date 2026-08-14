const REPOSITORY = "JustCodePL/ssh-tunnel-manager";
const RELEASES_URL = `https://github.com/${REPOSITORY}/releases`;
const API_URL = `https://api.github.com/repos/${REPOSITORY}/releases`;
const PAGE_SIZE = 100;
const INITIAL_RELEASE_COUNT = 6;
const IS_POLISH = document.documentElement.lang.toLowerCase().startsWith("pl");
const COPY = IS_POLISH
  ? {
      locale: "pl-PL",
      installer: "instalator",
      releaseDetails: "Szczegóły zmian są dostępne na stronie wydania.",
      betaRelease: "Wydanie beta",
      latestStable: "Najnowsze wydanie stabilne",
      stableRelease: "Wydanie stabilne",
      recommended: "POLECANE",
      downloadAria: (label, version) => `Pobierz ${label}, wersja ${version}`,
      viewAssets: "Zobacz pliki wydania ↗",
      latestLabel: (version) => `Najnowsza stabilna wersja ${version}`,
      platformDownload: {
        windows: (version) => `Pobierz dla Windows · ${version}`,
        mac: (version) => `Pobierz dla macOS · ${version}`,
        linux: (version) => `Pobierz dla Linux · ${version}`,
        other: (version) => `Pobierz aplikację · ${version}`,
      },
      releaseError:
        "Nie udało się teraz pobrać historii wydań. Wszystkie wersje i pliki instalacyjne znajdziesz bezpośrednio w",
    }
  : {
      locale: "en-GB",
      installer: "installer",
      releaseDetails: "Release details are available on the release page.",
      betaRelease: "Beta release",
      latestStable: "Latest stable release",
      stableRelease: "Stable release",
      recommended: "RECOMMENDED",
      downloadAria: (label, version) => `Download ${label}, version ${version}`,
      viewAssets: "View release files ↗",
      latestLabel: (version) => `Latest stable release ${version}`,
      platformDownload: {
        windows: (version) => `Download for Windows · ${version}`,
        mac: (version) => `Download for macOS · ${version}`,
        linux: (version) => `Download for Linux · ${version}`,
        other: (version) => `Download the app · ${version}`,
      },
      releaseError:
        "The release history could not be loaded right now. Every version and installer is available directly from",
    };

let releases = [];
let activeFilter = "stable";
let visibleReleaseCount = INITIAL_RELEASE_COUNT;

const releaseList = document.querySelector("#release-list");
const loadMoreButton = document.querySelector("#load-more");
const filterButtons = document.querySelectorAll("[data-filter]");

const detectedPlatform = window.PlatformDetection.detectPlatform(navigator);

function updatePlatformRecommendation() {
  const cards = [...document.querySelectorAll(".platform-card[data-platform]")];

  cards.forEach((card) => {
    const isRecommended = card.dataset.platform === detectedPlatform;
    card.classList.toggle("featured", isRecommended);

    const button = card.querySelector("[data-platform-cta]");
    button?.classList.toggle("button-primary", isRecommended);
    button?.classList.toggle("button-secondary", !isRecommended);
  });

  const recommendedCard = cards.find((card) => card.dataset.platform === detectedPlatform);
  if (!recommendedCard) return;

  const badge = document.createElement("div");
  badge.className = "recommended";
  badge.textContent = COPY.recommended;
  recommendedCard.prepend(badge);
}

function classifyAsset(name) {
  const value = name.toLowerCase();
  if (value.includes("installer") && value.endsWith(".exe")) return { label: `Windows · ${COPY.installer}`, platform: "windows" };
  if (value.includes("windows") && value.endsWith(".zip")) return { label: "Windows · portable", platform: "windows" };
  if (value.includes("darwin-arm64") || value.includes("macos-arm64")) return { label: "macOS · Apple Silicon", platform: "mac" };
  if (value.includes("darwin-amd64") || value.includes("macos-amd64")) return { label: "macOS · Intel", platform: "mac" };
  if (value.includes("darwin") || value.includes("macos")) return { label: "macOS", platform: "mac" };
  if (value.includes("linux")) return { label: "Linux · x64", platform: "linux" };
  return { label: name, platform: "other" };
}

function formatDate(value) {
  return new Intl.DateTimeFormat(COPY.locale, {
    day: "numeric",
    month: "long",
    year: "numeric",
  }).format(new Date(value));
}

function cleanReleaseBody(body) {
  if (!body) return COPY.releaseDetails;

  const cleaned = body
    .replace(/<!--.*?-->/gs, " ")
    .replace(/```.*?```/gs, " ")
    .replace(/!\[[^\]]*\]\([^)]*\)/g, " ")
    .replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
    .replace(/^#{1,6}\s+/gm, "")
    .replace(/^[-*+]\s+/gm, "")
    .replace(/[*_`>#]/g, "")
    .replace(/https?:\/\/\S+/g, "")
    .replace(/\s+/g, " ")
    .trim();

  if (!cleaned) return COPY.releaseDetails;
  return cleaned.length > 175 ? `${cleaned.slice(0, 172).trim()}…` : cleaned;
}

function createReleaseRow(release) {
  const row = document.createElement("article");
  row.className = "release-row";

  const version = document.createElement("div");
  version.className = "release-version";
  const versionLink = document.createElement("a");
  versionLink.href = release.html_url;
  versionLink.target = "_blank";
  versionLink.rel = "noreferrer";
  versionLink.textContent = release.tag_name;
  version.append(versionLink);

  const badge = document.createElement("span");
  badge.className = `release-badge${release.prerelease ? " beta" : ""}`;
  badge.textContent = release.prerelease ? "BETA" : release === releases.find((item) => !item.prerelease) ? "LATEST" : "STABLE";
  version.append(badge);

  const description = document.createElement("div");
  description.className = "release-description";
  const title = document.createElement("h3");
  title.textContent = window.ReleaseTitle.titleForRelease(
    release,
    releases.find((item) => !item.prerelease),
    COPY,
  );
  const summary = document.createElement("p");
  summary.textContent = cleanReleaseBody(release.body);
  const date = document.createElement("time");
  date.className = "release-date";
  date.dateTime = release.published_at;
  date.textContent = formatDate(release.published_at);
  description.append(title, summary, date);

  const assets = document.createElement("div");
  assets.className = "release-assets";
  const orderedAssets = [...release.assets].sort((left, right) => {
    return (
      Number(classifyAsset(right.name).platform === detectedPlatform) -
      Number(classifyAsset(left.name).platform === detectedPlatform)
    );
  });

  orderedAssets.forEach((asset) => {
    const metadata = classifyAsset(asset.name);
    const link = document.createElement("a");
    link.className = "asset-button";
    link.href = asset.browser_download_url;
    link.setAttribute("aria-label", COPY.downloadAria(metadata.label, release.tag_name));
    const icon = document.createElement("span");
    icon.setAttribute("aria-hidden", "true");
    icon.textContent = "↓";
    link.append(icon, document.createTextNode(metadata.label));
    assets.append(link);
  });

  if (!orderedAssets.length) {
    const link = document.createElement("a");
    link.className = "asset-button";
    link.href = release.html_url;
    link.textContent = COPY.viewAssets;
    assets.append(link);
  }

  row.append(version, description, assets);
  return row;
}

function filteredReleases() {
  return activeFilter === "stable" ? releases.filter((release) => !release.prerelease) : releases;
}

function renderReleases() {
  const filtered = filteredReleases();
  const visible = filtered.slice(0, visibleReleaseCount);
  releaseList.replaceChildren(...visible.map(createReleaseRow));
  releaseList.setAttribute("aria-busy", "false");
  loadMoreButton.hidden = visible.length >= filtered.length;
}

function updateLatestDownloads(latest) {
  document.querySelector("#latest-label").textContent = COPY.latestLabel(latest.tag_name);

  document.querySelectorAll(".asset-download[data-asset]").forEach((link) => {
    const asset = latest.assets.find((item) => item.name === link.dataset.asset);
    if (asset) link.href = asset.browser_download_url;
  });

  const preferred = {
    windows: "ssh-tunnel-manager-amd64-installer.exe",
    mac: "ssh-tunnel-manager-darwin-arm64.dmg",
    linux: "ssh-tunnel-manager-linux-amd64.tar.gz",
  }[detectedPlatform];
  const preferredAsset = latest.assets.find((asset) => asset.name === preferred);

  document.querySelectorAll(".download-auto").forEach((link) => {
    link.href = preferredAsset?.browser_download_url || latest.html_url;
    link.querySelector(".auto-download-label").textContent =
      COPY.platformDownload[detectedPlatform](latest.tag_name);
  });
}

async function fetchAllReleases() {
  const collected = [];
  for (let page = 1; page <= 3; page += 1) {
    const response = await fetch(`${API_URL}?per_page=${PAGE_SIZE}&page=${page}`, {
      headers: { Accept: "application/vnd.github+json" },
    });
    if (!response.ok) throw new Error(`GitHub API returned ${response.status}`);
    const pageItems = await response.json();
    collected.push(...pageItems.filter((release) => !release.draft));
    if (pageItems.length < PAGE_SIZE) break;
  }
  return collected;
}

async function initializeReleases() {
  try {
    releases = await fetchAllReleases();
    const latest = releases.find((release) => !release.prerelease);
    if (latest) updateLatestDownloads(latest);
    renderReleases();
  } catch (error) {
    console.error("Could not load GitHub releases", error);
    releaseList.setAttribute("aria-busy", "false");
    releaseList.innerHTML = `
      <p class="release-error">
        ${COPY.releaseError} <a href="${RELEASES_URL}">GitHub Releases</a>.
      </p>`;
    loadMoreButton.hidden = true;
  }
}

filterButtons.forEach((button) => {
  button.addEventListener("click", () => {
    activeFilter = button.dataset.filter;
    visibleReleaseCount = INITIAL_RELEASE_COUNT;
    filterButtons.forEach((item) => {
      const isActive = item === button;
      item.classList.toggle("active", isActive);
      item.setAttribute("aria-pressed", String(isActive));
    });
    renderReleases();
  });
});

loadMoreButton.addEventListener("click", () => {
  visibleReleaseCount += INITIAL_RELEASE_COUNT;
  renderReleases();
});

document.querySelector("#current-year").textContent = new Date().getFullYear();
updatePlatformRecommendation();
initializeReleases();
