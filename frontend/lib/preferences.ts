"use client";

import { useSyncExternalStore } from "react";

/**
 * Per-browser console preferences (docs/design-system.md, Implementation in RiskOS): table
 * density and whether the navigation is collapsed. Stored in localStorage. The product is light
 * only, so there is no theme preference; a stored theme from earlier versions is ignored.
 */
export interface Preferences {
  density: "default" | "compact";
  sidebarCollapsed: boolean;
}

export const PREFERENCES_KEY = "riskos.preferences";

const DEFAULTS: Preferences = {
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

/** Updates and persists preferences. */
export function setPreferences(patch: Partial<Preferences>) {
  cache = { ...read(), ...patch };
  try {
    window.localStorage.setItem(PREFERENCES_KEY, JSON.stringify(cache));
  } catch {
    // Storage can be unavailable (private mode); preferences then last for the page.
  }
  listeners.forEach((l) => l());
}

export function usePreferences(): Preferences {
  return useSyncExternalStore(subscribe, read, () => DEFAULTS);
}
