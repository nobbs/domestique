/**
 * The whole library, drawn once.
 *
 * The entry page is this map: every stored route on real cartography, in one
 * ink, at one weight. It is not a chooser dressed as a map — nothing is
 * classified, banded or labelled here, because forty-seven routes in five
 * colours is a pattern rather than an answer. Selection is the only thing that
 * stands out, and it stands out because it is the only thing painted in the
 * accent.
 *
 * There is one instance of it for the life of the page. The selected route is
 * drawn over the library by `RouteOverlay`, handed in as `overlay` and rendered
 * inside this map rather than beside it: a second map for the route would
 * download the style again and throw away the ground the reader was looking at.
 *
 * Every line comes from geometry the service already holds; only the basemap is
 * fetched from outside, and the tile provider is never told which routes are on
 * the screen.
 */

import { useMemo } from "react";
import {
  Layer,
  Map as MapLibre,
  NavigationControl,
  ScaleControl,
  Source,
} from "react-map-gl/maplibre";
import "maplibre-gl/dist/maplibre-gl.css";
import type { BoundingBox, Position } from "../../api/types";
import { MapCredits } from "../../components/MapCredits";
import { MapViewport } from "../../components/MapViewport";
// Configures the shared worker pool; without it this map renders no tiles.
import "../../lib/maplibre";

/**
 * The ink every unselected route is drawn in, per basemap.
 *
 * Keyed on which basemap is loaded rather than on the system scheme, because it
 * sits on the cartography rather than on the page. The same pair is `--ink` in
 * index.css, and the accent below is `--accent`; all of them must stay in step.
 */
const LIBRARY_INK = { light: "#1c2126", dark: "#eef0f3" } as const;

/**
 * How strongly the library is drawn.
 *
 * Below full so the basemap's own roads and rivers stay legible underneath —
 * the library is a layer over the ground, not a replacement for it — and far
 * enough above the ground that a route nowhere near the selection is still
 * plainly a route.
 */
const LIBRARY_OPACITY = 0.68;

/**
 * And how faintly, once one of them is the answer.
 *
 * Far enough down that the selected route is plainly the only route being read,
 * and far enough up that the others are still there: the reader asked where one
 * route goes, and the rest of the library is the answer to *where* — so it stays
 * on the ground as context rather than being switched off.
 */
const CONTEXT_OPACITY = 0.14;

/** How close the camera will go by default: enough for one route, not a street. */
const DEFAULT_MAX_ZOOM = 14;

/** One drawable route: its identity, and the line itself. */
export interface LibraryLine {
  key: string;
  coordinates: Position[];
}

export interface LibraryMapProps {
  styleUrl: string;
  /** Whether `styleUrl` is the dark cartography, which picks the ink. */
  darkBasemap?: boolean;
  lines: LibraryLine[];
  /** The route standing out, by `routeKey`. Null draws the library flat. */
  selectedKey: string | null;
  /**
   * What the camera frames: the library, or the selected route.
   *
   * Passed in rather than derived here, because the page already unions the
   * bounding boxes to know whether it has any geometry at all, and two answers
   * to that question would be one too many.
   */
  bounds: BoundingBox | null;
  /**
   * How close the camera may come to `bounds`.
   *
   * The library and one route are both framed at the default; a stretch of one
   * route was asked for, so it is allowed closer — see the callers.
   */
  maxZoom?: number;
  /**
   * The stack drawn over the library: the selected route, in full.
   *
   * A child rather than a prop this map draws itself, because everything in it
   * is about one route — the steepness, the surface, the position shared with
   * the chart — and none of that is this map's business.
   */
  overlay?: React.ReactNode;
  /** A credit the style cannot know about, joined to the ones it declares. */
  extraCredit?: string | undefined;
}

/** The lines as one collection, which is all a single line layer can be given. */
function collectionOf(lines: LibraryLine[]) {
  return {
    type: "FeatureCollection" as const,
    features: lines
      .filter((line) => line.coordinates.length > 1)
      .map((line) => ({
        type: "Feature" as const,
        geometry: { type: "LineString" as const, coordinates: line.coordinates },
        properties: {},
      })),
  };
}

export function LibraryMap({
  styleUrl,
  darkBasemap = false,
  lines,
  selectedKey,
  bounds,
  maxZoom = DEFAULT_MAX_ZOOM,
  overlay,
  extraCredit,
}: LibraryMapProps) {
  const theme = darkBasemap ? "dark" : "light";

  // The selected route stays in the collection rather than being cut out of it:
  // removing it would rebuild every line on every selection, and the overlay
  // covers the one underneath exactly.
  const library = useMemo(() => collectionOf(lines), [lines]);

  // A basemap that fails to load must not fail silently: the library is still
  // drawn, but the operator should be able to see why the ground is empty.
  const onError = (event: { error?: Error }) => {
    console.error("map error:", event.error?.message ?? event);
  };

  /*
   * MapLibre's own attribution control is off. It renders the provider's own
   * markup into a corner of its own, and this map has one corner: the credit is
   * drawn by MapCredits below, under the zoom pair and the scale bar.
   */
  return (
    <div className="route-map">
      <MapLibre
        mapStyle={styleUrl}
        onError={onError}
        style={{ width: "100%", height: "100%" }}
        aria-label="Map of the route library"
        attributionControl={false}
      >
        <MapViewport bounds={bounds} maxZoom={maxZoom} />
        {/* One cluster, top-right: the credit, the zoom pair, the scale bar. */}
        <NavigationControl position="top-right" showCompass={false} />
        <ScaleControl position="top-right" unit="metric" />
        <Source id="library-lines" type="geojson" data={library}>
          <Layer
            id="library-line"
            type="line"
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{
              "line-color": LIBRARY_INK[theme],
              "line-width": 2,
              "line-opacity": selectedKey === null ? LIBRARY_OPACITY : CONTEXT_OPACITY,
            }}
          />
        </Source>
        {/*
         * The selection, over the library. It is mounted and unmounted with the
         * selection rather than kept empty, because it is a stack of a dozen
         * layers built from one route's geometry — there is nothing to keep
         * mounted when there is no route.
         */}
        {overlay}
      </MapLibre>
      <MapCredits styleUrl={styleUrl} extra={extraCredit} />
    </div>
  );
}
