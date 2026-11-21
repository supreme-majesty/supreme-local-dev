/**
 * Dashboard theme: the viewer's preference ("system" follows the OS) and the
 * resolved theme applied as <html data-theme="dark|light">. index.html applies
 * the saved theme before the first paint so there is no flash; this module
 * keeps it in sync afterwards.
 */

export type ThemePreference = "dark" | "light" | "system";
export type ResolvedTheme = "dark" | "light";

const STORAGE_KEY = "theme";
const media = typeof window !== "undefined" && window.matchMedia ? window.matchMedia("(prefers-color-scheme: light)") : null;

export function readPreference(): ThemePreference {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === "dark" || v === "light" || v === "system") return v;
  } catch {
    /* storage blocked: fall through to the default */
  }
  return "dark";
}

export function savePreference(pref: ThemePreference) {
  try {
    localStorage.setItem(STORAGE_KEY, pref);
  } catch {
    /* not persisted; the choice still applies to this page */
  }
}

export function resolveTheme(pref: ThemePreference): ResolvedTheme {
  if (pref === "system") return media?.matches ? "light" : "dark";
  return pref;
}

export function applyTheme(theme: ResolvedTheme) {
  const root = document.documentElement;
  root.setAttribute("data-theme", theme);
  root.style.colorScheme = theme;
}

/**
 * Calls onChange when the OS theme changes or another tab changes the
 * preference. Returns an unsubscribe function.
 */
export function watchTheme(onChange: (pref: ThemePreference) => void): () => void {
  const onMedia = () => onChange(readPreference());
  const onStorage = (e: StorageEvent) => {
    if (e.key === STORAGE_KEY) onChange(readPreference());
  };
  media?.addEventListener("change", onMedia);
  window.addEventListener("storage", onStorage);
  return () => {
    media?.removeEventListener("change", onMedia);
    window.removeEventListener("storage", onStorage);
  };
}
