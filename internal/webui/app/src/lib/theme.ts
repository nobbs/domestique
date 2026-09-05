/**
 * The reader's explicit colour-scheme choice, over the system's own.
 *
 * System is a sensible default, but a route inspector needs an explicit local
 * override for whenever it is not the preferred reading surface — daylight on
 * a phone screen, say. The choice lives only in this browser: see
 * `useThemeChoice` for where and how.
 *
 * What actually paints the override is `[data-theme]` on the root element,
 * set from this choice — see `index.css` for the two palette blocks that
 * attribute switches between, and `App.tsx` for where it is applied.
 */

import { useCallback, useSyncExternalStore } from "react";

export const THEME_CHOICES = ["system", "light", "dark"] as const;

export type ThemeChoice = (typeof THEME_CHOICES)[number];

/**
 * Where the reader's pick is remembered. Namespaced the same way the basemap
 * choice is — see `basemap.ts` — and holds nothing but this one word.
 */
const CHOICE_STORAGE_KEY = "domestique.theme";

// Two consumers share one screen — the bar's toggle and the `data-theme`
// attribute `App.tsx` sets from the same choice — so a pick must reach both at
// once rather than wait for a remount. The same store `identity.ts` keeps, for
// the same reason.
const listeners = new Set<() => void>();

function subscribe(listener: () => void): () => void {
  listeners.add(listener);

  return () => listeners.delete(listener);
}

/**
 * The reader's colour-scheme choice, remembered across visits and shared live
 * across every consumer.
 *
 * Guarded the same way `useBasemapChoice` guards storage: a browser may
 * refuse it outright, in a private window or with third-party storage
 * blocked, and a refusal costs the choice its memory rather than the page its
 * theme.
 */
export function useThemeChoice(): [ThemeChoice, (choice: ThemeChoice) => void] {
  const choice = useSyncExternalStore(subscribe, readChoice);

  const setChoice = useCallback((next: ThemeChoice) => {
    writeChoice(next);
    for (const listener of listeners) {
      listener();
    }
  }, []);

  return [choice, setChoice];
}

/** The next choice round the bar toggle's cycle, wrapping at the end. */
export function nextThemeChoice(choice: ThemeChoice): ThemeChoice {
  const at = THEME_CHOICES.indexOf(choice);

  return THEME_CHOICES[(at + 1) % THEME_CHOICES.length] ?? "system";
}

// A pick made on this page, which outranks whatever storage holds: a browser
// may refuse the write, and a refusal must cost the choice its memory rather
// than undo it on the very next read. Until one is made there is nothing to
// outrank, so storage is read as it stands.
let picked: ThemeChoice | null = null;

function isThemeChoice(value: string | null): value is ThemeChoice {
  return (THEME_CHOICES as readonly string[]).includes(value ?? "");
}

function readChoice(): ThemeChoice {
  if (picked !== null) {
    return picked;
  }
  try {
    const stored = window.localStorage.getItem(CHOICE_STORAGE_KEY);
    if (isThemeChoice(stored)) {
      return stored;
    }
  } catch {}

  return "system";
}

function writeChoice(choice: ThemeChoice): void {
  picked = choice;
  try {
    window.localStorage.setItem(CHOICE_STORAGE_KEY, choice);
  } catch {
    // Remembering is the whole of what is lost, and the pick still stands for
    // as long as the page is open.
  }
}

/**
 * Whether the scheme in force is dark, given the reader's choice and what the
 * system itself is currently asking for.
 *
 * "system" defers to the system query; "light" and "dark" hold regardless of
 * it, which is the entire point of an explicit override.
 */
export function resolvesDark(choice: ThemeChoice, systemPrefersDark: boolean): boolean {
  if (choice === "system") {
    return systemPrefersDark;
  }

  return choice === "dark";
}
