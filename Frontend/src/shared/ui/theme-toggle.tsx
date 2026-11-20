"use client";

import { useEffect, useState } from "react";

/**
 * Light/dark switch (S2).
 *
 * The choice is stored explicitly, so picking light on a machine set to dark stays light.
 * The first visit stores nothing and follows the system, which is why the root layout runs a
 * small inline script before paint: without it, the page renders light and then flips, and a
 * flash of the wrong theme is worse than no dark mode.
 *
 * The attribute is `data-theme` on `<html>`, which is what globals.css overrides.
 */

type Theme = "light" | "dark";

export const THEME_STORAGE_KEY = "aveniq-theme";

function currentTheme(): Theme {
  return document.documentElement.dataset.theme === "dark" ? "dark" : "light";
}

export function ThemeToggle() {
  // Null until the effect reads the real value: rendering an assumed theme would make the
  // button lie for one frame.
  const [theme, setTheme] = useState<Theme | null>(null);

  useEffect(() => {
    setTheme(currentTheme());
  }, []);

  const dark = theme === "dark";
  const label = dark ? "Usar tema claro" : "Usar tema oscuro";

  return (
    <button
      type="button"
      aria-pressed={theme === null ? undefined : dark}
      aria-label={label}
      title={label}
      onClick={() => {
        const next: Theme = dark ? "light" : "dark";
        document.documentElement.dataset.theme = next;
        try {
          localStorage.setItem(THEME_STORAGE_KEY, next);
        } catch {
          // A blocked storage must not break the switch: the theme still applies for the visit.
        }
        setTheme(next);
      }}
      className="grid h-8 w-8 place-items-center rounded-control border border-border bg-surface text-ink-muted hover:bg-surface-muted hover:text-ink"
    >
      {dark ? <SunIcon /> : <MoonIcon />}
    </button>
  );
}

function MoonIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
      <path d="M13 9.6A5.6 5.6 0 0 1 6.4 3 5.6 5.6 0 1 0 13 9.6Z" />
    </svg>
  );
}

function SunIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
      <circle cx="8" cy="8" r="3" />
      <path d="M8 1.5v1.6M8 12.9v1.6M1.5 8h1.6M12.9 8h1.6M3.4 3.4l1.1 1.1M11.5 11.5l1.1 1.1M12.6 3.4l-1.1 1.1M4.5 11.5l-1.1 1.1" />
    </svg>
  );
}
