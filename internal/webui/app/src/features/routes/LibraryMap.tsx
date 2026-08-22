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
import type { MapLayerMouseEvent } from "react-map-gl/maplibre";
import {
  Layer,
  Map as MapLibre,
  NavigationControl,
  ScaleControl,
  Source,
} from "react-map-gl/maplibre";
import "maplibre-gl/dist/maplibre-gl.css";
import type { Basemap, BoundingBox, Position } from "../../api/types";
import { BasemapPicker } from "../../components/BasemapPicker";
import { MapCredits } from "../../components/MapCredits";
import { MapViewport } from "../../components/MapViewport";
import type { Insets } from "../../lib/overlayInsets";
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

/** The invisible band along each route that a pointer is actually asked about. */
const HIT_LAYER = "library-hit";

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

/**
 * How wide the line a pointer actually has to hit is.
 *
 * The library is drawn at two pixels, and two pixels is not a target: a route is
 * picked by pointing at where it goes, not by tracing it. The band is invisible
 * and carries the same identity as the line inside it, so what is clicked and
 * what lights up cannot disagree.
 */
const HIT_WIDTH = 18;

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
  /**
   * The basemaps on offer, and which of them `styleUrl` came from.
   *
   * The map does not choose: it is handed a style and told what the ground
   * looks like. These are here so the chooser can sit in the same cluster as
   * the credit, which is the only corner this map has.
   */
  basemaps?: Basemap[];
  selectedBasemap?: string;
  onBasemapChange?: (name: string) => void;
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
  /** How much of the map the panels over it are covering, if anything is. */
  insets?: Insets;
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
  /**
   * Picks the route the pointer landed on, by `routeKey`.
   *
   * The map is the library, so a line on it is the route itself rather than a
   * picture of one: pointing at where a ride goes is the most direct way of
   * asking about it there is, and it needs no column at all. What picking means
   * — expanding its card, or opening it — is the page's to decide; this map
   * knows only which line was under the pointer.
   *
   * Left out where nothing is listening, which also leaves the map inert: no
   * pointer cursor and no hover paint promising a click that does nothing.
   */
  onPick?: (key: string) => void;
  /**
   * The one line a pick would do nothing to: the route already opened.
   *
   * It gets no pointer cursor, because the cursor is a promise. Which route
   * that is cannot be read off `selectedKey`, which is the route standing out
   * whether it is merely picked — where a second click opens it — or open
   * already, where there is nothing left for a click to do.
   */
  inertKey?: string | null;
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
  basemaps = [],
  selectedBasemap = "",
  onBasemapChange,
  lines,
  selectedKey,
  bounds,
  insets,
  maxZoom = DEFAULT_MAX_ZOOM,
  overlay,
  extraCredit,
  onPick,
  inertKey = null,
}: LibraryMapProps) {
  const theme = darkBasemap ? "dark" : "light";
  const [cluster, setCluster] = useState<HTMLElement | null>(null);
  const [hoveredKey, setHoveredKey] = useState<string | null>(null);
  /*
   * Whether the reader folded the credit away, held here rather than inside it.
   * Finding the cluster moves the credit into it through a portal, which
   * remounts the credit — so a choice it held itself would be undone by the map
   * finishing loading. `null` until the reader says, so the viewport decides.
   */
  const [creditChoice, setCreditChoice] = useState<boolean | null>(null);
  /*
   * And whether the basemap list is unfolded, held here for the same reason:
   * the portal that moves the cluster's contents remounts them, so a list the
   * picker opened for itself would fold shut when the map finished loading.
   * Closed until asked for — the ground on screen is the answer most of the
   * time, and the names are only worth room while somebody is changing it.
   */
  const [pickerOpen, setPickerOpen] = useState(false);

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

  /** Which route the pointer is over, out of the band it actually hit. */
  const keyAt = (event: MapLayerMouseEvent): string | null => {
    const key = event.features?.[0]?.properties?.key;

    return typeof key === "string" ? key : null;
  };

  /*
   * One fragment, rendered in one of two places below, so that moving it does
   * not give either piece a different set of props — and so the picker sits
   * above the credit either way, rather than in whichever order the two places
   * happen to produce.
   */
  const furniture = (
    <>
      {onBasemapChange ? (
        <BasemapPicker
          basemaps={basemaps}
          selectedName={selectedBasemap}
          onSelect={onBasemapChange}
          expanded={pickerOpen}
          onExpandedChange={setPickerOpen}
        />
      ) : null}
      <MapCredits
        styleUrl={styleUrl}
        extra={extraCredit}
        choice={creditChoice}
        onChoiceChange={setCreditChoice}
      />
    </>
  );

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
        /*
         * Only the invisible band is asked about. Querying the drawn line would
         * make the answer depend on how wide the ink happens to be, and the
         * overlay's own layers answer for themselves.
         */
        interactiveLayerIds={onPick ? [HIT_LAYER] : []}
        cursor={hoveredKey !== null && hoveredKey !== inertKey ? "pointer" : ""}
        onMouseMove={(event: MapLayerMouseEvent) => setHoveredKey(keyAt(event))}
        onMouseOut={() => setHoveredKey(null)}
        onClick={(event: MapLayerMouseEvent) => {
          const key = keyAt(event);
          if (key !== null) {
            onPick?.(key);
          }
        }}
      >
        <MapViewport bounds={bounds} maxZoom={maxZoom} {...(insets ? { insets } : {})} />
        {/*
         * One cluster, bottom-left: the zoom pair, the scale bar, then the
         * basemap chip and the credit, top to bottom. The two MapLibre owns are
         * asked for in the other order because it adds to a bottom corner by
         * prepending, so that a control added later stacks above the corner
         * rather than under the ones already there.
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
          {/*
           * What a click would pick, while the pointer is over it.
           *
           * The line under the pointer is lit in the same accent the selection
           * is drawn in, so the map answers "this one?" before it is asked to
           * commit — and the route the reader is already looking at is left
           * alone, because it is painted in that accent already.
           */}
          {hoveredKey !== null && hoveredKey !== selectedKey ? (
            <Layer
              id="library-hover-line"
              type="line"
              filter={["==", ["get", "key"], hoveredKey]}
              layout={{ "line-cap": "round", "line-join": "round" }}
              paint={{
                "line-color": SELECTION_ACCENT[theme],
                "line-width": 3,
                "line-opacity": 0.75,
              }}
            />
          ) : null}
          {/*
           * The target, over everything the source draws: a transparent band
           * along each route, wide enough to point at. It is drawn rather than
           * merely queried because MapLibre answers about what it rendered, and
           * it is only mounted where a picked route has somewhere to go.
           */}
          {onPick ? (
            <Layer
              id={HIT_LAYER}
              type="line"
              layout={{ "line-cap": "round", "line-join": "round" }}
              paint={{
                "line-color": LIBRARY_INK[theme],
                "line-width": HIT_WIDTH,
                "line-opacity": 0,
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
       * every piece of furniture is one column with one gap: a credit
       * positioned alongside the cluster would have to be cleared by it, and
       * whatever number did that clearing would be wrong for the next provider
       * whose attribution runs to a second line.
       */}
      {cluster === null ? furniture : createPortal(furniture, cluster)}
    </div>
  );
}
