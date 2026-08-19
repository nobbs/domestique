/**
 * The route map.
 *
 * The basemap style is loaded from the operator-configured tile origin — the
 * one thing this otherwise Tailnet-private service fetches from outside. The
 * route itself is drawn from geometry the service already holds locally and is
 * never sent anywhere.
 *
 * It reads as part of the page until it is asked to be a map: the wheel scrolls
 * past it, a finger scrolls past it, and only a drag along the painted route is
 * taken as a question about the ride. The control in its corner is what hands it
 * the gestures, and Escape is what gives them back — see `mapExploration`.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Layer,
  Map as MapLibre,
  NavigationControl,
  ScaleControl,
  Source,
  useMap,
} from "react-map-gl/maplibre";
import "maplibre-gl/dist/maplibre-gl.css";
import type { DataDrivenPropertyValueSpecification } from "maplibre-gl";
import type { BoundingBox, Position, SurfaceRange } from "../api/types";
import type { Highlight } from "../lib/highlight";
import { highlightRanges, litRanges } from "../lib/highlight";
import { mapExploration } from "../lib/mapExploration";
import { routeMetresAt, routeSelection } from "../lib/mapSelection";
import type { DistanceWindow, Profile } from "../lib/profile";
import { coordinateRange, nearestSample, rangeBounds, sampleAt } from "../lib/profile";
import { cuesDescription, directionChevrons, metresPerPixel, routeCues } from "../lib/routeCues";
import { gradientSlices, routeLinesWithin } from "../lib/routeLines";
import { NEAR_ROUTE_PIXELS, NEAR_ROUTE_TOUCH_PIXELS } from "../lib/selection";
import { SURFACE_LINE_WIDTH, SURFACE_STYLES, surfaceLinesWithin } from "../lib/surface";
import { useEscapeKey } from "../lib/useEscapeKey";
import { ExploreToggle } from "./ExploreToggle";
// Configures the shared worker pool; without it this map renders no tiles.
import "../lib/maplibre";

/** The brand accent, chosen to stay legible over both light and dark basemaps. */
const ROUTE_ACCENT = "#C8502E";
const SOURCE_ID = "stage-geometry";

/**
 * Credit for the surface classification, which is derived from OpenStreetMap.
 *
 * The basemap's own credit arrives from the style document and covers the tiles
 * only. The classification is a separate derived database under the ODbL, whose
 * share-alike terms oblige this attribution wherever it is shown. It is stated
 * unconditionally: MapLibre fixes a control's options when the map is built, so
 * a credit made conditional on the open stage having been classified would be
 * the stale one by the time it mattered.
 */
const SURFACE_ATTRIBUTION = "Surface data © OpenStreetMap contributors (ODbL)";

/** The casing's normal opacity, kept in one place so the dimmed one follows it. */
const CASING_OPACITY = 0.85;

/**
 * How far the two terminal markers are nudged apart, in screen pixels.
 *
 * A loop and an out-and-back both finish where they started, so both markers
 * would be drawn on the same coordinate and one would simply be missing. The
 * nudge is applied only when the two ends are that close: elsewhere each marker
 * sits exactly on its own point, because a marker is where a ride begins, not a
 * decoration near it. Even nudged, it stays a label for the terminal rather than
 * a survey mark, which is why the words beside the map say plainly that the two
 * ends are the same place.
 */
const TERMINAL_NUDGE_PIXELS = 9;

/** How wide a terminal marker is drawn, and how heavy its ring. */
const TERMINAL_RADIUS = 7;
const TERMINAL_RING_WIDTH = 3;

/**
 * The steepness ramp, band by band, mirroring `--band-*` in `index.css`.
 *
 * Repeated here rather than read from the stylesheet because MapLibre paints
 * from a style document and knows nothing of CSS custom properties. Both ramps
 * are carried for the same reason the stylesheet carries both: the elevation
 * chart and the map band the same ground, and a chart that lifted its colours
 * for a dark page while the map kept the light ones would be two legends for one
 * encoding.
 *
 * Keyed on which basemap is loaded rather than on the system scheme, because
 * these sit on the cartography rather than on the page — see `Basemap.dark`.
 *
 * Only the steeper four are ever drawn — see `GRADIENT_BANDS_DRAWN`.
 */
const BAND_COLOURS = {
  light: ["#e0ac2c", "#c87f41", "#b15635", "#952e2c", "#63202b"],
  dark: ["#f3cb60", "#ef9a55", "#e07550", "#cd554d", "#b8354a"],
} as const;

/**
 * The bands worth ink on a map.
 *
 * Gentle ground is most of most routes, and painting it would edge the whole
 * line in gold to say the unremarkable thing. Left unpainted, the white casing
 * stands for it, and colour appears only where the road starts to bite. The
 * chart still shows every band, which is where a reader goes to compare one
 * stretch against another; the map is answering "where does it get steep?".
 */
const GRADIENT_BANDS_DRAWN = [1, 2, 3, 4] as const;

/** The band whose layer sits lowest, and so the one the halo goes under. */
const LOWEST_BAND_DRAWN = GRADIENT_BANDS_DRAWN[0];

/**
 * How wide the steepness line runs beneath the casing.
 *
 * Wider than the casing, so what shows is an edging either side of it rather
 * than a colour behind it. The casing keeps the two encodings apart: steepness
 * never touches the class colour it would otherwise be read as part of.
 */
const BAND_EDGE_WIDTH = 11;

/**
 * How much of the route survives outside the ground on show.
 *
 * Dimmed rather than hidden: the ride does not stop at the edges of a window,
 * and a route drawn only in the middle would read as a shorter route rather than
 * as a closer look at a longer one. The same holds for a class picked out of the
 * key — the gravel is somewhere on this ride, and hiding the tarmac either side
 * of it would lose where. A quarter is faint enough that the eye lands on the
 * lit ground first and still dark enough to follow the road between.
 */
const OUTSIDE_OPACITY = 0.25;

/**
 * An opacity that drops away outside the ground on show.
 *
 * One expression over one tagged source rather than a second stack of layers:
 * `line-opacity` is a layer property, and one layer per class is exactly what
 * keeps a class's dash pattern identical on both sides of a lit stretch's edge.
 * Written as a function because a bare array in a `const` widens to `unknown[]`
 * and stops matching the tuple union the paint property is typed as.
 */
function dimmedOutside(
  full: number,
  dimmed: boolean,
): DataDrivenPropertyValueSpecification<number> {
  return dimmed ? ["case", ["get", "shown"], full, full * OUTSIDE_OPACITY] : full;
}

/**
 * The route's geometry as at most two features, tagged by whether the chart is
 * showing them, so one paint expression can tell the two apart.
 */
function taggedCollection(slices: { inside: Position[][]; outside: Position[][] }) {
  const features = ([lines, shown]: [Position[][], boolean]) =>
    lines.length === 0
      ? []
      : [
          {
            type: "Feature" as const,
            geometry: { type: "MultiLineString" as const, coordinates: lines },
            properties: { shown },
          },
        ];

  return {
    type: "FeatureCollection" as const,
    features: [...features([slices.inside, true]), ...features([slices.outside, false])],
  };
}

/**
 * How close the camera may come when it frames the whole stage.
 *
 * A short stage would otherwise open at street level, which says nothing about
 * where the ride goes.
 */
const ROUTE_MAX_ZOOM = 15;

/**
 * How close it may come when it frames the stretch the chart is showing.
 *
 * Higher, because that framing was asked for: the shortest window the chart
 * allows is 200 m, and holding it to the whole-stage cap would answer a request
 * to look closer by barely moving. The cap still only bites on the very
 * shortest selections, and it stops the map from diving past the point where
 * the basemap has anything left to add.
 */
const WINDOW_MAX_ZOOM = 17;

/**
 * Keeps the camera framed on the ground on show and the canvas sized to its
 * pane.
 *
 * This is a child of the map rather than a ref on it: `useMap` is the supported
 * way to reach the instance, and it resolves once the map is actually ready,
 * whereas a ref on the map component is not populated.
 */
function MapViewport({ bounds, maxZoom }: { bounds: BoundingBox; maxZoom: number }) {
  const { current: map } = useMap();

  useEffect(() => {
    if (!map) {
      return;
    }
    // The map mounts inside a flex pane whose height is not resolved on the
    // first paint. Observing the container also keeps the canvas correct when
    // the sidebar reflows at narrow widths.
    const container = map.getContainer();
    const observer = new ResizeObserver(() => map.resize());
    observer.observe(container);
    map.resize();

    return () => observer.disconnect();
  }, [map]);

  useEffect(() => {
    if (!map) {
      return;
    }
    // Re-frame when a different stage is selected, rather than remounting the
    // map and re-downloading the style — and when a stretch is chosen, so the
    // map shows the ground the chart is showing however the stretch was asked
    // for. Only a window moves the camera: panning away to look at the
    // surrounding roads costs nothing and needs no way back.
    map.fitBounds(
      [
        [bounds[0], bounds[1]],
        [bounds[2], bounds[3]],
      ],
      { padding: 56, duration: 600, maxZoom },
    );
  }, [map, bounds, maxZoom]);

  return null;
}

/**
 * Reports the position under the pointer, so the elevation chart can mark it.
 *
 * This is a child of the map because `useMap` is what resolves the instance,
 * and projecting the candidate sample back to the screen is the only way to
 * judge nearness in the units the pointer actually moves in.
 */
function HoverLink({
  profile,
  onActiveChange,
}: {
  profile: Profile | null;
  onActiveChange: ((metres: number | null) => void) | undefined;
}) {
  const { current: map } = useMap();

  useEffect(() => {
    if (!map || !profile || !onActiveChange) {
      return;
    }

    const onMove = (event: {
      lngLat: { lng: number; lat: number };
      point: { x: number; y: number };
    }) => {
      const index = nearestSample(profile, event.lngLat.lng, event.lngLat.lat);
      const sample = index === null ? undefined : profile.samples[index];
      if (!sample) {
        onActiveChange(null);

        return;
      }
      const projected = map.project([sample.longitude, sample.latitude]);
      const near =
        Math.hypot(projected.x - event.point.x, projected.y - event.point.y) <= NEAR_ROUTE_PIXELS;
      // Reported as a distance along the route, which is the one unit that means
      // the same ground to this map and to a chart showing any stretch of it.
      onActiveChange(near ? sample.distanceMetres : null);
    };
    const onLeave = () => onActiveChange(null);

    map.on("mousemove", onMove);
    map.on("mouseout", onLeave);

    return () => {
      map.off("mousemove", onMove);
      map.off("mouseout", onLeave);
    };
  }, [map, profile, onActiveChange]);

  return null;
}

/**
 * Lets a drag along the painted route pick the stretch it covers.
 *
 * A child of the map for the same reason the hover link is: `useMap` is what
 * resolves the instance, and the gesture is judged in pixels against the
 * projected route, which needs the live camera. The stretch under the hand is
 * reported back while it is still being drawn, so the map lights it as it goes
 * — the same running answer the chart gives a drag across its own plot.
 */
function SelectionLink({
  profile,
  onPending,
  onZoomChange,
}: {
  profile: Profile | null;
  onPending: (window: DistanceWindow | null) => void;
  onZoomChange: ((window: DistanceWindow | null) => void) | undefined;
}) {
  const { current: map } = useMap();

  useEffect(() => {
    if (!map || !profile || !onZoomChange) {
      return;
    }

    return routeSelection(map.getMap(), { profile, onPending, onSelect: onZoomChange });
  }, [map, profile, onPending, onZoomChange]);

  return null;
}

/**
 * Gives the map the wheel, the fingers, and the arrow keys, or leaves them to
 * the page.
 *
 * A child of the map for the same reason its neighbours are: `useMap` is what
 * resolves the instance, and the mode is a handful of MapLibre handlers rather
 * than anything React can render.
 *
 * The one touch reading mode still hands over is a finger drawing along the
 * route, which is why the whole profile comes down here: the same measurement
 * that tells a selection from a pan tells a gesture for the route from a scroll
 * of the page.
 */
function ExplorationLink({
  exploring,
  profile,
  selectable,
}: {
  exploring: boolean;
  profile: Profile | null;
  selectable: boolean;
}) {
  const { current: map } = useMap();

  const claimsTouch = useCallback(
    (point: { clientX: number; clientY: number }) => {
      if (!map || !profile || !selectable) {
        return false;
      }

      return (
        routeMetresAt(
          map.getMap(),
          profile,
          point.clientX,
          point.clientY,
          NEAR_ROUTE_TOUCH_PIXELS,
        ) !== null
      );
    },
    [map, profile, selectable],
  );
  // Held in a ref so that a fresh profile — a different stage, say — does not
  // re-apply the mode underneath a reader who is in the middle of using it.
  const latest = useRef(claimsTouch);
  useEffect(() => {
    latest.current = claimsTouch;
  }, [claimsTouch]);

  useEffect(() => {
    if (!map) {
      return;
    }

    return mapExploration(map.getMap(), {
      exploring,
      claimsTouch: (point) => latest.current(point),
    });
  }, [map, exploring]);

  return null;
}

/**
 * Chevrons along the route, pointing the way it is ridden.
 *
 * A child of the map because the cues are sized and spaced in screen pixels, and
 * only the live camera can say what a pixel is worth on the ground. They are
 * rebuilt when the camera settles rather than on every frame of a flight: the
 * geometry is measured over the whole route, and a stage of several thousand
 * points would otherwise be re-measured sixty times a second to no visible end.
 */
function DirectionCues({ coordinates }: { coordinates: Position[] }) {
  const { current: map } = useMap();
  const [resolution, setResolution] = useState<number | null>(null);

  useEffect(() => {
    if (!map) {
      return;
    }
    const read = () => setResolution(metresPerPixel(map.getZoom(), map.getCenter().lat));
    read();
    map.on("moveend", read);
    map.on("zoomend", read);

    return () => {
      map.off("moveend", read);
      map.off("zoomend", read);
    };
  }, [map]);

  const chevrons = useMemo(
    () =>
      resolution === null ? [] : directionChevrons(coordinates, { metresPerPixel: resolution }),
    [coordinates, resolution],
  );
  // An empty collection keeps the source and its layer mounted, so the stack
  // above them never has to be rebuilt when a zoom leaves no room for a cue.
  const data = useMemo(
    () => ({
      type: "FeatureCollection" as const,
      features:
        chevrons.length === 0
          ? []
          : [
              {
                type: "Feature" as const,
                geometry: { type: "MultiLineString" as const, coordinates: chevrons },
                properties: {},
              },
            ],
    }),
    [chevrons],
  );

  return (
    <Source id="stage-direction" type="geojson" data={data}>
      {/*
       * White, and over the route rather than beside it: every class and band
       * beneath is a mid-to-dark colour, so the same cue reads on all of them,
       * and a cue drawn alongside the line would be a second thing to follow.
       */}
      <Layer
        id="stage-direction-chevrons"
        type="line"
        layout={{ "line-cap": "round", "line-join": "round" }}
        paint={{ "line-color": "#ffffff", "line-width": 2.4, "line-opacity": 0.95 }}
      />
    </Source>
  );
}

/** One terminal as a point collection, empty when the stage has no cues. */
function terminalCollection(position: Position | undefined) {
  return {
    type: "FeatureCollection" as const,
    features: position
      ? [
          {
            type: "Feature" as const,
            geometry: { type: "Point" as const, coordinates: position },
            properties: {},
          },
        ]
      : [],
  };
}

export interface RouteMapProps {
  styleUrl: string;
  /**
   * Whether `styleUrl` is the dark cartography, which picks the steepness ramp.
   *
   * Passed in rather than read from the system scheme here, because a deployment
   * with no dark style configured keeps the light basemap under a dark scheme,
   * and the edging has to match the ground it is drawn on.
   */
  darkBasemap?: boolean;
  coordinates: Position[];
  bbox: BoundingBox;
  title: string;
  /**
   * The ground under the route, addressing `coordinates` by index. Absent leaves
   * the route drawn in the accent, which is what an unclassified stage gets.
   */
  surface?: SurfaceRange[] | undefined;
  /**
   * The whole route's profile, never a zoomed one: it is what turns a point on
   * this map into a distance, and it has to answer for the whole route however
   * little of it the chart is showing.
   */
  profile?: Profile | null;
  /** Shared with the elevation chart, in metres from the start of the route. */
  activeMetres?: number | null;
  onActiveChange?: (metres: number | null) => void;
  /**
   * The stretch the chart is showing, lit while the rest of the route dims.
   *
   * Named for what it is rather than `window`, which would shadow the global of
   * that name inside this module. The camera follows it, so the map reads at
   * the scale the chart is being read at; the rest of the route stays drawn,
   * dimmed, so the stretch is still seen as part of a longer ride rather than
   * as a route of its own. A drag along the painted route can set it, through
   * `onZoomChange`; moving the camera never does, which is what keeps a look
   * around at the surrounding roads free.
   */
  zoomWindow?: DistanceWindow | null;
  /**
   * Takes the stretch a drag along the route settled on, and null for the whole
   * route again.
   *
   * Absent leaves the map reading the window and never writing it, which is what
   * every other view of a route wants: a thumbnail or a mini-map is not somewhere
   * a stretch is chosen. Present, the link runs both ways for the window alone —
   * panning and zooming the camera still change nothing but the camera.
   */
  onZoomChange?: ((window: DistanceWindow | null) => void) | undefined;
  /**
   * The class picked out of the key, lit while the rest of the route dims.
   *
   * Scattered along the whole ride rather than one stretch of it, which is the
   * question it answers: where on this route is the gravel, where does it get
   * steep. It narrows a window rather than replacing one — a reader who zooms
   * in on a climb and then asks for the gravel means the gravel on that climb.
   */
  highlight?: Highlight | null;
}

export function RouteMap({
  styleUrl,
  darkBasemap = false,
  coordinates,
  bbox,
  title,
  surface,
  profile = null,
  activeMetres = null,
  onActiveChange,
  zoomWindow = null,
  onZoomChange,
  highlight = null,
}: RouteMapProps) {
  /**
   * The stretch being drawn on the route right now, which is not a window yet.
   *
   * Kept here rather than handed up to the page: while a hand is still moving,
   * there is nothing to show the chart and nowhere to fly the camera — the
   * answer to a question half asked is to light the ground it covers so far.
   */
  const [pending, setPending] = useState<DistanceWindow | null>(null);
  /**
   * Whether the map has been handed the gestures the page otherwise keeps.
   *
   * Held here rather than by the page, because it is a property of this view of
   * this map: a reader who went looking around one stage's roads has not said
   * anything about how they want to read the rest of the page.
   */
  const [exploring, setExploring] = useState(false);
  // Rounded outwards, so the lit stretch covers every metre the chart draws.
  const windowRange = useMemo(
    () =>
      zoomWindow
        ? coordinateRange(coordinates, zoomWindow.startMetres, zoomWindow.endMetres)
        : null,
    [coordinates, zoomWindow],
  );
  // Memoised so the camera effect fires when the stretch changes rather than on
  // every render: a fresh box each time would re-fly the map on a hover. The
  // camera follows the window alone: a highlight is scattered along the ride,
  // and framing every stretch of it is the whole route with extra travel.
  const windowBounds = useMemo(
    () => (windowRange ? rangeBounds(coordinates, windowRange) : null),
    [coordinates, windowRange],
  );

  // The ground a drag along the route has covered so far, lit exactly as a
  // settled window is. The camera is deliberately left out of it: flying after a
  // hand that is still moving would drag the ground out from under the gesture
  // being drawn on it.
  const pendingRange = useMemo(
    () => (pending ? coordinateRange(coordinates, pending.startMetres, pending.endMetres) : null),
    [coordinates, pending],
  );

  // Both questions come out as the same answer — the stretches of route left
  // lit — so one mask serves every layer, and asking both at once narrows
  // rather than fights.
  const lit = useMemo(
    () =>
      litRanges(
        pendingRange ?? windowRange,
        highlight ? highlightRanges(coordinates, surface ?? [], highlight) : null,
      ),
    [coordinates, highlight, pendingRange, surface, windowRange],
  );
  const dimmed = lit !== null;

  const routeSlices = useMemo(() => routeLinesWithin(coordinates, lit), [coordinates, lit]);
  const route = useMemo(() => taggedCollection(routeSlices), [routeSlices]);

  // One feature per class rather than one with a data-driven colour, because
  // `line-dasharray` cannot be driven by a property: the pattern belongs to the
  // layer, so each class needs its own.
  const surfaceFeatures = useMemo(
    () =>
      surfaceLinesWithin(coordinates, surface ?? [], lit).map(({ kind, ...slices }) => ({
        kind,
        data: taggedCollection(slices),
      })),
    [coordinates, surface, lit],
  );

  // One feature collection per band, for the same reason the classes get one
  // each: the width and colour of an edging belong to its layer. Always one per
  // drawn band, empty where the route has no such ground, so the layers stay
  // mounted and the stack keeps the order it was built in.
  const gradientFeatures = useMemo(() => {
    const slices = gradientSlices(coordinates, lit);

    return GRADIENT_BANDS_DRAWN.map((band) => ({
      band,
      data: taggedCollection(
        slices.find((entry) => entry.band === band) ?? { inside: [], outside: [] },
      ),
    }));
  }, [coordinates, lit]);

  // The halo marks the ground on show, so it has nothing to draw until
  // something is asked: unqualified, every metre of the route counts as inside.
  const haloRoute = useMemo(
    () => taggedCollection({ inside: dimmed ? routeSlices.inside : [], outside: [] }),
    [routeSlices, dimmed],
  );

  // The position shared with the elevation chart. An empty collection keeps the
  // source mounted, so the marker appears without rebuilding the layer.
  const marker = useMemo(() => {
    const sample = profile && activeMetres !== null ? sampleAt(profile, activeMetres) : null;

    return {
      type: "FeatureCollection" as const,
      features: sample
        ? [
            {
              type: "Feature" as const,
              geometry: {
                type: "Point" as const,
                coordinates: [sample.longitude, sample.latitude],
              },
              properties: {},
            },
          ]
        : [],
    };
  }, [profile, activeMetres]);

  // The terminals and the direction they are ridden in, from the same stored
  // coordinates already drawn. Null for anything that is not a ride — a single
  // point has an end but no direction, and there is nothing honest to draw.
  const cues = useMemo(() => routeCues(coordinates), [coordinates]);
  const startPoint = useMemo(() => terminalCollection(cues?.start), [cues]);
  const finishPoint = useMemo(() => terminalCollection(cues?.finish), [cues]);
  // Opposite nudges, so a loop shows two markers side by side at the one point
  // it both leaves and returns to.
  const nudge = cues?.sharedTerminal ? TERMINAL_NUDGE_PIXELS : 0;

  // A basemap that fails to load must not fail silently: the route itself is
  // still drawn, but the operator should be able to see why the background is
  // empty.
  const onError = useCallback((event: { error?: Error }) => {
    console.error("map error:", event.error?.message ?? event);
  }, []);

  /**
   * Escape leaves whatever the map is currently holding.
   *
   * Two things can be left, so they are answered by one listener in a stated
   * order rather than by two racing to claim the key. Exploration goes first: it
   * is the one that has taken something away from the page, and a reader
   * pressing Escape over a map that has stopped scrolling is asking for the
   * scrolling back. The stretch goes second, which is the same way back the
   * chart's own control offers — carried here as well because the chart is
   * unmounted whenever the overview is collapsed, and a stretch chosen with the
   * overview closed would otherwise be a view with no way out of it.
   *
   * A stretch still being drawn is neither: `mapSelection` takes Escape in the
   * capture phase and marks it handled, so abandoning a half-drawn window never
   * also throws away the view it was being drawn on.
   */
  const zoomable = zoomWindow !== null && onZoomChange !== undefined;
  useEscapeKey(exploring || zoomable, () => {
    if (exploring) {
      setExploring(false);

      return;
    }
    onZoomChange?.(null);
  });

  return (
    <div className="route-map">
      <MapLibre
        mapStyle={styleUrl}
        onError={onError}
        initialViewState={{ bounds: bbox, fitBoundsOptions: { padding: 56 } }}
        style={{ width: "100%", height: "100%" }}
        aria-label={`Map of ${title}`}
        attributionControl={{ customAttribution: SURFACE_ATTRIBUTION }}
      >
        <MapViewport
          bounds={windowBounds ?? bbox}
          maxZoom={windowBounds ? WINDOW_MAX_ZOOM : ROUTE_MAX_ZOOM}
        />
        <HoverLink profile={profile} onActiveChange={onActiveChange} />
        <SelectionLink profile={profile} onPending={setPending} onZoomChange={onZoomChange} />
        <ExplorationLink
          exploring={exploring}
          profile={profile}
          selectable={onZoomChange !== undefined}
        />
        <NavigationControl position="top-right" showCompass={false} />
        <ScaleControl position="bottom-left" unit="metric" />
        <Source id={SOURCE_ID} type="geojson" data={route}>
          {/*
           * The casing is drawn from the whole route and stays solid under the
           * classes, so the line the rider follows is continuous even where the
           * class above it is a row of dots.
           */}
          <Layer
            id="stage-casing"
            type="line"
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{
              "line-color": "#ffffff",
              "line-opacity": dimmedOutside(CASING_OPACITY, dimmed),
              "line-width": 7,
            }}
          />
          {surfaceFeatures.length === 0 ? (
            <Layer
              id="stage-line"
              type="line"
              layout={{ "line-cap": "round", "line-join": "round" }}
              paint={{
                "line-color": ROUTE_ACCENT,
                "line-width": SURFACE_LINE_WIDTH,
                "line-opacity": dimmedOutside(1, dimmed),
              }}
            />
          ) : null}
        </Source>
        {/*
         * Steepness, as an edging under the casing: over it, it would be a
         * second line to read along the route, and the point of putting it
         * underneath is that the eye picks up the colour at the edges without
         * ever losing the route itself.
         *
         * Mounted after the casing it names and kept mounted whether or not the
         * band has any ground — an empty collection draws nothing. MapLibre
         * refuses to add a layer before one that does not exist yet, and a band
         * that came and went with the window would be re-added at whatever
         * height the stack happened to have then. Mounting every layer once
         * fixes the order for the life of the map.
         */}
        {gradientFeatures.map(({ band, data }) => (
          <Source key={band} id={`stage-gradient-${band}`} type="geojson" data={data}>
            <Layer
              id={`stage-gradient-${band}-line`}
              type="line"
              beforeId="stage-casing"
              // Butt caps, so a band ends on the point it hands over at rather
              // than half a line width into the band that follows it.
              layout={{ "line-cap": "butt", "line-join": "round" }}
              paint={{
                "line-color": BAND_COLOURS[darkBasemap ? "dark" : "light"][band] ?? ROUTE_ACCENT,
                "line-width": BAND_EDGE_WIDTH,
                "line-opacity": dimmedOutside(1, dimmed),
              }}
            />
          </Source>
        ))}
        {/*
         * A soft halo under the stretch on show, at the bottom of the stack:
         * it is a pointer, and a pointer that tints the two things it points at
         * has changed what they say. Named against the lowest band rather than
         * against the casing for that reason, and mounted last so that layer is
         * there to name.
         */}
        <Source id="stage-window" type="geojson" data={haloRoute}>
          <Layer
            id="stage-window-halo"
            type="line"
            beforeId={`stage-gradient-${LOWEST_BAND_DRAWN}-line`}
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{
              "line-color": ROUTE_ACCENT,
              // Wider than the edging it sits under, so the glow shows either
              // side of it rather than only through it.
              "line-width": BAND_EDGE_WIDTH + 6,
              "line-opacity": 0.22,
              "line-blur": 3,
            }}
          />
        </Source>
        {surfaceFeatures.map(({ kind, data }) => (
          <Source key={kind} id={`stage-surface-${kind}`} type="geojson" data={data}>
            <Layer
              id={`stage-surface-${kind}-line`}
              type="line"
              // Butt caps, so a dash is the length the palette says it is.
              layout={{ "line-cap": "butt", "line-join": "round" }}
              paint={{
                "line-color": SURFACE_STYLES[kind].colour,
                "line-width": SURFACE_LINE_WIDTH,
                "line-opacity": dimmedOutside(1, dimmed),
                ...(SURFACE_STYLES[kind].dashes.length > 0
                  ? { "line-dasharray": SURFACE_STYLES[kind].dashes }
                  : {}),
              }}
            />
          </Source>
        ))}
        <DirectionCues coordinates={coordinates} />
        {/*
         * The two ends, mounted above the route and the cues so neither is lost
         * under a line. They are told apart by shape as well as by fill — a disc
         * for the start, a ring for the finish — because a reader who cannot
         * separate the two colours still has to be able to separate the two ends.
         */}
        <Source id="stage-start" type="geojson" data={startPoint}>
          <Layer
            id="stage-start-point"
            type="circle"
            paint={{
              "circle-radius": TERMINAL_RADIUS,
              "circle-color": ROUTE_ACCENT,
              "circle-stroke-color": "#ffffff",
              "circle-stroke-width": 2.5,
              "circle-translate": [-nudge, 0],
            }}
          />
        </Source>
        <Source id="stage-finish" type="geojson" data={finishPoint}>
          <Layer
            id="stage-finish-point"
            type="circle"
            paint={{
              "circle-radius": TERMINAL_RADIUS - TERMINAL_RING_WIDTH / 2,
              "circle-color": "#ffffff",
              "circle-stroke-color": ROUTE_ACCENT,
              "circle-stroke-width": TERMINAL_RING_WIDTH,
              "circle-translate": [nudge, 0],
            }}
          />
        </Source>
        <Source id="stage-position" type="geojson" data={marker}>
          <Layer
            id="stage-position-halo"
            type="circle"
            paint={{ "circle-radius": 8, "circle-color": "#ffffff", "circle-opacity": 0.9 }}
          />
          <Layer
            id="stage-position-dot"
            type="circle"
            paint={{ "circle-radius": 5, "circle-color": ROUTE_ACCENT }}
          />
        </Source>
      </MapLibre>
      {/*
       * The cues in words. Markers and chevrons are drawn into a WebGL surface
       * that carries no text at all, so this is not a caption repeating what is
       * visible — for a reader who is not looking at the canvas it is the whole
       * of what the cues say.
       */}
      {cues ? <p className="visually-hidden">{cuesDescription(cues)}</p> : null}
      <ExploreToggle exploring={exploring} onExploringChange={setExploring} />
    </div>
  );
}
