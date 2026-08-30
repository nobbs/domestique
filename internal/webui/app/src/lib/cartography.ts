/**
 * The colours drawn onto the cartography, and how close the camera may go.
 *
 * Each pair is keyed by which basemap is loaded, never by the page's scheme —
 * a deployment with no dark style keeps the light basemap under a dark page
 * (see `LoadedBasemap.dark`). The same pairs are `--accent`, `--panel` and
 * `--ink` in index.css, where they follow the page; a CSS variable would
 * resolve against that wrong key, so `cartography.test.ts` holds the two
 * files' copies equal instead.
 */

/** The accent the route itself is drawn in, per basemap. */
export const ROUTE_ACCENT = { light: "#236fc7", dark: "#70adfb" } as const;

/** The panel colour on the cartography: the route's casing, the tooltip's box. */
export const PANEL = { light: "#fcfdff", dark: "#24282c" } as const;

/** The ink the library's lines are drawn in, per basemap. */
export const INK = { light: "#1c2126", dark: "#eef0f3" } as const;

/**
 * How close the camera will go to the library, or to one whole route.
 *
 * A short route would otherwise open at street level, which says nothing about
 * where the ride goes.
 */
export const ROUTE_MAX_ZOOM = 14;

/**
 * And how close it may come to the stretch the chart is showing.
 *
 * Higher, because that framing was asked for: the shortest window the chart
 * allows is 200 m, and holding it to the whole-route cap would answer a
 * request to look closer by barely moving.
 */
export const WINDOW_MAX_ZOOM = 17;
