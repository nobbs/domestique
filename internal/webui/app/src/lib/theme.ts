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

import { useCallback, useState } from "react";

export const THEME_CHOICES = ["system", "light", "dark"] as const;

export type ThemeChoice = (typeof THEME_CHOICES)[number];

/**
 * Where the reader's pick is remembered. Namespaced the same way the basemap
 * choice is — see `basemap.ts` — and holds nothing but this one word.
 */
const CHOICE_STORAGE_KEY = "domestique.theme";

function isThemeChoice(value: string | null): value is ThemeChoice {
  return (THEME_CHOICES as readonly string[]).includes(value ?? "");
}

function readChoice(): ThemeChoice {
  try {
    const stored = window.localStorage.getItem(CHOICE_STORAGE_KEY);

    return isThemeChoice(stored) ? stored : "system";
  } catch {
    return "system";
  }
}

function writeChoice(choice: ThemeChoice): void {
  try {
    window.localStorage.setItem(CHOICE_STORAGE_KEY, choice);
  } catch {
    // Remembering is the whole of what is lost, and the pick still stands for
    // as long as the page is open.
  }
}

/**
 * The reader's colour-scheme choice, remembered across visits.
 *
 * Guarded the same way `useBasemapChoice` guards storage: a browser may
 * refuse it outright, in a private window or with third-party storage
 * blocked, and a refusal costs the choice its memory rather than the page its
 * theme.
 */
export function useThemeChoice(): [ThemeChoice, (choice: ThemeChoice) => void] {
  const [choice, setChoiceState] = useState<ThemeChoice>(readChoice);

  const setChoice = useCallback((next: ThemeChoice) => {
    setChoiceState(next);
    writeChoice(next);
  }, []);

  return [choice, setChoice];
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
