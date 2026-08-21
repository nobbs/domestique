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
const ROUTE_ACCENT = { light: "#236fc7", dark: "#70adfb" } as const;
/** The casing under the selected route: the panel colour, so it reads as lifted. */
const SELECTED_CASING = { light: "#fcfdff", dark: "#24282c" } as const;

/**
 * How strongly the library is drawn.
 *
 * Below full so the basemap's own roads and rivers stay legible underneath —
 * the library is a layer over the ground, not a replacement for it — and far
 * enough above the ground that a route nowhere near the selection is still
 * plainly a route.
 */
const LIBRARY_OPACITY = 0.68;

/** How close the camera will go when it has only one route to frame. */
const ROUTE_MAX_ZOOM = 14;

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
}: LibraryMapProps) {
  const theme = darkBasemap ? "dark" : "light";

  // The selected route is drawn twice — once with the library, once on top —
  // rather than cut out of it. Removing it would rebuild the whole collection
  // on every selection, and the line underneath is exactly covered anyway.
  const library = useMemo(() => collectionOf(lines), [lines]);
  const selected = useMemo(
    () => collectionOf(selectedKey ? lines.filter((line) => line.key === selectedKey) : []),
    [lines, selectedKey],
  );

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
        <MapViewport bounds={bounds} maxZoom={ROUTE_MAX_ZOOM} />
        {/* One cluster, bottom-right: the zoom pair, the scale bar, the credit. */}
        <NavigationControl position="bottom-right" showCompass={false} />
        <ScaleControl position="bottom-right" unit="metric" />
        <Source id="library-lines" type="geojson" data={library}>
          <Layer
            id="library-line"
            type="line"
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{
              "line-color": LIBRARY_INK[theme],
              "line-width": 2,
              "line-opacity": LIBRARY_OPACITY,
            }}
          />
        </Source>
        {/*
         * The selection, over the library: a casing in the panel colour so the
         * line reads as lifted off the ground rather than merely recoloured,
         * and the accent over it. Both layers stay mounted with nothing in them
         * when there is no selection, so picking a route repaints rather than
         * rebuilding the style.
         */}
        <Source id="library-selected" type="geojson" data={selected}>
          <Layer
            id="library-selected-casing"
            type="line"
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{ "line-color": SELECTED_CASING[theme], "line-width": 7 }}
          />
          <Layer
            id="library-selected-line"
            type="line"
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{ "line-color": ROUTE_ACCENT[theme], "line-width": 4 }}
          />
        </Source>
      </MapLibre>
      <MapCredits styleUrl={styleUrl} />
    </div>
  );
}
