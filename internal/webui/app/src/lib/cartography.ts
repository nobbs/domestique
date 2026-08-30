/**
 * The colours drawn onto the cartography, and how close the camera may go.
 *
 * Every pair here is keyed by which basemap is loaded rather than by the
 * page's scheme, because these sit on the map: a deployment with no dark style
 * configured keeps the light basemap under a dark page, and a line has to
 * match the ground it is actually drawn on — see `LoadedBasemap.dark`.
 *
 * The same three pairs are `--accent`, `--panel` and `--ink` in index.css,
 * where they follow the page instead. Neither file can read the other's copy —
 * a CSS variable resolves against the page's scheme, which is exactly the key
 * these must not use — so `cartography.test.ts` holds the two files to the
 * same values instead.
 */

/** The accent the route itself is drawn in, per basemap. */
export const ROUTE_ACCENT = { light: "#236fc7", dark: "#70adfb" } as const;

/**
 * The panel colour, on the cartography.
 *
 * Under the route it is the casing that lifts the line off the ground rather
 * than merely recolouring it; on the tooltip it is the box the words sit in.
 */
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
