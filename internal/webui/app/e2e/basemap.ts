/**
 * The basemap the browser suite loads instead of a provider's.
 *
 * A style document with no sources, no sprite and no glyphs: MapLibre paints the
 * background colour and then the application's own layers over it, and issues no
 * further request of its own. That is what keeps a browser test off the network —
 * a real style names tile, sprite and font URLs, and every one of them would be a
 * request to somebody else's server from a suite that is supposed to be
 * hermetic — and it is also what makes the map's own paint reproducible: a flat
 * background renders identically on every run, so a difference between two
 * screenshots of it is a difference the application drew.
 *
 * The two documents differ only in that colour, as a provider's light and dark
 * styles do, so a test can tell which one the page chose by looking at it.
 */

/** A MapLibre style document, as far as this suite needs to describe one. */
export interface BasemapStyle {
  version: 8;
  name: string;
  sources: Record<string, never>;
  layers: Array<{
    id: string;
    type: "background";
    paint: { "background-color": string };
  }>;
}

function styleWithBackground(name: string, colour: string): BasemapStyle {
  return {
    version: 8,
    name,
    sources: {},
    layers: [{ id: "background", type: "background", paint: { "background-color": colour } }],
  };
}

/** Stands in for the operator's light style. */
export const lightBasemapStyle = styleWithBackground("demo-light", "#f2efe9");

/** Stands in for the operator's dark style. */
export const darkBasemapStyle = styleWithBackground("demo-dark", "#12161c");
