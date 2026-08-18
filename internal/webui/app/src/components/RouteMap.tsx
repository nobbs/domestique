/**
 * The route map.
 *
 * The basemap style is loaded from the operator-configured tile origin — the
 * one thing this otherwise Tailnet-private service fetches from outside. The
 * route itself is drawn from geometry the service already holds locally and is
 * never sent anywhere.
 */

import { useCallback, useEffect, useMemo } from "react";
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
import type { DistanceWindow, Profile } from "../lib/profile";
import { coordinateRange, nearestSample, rangeBounds, sampleAt } from "../lib/profile";
import { gradientSlices, routeLinesWithin } from "../lib/routeLines";
import { SURFACE_LINE_WIDTH, SURFACE_STYLES, surfaceLinesWithin } from "../lib/surface";
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
 * The steepness ramp, band by band, mirroring `--band-*` in `index.css`.
 *
 * Repeated here rather than read from the stylesheet because MapLibre paints
 * from a style document and knows nothing of CSS custom properties. The light
 * theme's values are the ones used: the basemap is whatever the operator
 * configured and does not follow the page, which is the same reason the surface
 * palette and the route accent are fixed.
 *
 * Only the steeper two are ever drawn — see `GRADIENT_BANDS_DRAWN`.
 */
const BAND_COLOURS = ["#e0ac2c", "#c2542a", "#63202b"] as const;

/**
 * The bands worth ink on a map.
 *
 * Gentle ground is most of most routes, and painting it would edge the whole
 * line in gold to say the unremarkable thing. Left unpainted, the white casing
 * stands for it, and colour appears only where the road starts to bite. The
 * chart still shows all three, which is where a reader goes to compare one
 * stretch against another; the map is answering "where does it get steep?".
 */
const GRADIENT_BANDS_DRAWN = [1, 2] as const;

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
 * How much of the route survives outside the stretch the chart is showing.
 *
 * Dimmed rather than hidden: the ride does not stop at the edges of the window,
 * and a route drawn only in the middle would read as a shorter route rather than
 * as a closer look at a longer one. A quarter is faint enough that the eye lands
 * on the stretch first and still dark enough to follow the road it came in on.
 */
const OUTSIDE_OPACITY = 0.25;

/**
 * An opacity that drops away outside the stretch on show.
 *
 * One expression over one tagged source rather than a second stack of layers:
 * `line-opacity` is a layer property, and one layer per class is exactly what
 * keeps a class's dash pattern identical on both sides of the window's edge.
 * Written as a function because a bare array in a `const` widens to `unknown[]`
 * and stops matching the tuple union the paint property is typed as.
 */
function dimmedOutside(
  full: number,
  windowed: boolean,
): DataDrivenPropertyValueSpecification<number> {
  return windowed ? ["case", ["get", "shown"], full, full * OUTSIDE_OPACITY] : full;
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
    // map and re-downloading the style — and when the chart is zoomed into a
    // stretch, so the map answers the same question the chart was asked. The
    // link runs one way only: nothing on the map sets the chart's window, so
    // panning away to look at the surrounding roads costs nothing and needs no
    // way back.
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
 * How near the route the pointer must be, in pixels, to mark a position.
 *
 * The painted line is a few pixels wide, which is a pinpoint to aim at. Testing
 * against the projected position instead gives a hit area comfortably larger
 * than the mark, so following the route with the pointer actually works.
 */
const HOVER_RADIUS_PIXELS = 22;

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
        Math.hypot(projected.x - event.point.x, projected.y - event.point.y) <= HOVER_RADIUS_PIXELS;
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

export interface RouteMapProps {
  styleUrl: string;
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
   * as a route of its own. The link runs this way only — the map never sets the
   * window, which is what keeps a look around at the surrounding roads free.
   */
  zoomWindow?: DistanceWindow | null;
}

export function RouteMap({
  styleUrl,
  coordinates,
  bbox,
  title,
  surface,
  profile = null,
  activeMetres = null,
  onActiveChange,
  zoomWindow = null,
}: RouteMapProps) {
  // Rounded outwards, so the lit stretch covers every metre the chart draws.
  const windowRange = useMemo(
    () =>
      zoomWindow
        ? coordinateRange(coordinates, zoomWindow.startMetres, zoomWindow.endMetres)
        : null,
    [coordinates, zoomWindow],
  );
  const windowed = windowRange !== null;

  // Memoised so the camera effect fires when the stretch changes rather than on
  // every render: a fresh box each time would re-fly the map on a hover.
  const windowBounds = useMemo(
    () => (windowRange ? rangeBounds(coordinates, windowRange) : null),
    [coordinates, windowRange],
  );

  const routeSlices = useMemo(
    () => routeLinesWithin(coordinates, windowRange),
    [coordinates, windowRange],
  );
  const route = useMemo(() => taggedCollection(routeSlices), [routeSlices]);

  // One feature per class rather than one with a data-driven colour, because
  // `line-dasharray` cannot be driven by a property: the pattern belongs to the
  // layer, so each class needs its own.
  const surfaceFeatures = useMemo(
    () =>
      surfaceLinesWithin(coordinates, surface ?? [], windowRange).map(({ kind, ...slices }) => ({
        kind,
        data: taggedCollection(slices),
      })),
    [coordinates, surface, windowRange],
  );

  // One feature collection per band, for the same reason the classes get one
  // each: the width and colour of an edging belong to its layer. Always one per
  // drawn band, empty where the route has no such ground, so the layers stay
  // mounted and the stack keeps the order it was built in.
  const gradientFeatures = useMemo(() => {
    const slices = gradientSlices(coordinates, windowRange);

    return GRADIENT_BANDS_DRAWN.map((band) => ({
      band,
      data: taggedCollection(
        slices.find((entry) => entry.band === band) ?? { inside: [], outside: [] },
      ),
    }));
  }, [coordinates, windowRange]);

  // The halo marks the stretch on show, so it has nothing to draw until there
  // is one: unzoomed, every metre of the route counts as inside.
  const haloRoute = useMemo(
    () => taggedCollection({ inside: windowed ? routeSlices.inside : [], outside: [] }),
    [routeSlices, windowed],
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

  // A basemap that fails to load must not fail silently: the route itself is
  // still drawn, but the operator should be able to see why the background is
  // empty.
  const onError = useCallback((event: { error?: Error }) => {
    console.error("map error:", event.error?.message ?? event);
  }, []);

  return (
    <div className="route-map">
      <MapLibre
        mapStyle={styleUrl}
        onError={onError}
        initialViewState={{ bounds: bbox, fitBoundsOptions: { padding: 56 } }}
        style={{ width: "100%", height: "100%" }}
        aria-label={`Map of ${title}`}
        attributionControl={{ customAttribution: SURFACE_ATTRIBUTION }}
        cooperativeGestures
      >
        <MapViewport
          bounds={windowBounds ?? bbox}
          maxZoom={windowBounds ? WINDOW_MAX_ZOOM : ROUTE_MAX_ZOOM}
        />
        <HoverLink profile={profile} onActiveChange={onActiveChange} />
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
              "line-opacity": dimmedOutside(CASING_OPACITY, windowed),
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
                "line-opacity": dimmedOutside(1, windowed),
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
                "line-color": BAND_COLOURS[band] ?? ROUTE_ACCENT,
                "line-width": BAND_EDGE_WIDTH,
                "line-opacity": dimmedOutside(1, windowed),
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
                "line-opacity": dimmedOutside(1, windowed),
                ...(SURFACE_STYLES[kind].dashes.length > 0
                  ? { "line-dasharray": SURFACE_STYLES[kind].dashes }
                  : {}),
              }}
            />
          </Source>
        ))}
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
    </div>
  );
}
