/**
 * The ground under the library, chosen by the reader.
 *
 * The operator names the basemaps a deployment offers; which of them is on
 * screen is a matter of what the reader is trying to see. Streets answer where
 * a road goes and imagery answers what the ground there actually is, and no
 * operator can pick between those two questions once, in a config file, on
 * somebody else's behalf.
 *
 * A chip in the map's own control cluster, folded away until it is asked for,
 * for the same reason the credit beside it folds: the map is the page, and the
 * furniture on it earns its room. Nothing is shown at all where the operator
 * configured one basemap — one basemap is not a choice, and a control that can
 * only confirm what is already true is noise in the corner.
 *
 * Which entry is checked is handed in rather than worked out here, because the
 * fallbacks in `basemap.ts` decide it: a reader who has picked nothing, or
 * picked an entry since removed from the config, is looking at a basemap they
 * did not name, and the mark must follow the ground actually loaded.
 */

import { IconStack2 } from "@tabler/icons-react";
import type { Basemap } from "../api/types";

/** What the button expands, named so the button can point at it. */
const BASEMAP_LIST_ID = "map-basemap-list";

/**
 * The radios' shared group, which is what makes them one control to a keyboard.
 *
 * A constant is enough: there is one library map on the page, and a second
 * would be a second map of the same library rather than a second choice.
 */
const BASEMAP_GROUP = "map-basemap";

export interface BasemapPickerProps {
  /** Every basemap the operator offers, in the order they configured them. */
  basemaps: Basemap[];
  /** The one on screen, by name. */
  selectedName: string;
  onSelect: (name: string) => void;
  /** Whether the reader has the list open. Held by the caller — see below. */
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
}

export function BasemapPicker({
  basemaps,
  selectedName,
  onSelect,
  expanded,
  onExpandedChange,
}: BasemapPickerProps) {
  if (basemaps.length < 2) {
    return null;
  }

  return (
    <div className="basemap-picker">
      <button
        className="basemap-picker__toggle"
        type="button"
        aria-expanded={expanded}
        // The mark says "the ground is a choice" to anyone who can see it; the
        // name says what, for anyone who cannot. `aria-expanded` carries the
        // state — the glyph does not change and must not be the only thing
        // saying which way the list is folded.
        aria-label={expanded ? "Hide the basemap choices" : "Choose the basemap"}
        // Only while there is a list to point at, because it is unmounted
        // rather than hidden when folded and a control naming an element
        // outside the document is a reference a screen reader cannot follow.
        {...(expanded ? { "aria-controls": BASEMAP_LIST_ID } : {})}
        onClick={() => onExpandedChange(!expanded)}
      >
        <IconStack2 size={12} stroke={1.2} aria-hidden="true" />
      </button>
      {expanded ? (
        <div
          className="basemap-picker__list"
          id={BASEMAP_LIST_ID}
          role="radiogroup"
          aria-label="Basemap"
        >
          {basemaps.map((basemap) => (
            <label className="basemap-picker__option" key={basemap.name}>
              {/*
               * Native radios rather than painted ones: arrow keys move within
               * the group, the group is one tab stop, and the checked one is
               * announced, all without this component reimplementing any of it.
               */}
              <input
                type="radio"
                name={BASEMAP_GROUP}
                value={basemap.name}
                checked={basemap.name === selectedName}
                onChange={() => onSelect(basemap.name)}
              />
              <span>{basemap.name}</span>
            </label>
          ))}
        </div>
      ) : null}
    </div>
  );
}
