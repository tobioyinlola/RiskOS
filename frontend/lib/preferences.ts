"use client";

import { useSyncExternalStore } from "react";

/**
 * Per-browser console preferences (docs/design-system.md, Implementation in RiskOS): theme,
 * table density and whether the navigation is collapsed. Stored in localStorage; the theme is
 * also applied before first paint by the inline script in app/layout.tsx.
 */
export interface Preferences {
  theme: "dark" | "light";
  density: "default" | "compact";
  sidebarCollapsed: boolean;
}

export const PREFERENCES_KEY = "riskos.preferences";

const DEFAULTS: Preferences = {
  theme: "dark",
  density: "default",
  sidebarCollapsed: false,
};

let cache: Preferences = DEFAULTS;
let loaded = false;
const listeners = new Set<() => void>();

function read(): Preferences {
  if (loaded) return cache;
  loaded = true;
  try {
    const raw = window.localStorage.getItem(PREFERENCES_KEY);
    if (raw) {
      const p = JSON.parse(raw) as Partial<Preferences>;
      cache = {
        theme: p.theme === "light" ? "light" : "dark",
        density: p.density === "compact" ? "compact" : "default",
        sidebarCollapsed: p.sidebarCollapsed === true,
      };
    }
  } catch {
    cache = DEFAULTS;
  }
  return cache;
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** The current preferences, for code outside React (keyboard handlers). */
export function getPreferences(): Preferences {
  return read();
}

/** Updates preferences, persists them and applies the theme. */
export function setPreferences(patch: Partial<Preferences>) {
  cache = { ...read(), ...patch };
  try {
    window.localStorage.setItem(PREFERENCES_KEY, JSON.stringify(cache));
  } catch {
    // Storage can be unavailable (private mode); preferences then last for the page.
  }
  document.documentElement.dataset.theme = cache.theme;
  listeners.forEach((l) => l());
}

export function usePreferences(): Preferences {
  return useSyncExternalStore(subscribe, read, () => DEFAULTS);
}

/** Runs before first paint (inline, with the page's CSP nonce): applies the stored theme. */
export const THEME_SCRIPT = `try{var p=JSON.parse(localStorage.getItem(${JSON.stringify(
  PREFERENCES_KEY,
)})||"{}");document.documentElement.dataset.theme=p.theme==="light"?"light":"dark"}catch(e){}`;
