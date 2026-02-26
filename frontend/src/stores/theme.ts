import { writable } from "svelte/store";

export type Theme = "dark" | "light" | "system";

const STORAGE_KEY = "stm-theme";

function getInitialTheme(): Theme {
  const stored = localStorage.getItem(STORAGE_KEY);
  if (stored === "dark" || stored === "light" || stored === "system") {
    return stored;
  }
  return "dark";
}

function resolveTheme(theme: Theme): "dark" | "light" {
  if (theme === "system") {
    return window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
  }
  return theme;
}

function applyTheme(theme: Theme) {
  const resolved = resolveTheme(theme);
  if (resolved === "dark") {
    document.documentElement.classList.add("dark");
  } else {
    document.documentElement.classList.remove("dark");
  }
}

export const theme = writable<Theme>(getInitialTheme());

// Apply on init
applyTheme(getInitialTheme());

// React to changes
theme.subscribe((t) => {
  localStorage.setItem(STORAGE_KEY, t);
  applyTheme(t);
});

// Listen for system preference changes
window
  .matchMedia("(prefers-color-scheme: dark)")
  .addEventListener("change", () => {
    let current: Theme = "dark";
    theme.subscribe((t) => (current = t))();
    if (current === "system") {
      applyTheme("system");
    }
  });
