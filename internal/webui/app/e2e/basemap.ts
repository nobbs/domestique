/**
 * The basemap the browser suite loads instead of a provider's.
 *
 * A style document with no sprite, no glyphs and one empty source: MapLibre
 * paints the background colour and then the application's own layers over it,
 * and issues no further request of its own. That is what keeps a browser test
 * off the network — a real style names tile, sprite and font URLs, and every one
 * of them would be a request to somebody else's server from a suite that is
 * supposed to be hermetic — and it is also what makes the map's own paint
 * reproducible: a flat background renders identically on every run, so a
 * difference between two screenshots of it is a difference the application drew.
 *
 * The two documents differ only in that colour, as a provider's light and dark
 * styles do, so a test can tell which one the page chose by looking at it.
 *
 * The one source exists to declare an attribution, because a real style is where
 * the credit comes from and a suite whose style has none could not see the
 * credit at all. It holds no features, so it paints nothing and fetches nothing,
 * and its attribution is wrapped in a link exactly as a provider's is — which is
 * how the suite can tell that the words arrive and the markup does not.
 */

/** What the fixture style declares, as a provider would: markup and all. */
export const BASEMAP_ATTRIBUTION_HTML =
  '<a href="https://example.test/">&copy; Demo Cartography</a>';

/** The same credit as the page is obliged to show it: words, no markup. */
export const BASEMAP_ATTRIBUTION_TEXT = "© Demo Cartography";

/** A MapLibre style document, as far as this suite needs to describe one. */
export interface BasemapStyle {
  version: 8;
  name: string;
  sources: {
    credit: {
      type: "geojson";
      attribution: string;
      data: { type: "FeatureCollection"; features: [] };
    };
  };
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
    sources: {
      credit: {
        type: "geojson",
        attribution: BASEMAP_ATTRIBUTION_HTML,
        data: { type: "FeatureCollection", features: [] },
      },
    },
    layers: [{ id: "background", type: "background", paint: { "background-color": colour } }],
  };
}

/** Stands in for the operator's light style. */
export const lightBasemapStyle = styleWithBackground("demo-light", "#f2efe9");

/** Stands in for the operator's dark style. */
export const darkBasemapStyle = styleWithBackground("demo-dark", "#12161c");
