/**
 * The colour scheme, chosen by the reader.
 *
 * System is the sensible default and the one the page starts on, but a route
 * inspector needs an explicit override for whenever the system's own setting
 * is not the preferred reading surface. See `lib/theme.ts` for where the
 * choice is kept and how it resolves against the system's own preference.
 *
 * A chip in the map's own control cluster, folded away until it is asked for,
 * the same as the basemap chooser beside it: the map is the page, and the
 * furniture on it earns its room.
 */

import type { ThemeChoice } from "../lib/theme";
import { THEME_CHOICES } from "../lib/theme";

/** What the button expands, named so the button can point at it. */
const THEME_LIST_ID = "map-theme-list";

/**
 * The radios' shared group, which is what makes them one control to a
 * keyboard. A constant is enough for the same reason it is in the basemap
 * picker: there is one of these on the page.
 */
const THEME_GROUP = "map-theme";

const THEME_LABELS: Record<ThemeChoice, string> = {
  system: "System",
  light: "Light",
  dark: "Dark",
};

export interface ThemePickerProps {
  choice: ThemeChoice;
  onChoose: (choice: ThemeChoice) => void;
  /** Whether the reader has the list open. Held by the caller — see below. */
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
}

export function ThemePicker({ choice, onChoose, expanded, onExpandedChange }: ThemePickerProps) {
  return (
    <div className="theme-picker">
      <button
        className="theme-picker__toggle"
        type="button"
        aria-expanded={expanded}
        // The mark says "the theme is a choice" to anyone who can see it; the
        // name says what, for anyone who cannot — the same split the basemap
        // picker's own toggle uses.
        aria-label={expanded ? "Hide the colour theme choices" : "Choose the colour theme"}
        // Only while there is a list to point at, because it is unmounted
        // rather than hidden when folded and a control naming an element
        // outside the document is a reference a screen reader cannot follow.
        {...(expanded ? { "aria-controls": THEME_LIST_ID } : {})}
        onClick={() => onExpandedChange(!expanded)}
      >
        <svg
          viewBox="0 0 12 12"
          width="12"
          height="12"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.2"
          strokeLinecap="round"
          aria-hidden="true"
          focusable="false"
        >
          <circle cx="6" cy="6" r="2.3" />
          <path d="M6 0.6v1.5M6 9.9v1.5M0.6 6h1.5M9.9 6h1.5M2.05 2.05l1.05 1.05M8.9 8.9l1.05 1.05M9.95 2.05 8.9 3.1M3.1 8.9l-1.05 1.05" />
        </svg>
      </button>
      {expanded ? (
        <div
          className="theme-picker__list"
          id={THEME_LIST_ID}
          role="radiogroup"
          aria-label="Colour theme"
        >
          {THEME_CHOICES.map((option) => (
            <label className="theme-picker__option" key={option}>
              {/*
               * Native radios rather than painted ones: arrow keys move within
               * the group, the group is one tab stop, and the checked one is
               * announced, all without this component reimplementing any of it.
               */}
              <input
                type="radio"
                name={THEME_GROUP}
                value={option}
                checked={option === choice}
                onChange={() => onChoose(option)}
              />
              <span>{THEME_LABELS[option]}</span>
            </label>
          ))}
        </div>
      ) : null}
    </div>
  );
}
