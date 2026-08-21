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

import { useMemo, useState } from "react";
import { createPortal } from "react-dom";
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
 * And the ink the selected one is drawn in, per basemap.
 *
 * The same pair as `--accent` in index.css and `ROUTE_ACCENT` in RouteOverlay,
 * because a route picked out of the column and the same route opened must not
 * be two different colours.
 */
const SELECTION_ACCENT = { light: "#236fc7", dark: "#70adfb" } as const;

/** Where MapLibre keeps the controls this map asked for. */
const CLUSTER_SELECTOR = ".maplibregl-ctrl-bottom-left";

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
        // The identity travels with the line so the selection can be drawn out
        // of the same collection by filter, rather than by a second source that
        // would have to be rebuilt on every selection.
        properties: { key: line.key },
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
  const [cluster, setCluster] = useState<HTMLElement | null>(null);

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
   * The corner MapLibre keeps its controls in, once there is one.
   *
   * It is read off the loaded map rather than assumed, so a version of MapLibre
   * that arranges its corners differently costs the credit its place in the
   * cluster rather than its place on the page.
   */
  const findCluster = (event: { target?: { getContainer?: () => HTMLElement } }) => {
    setCluster(event.target?.getContainer?.()?.querySelector(CLUSTER_SELECTOR) ?? null);
  };

  /*
   * MapLibre's own attribution control is off. It renders the provider's own
   * markup into a corner of its own, and this map has one corner: the credit is
   * drawn by MapCredits, under the zoom pair and the scale bar.
   */
  return (
    <div className="route-map">
      <MapLibre
        mapStyle={styleUrl}
        onError={onError}
        onLoad={findCluster}
        style={{ width: "100%", height: "100%" }}
        aria-label="Map of the route library"
        attributionControl={false}
      >
        <MapViewport bounds={bounds} maxZoom={maxZoom} />
        {/*
         * One cluster, bottom-left: the zoom pair, the scale bar, the credit,
         * top to bottom. They are asked for in the other order because MapLibre
         * adds to a bottom corner by prepending, so that a control added later
         * stacks above the corner rather than under the ones already there.
         */}
        <ScaleControl position="bottom-left" unit="metric" />
        <NavigationControl position="bottom-left" showCompass={false} />
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
          {/*
           * The selection, in the accent.
           *
           * Picking a route out of the column drops the library to context, so
           * without this the one route being pointed at fades along with the
           * rest: the camera would fly to a line the reader can no longer see.
           * `RouteOverlay` paints the same geometry far wider once the route is
           * opened, so this is drawn only while there is no overlay covering it.
           */}
          {selectedKey !== null && !overlay ? (
            <Layer
              id="library-selected-line"
              type="line"
              filter={["==", ["get", "key"], selectedKey]}
              layout={{ "line-cap": "round", "line-join": "round" }}
              paint={{
                "line-color": SELECTION_ACCENT[theme],
                "line-width": 3,
                "line-opacity": 1,
              }}
            />
          ) : null}
        </Source>
        {/*
         * The selection, over the library. It is mounted and unmounted with the
         * selection rather than kept empty, because it is a stack of a dozen
         * layers built from one route's geometry — there is nothing to keep
         * mounted when there is no route.
         */}
        {overlay}
      </MapLibre>
      {/*
       * Into the cluster if the map has one, and into the corner itself if it
       * has not. Drawn into MapLibre's own container rather than beside it so
       * the three pieces of furniture are one column with one gap: a credit
       * positioned alongside the cluster would have to be cleared by it, and
       * whatever number did that clearing would be wrong for the next provider
       * whose attribution runs to a second line.
       */}
      {cluster === null ? (
        <MapCredits styleUrl={styleUrl} extra={extraCredit} />
      ) : (
        createPortal(<MapCredits styleUrl={styleUrl} extra={extraCredit} />, cluster)
      )}
    </div>
  );
}
