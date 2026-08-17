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
import type { BoundingBox, Position, SurfaceRange } from "../api/types";
import type { Profile } from "../lib/profile";
import { nearestSample } from "../lib/profile";
import { SURFACE_LINE_WIDTH, SURFACE_STYLES, surfaceLines } from "../lib/surface";
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

/**
 * Keeps the camera framed on the selected stage and the canvas sized to its
 * pane.
 *
 * This is a child of the map rather than a ref on it: `useMap` is the supported
 * way to reach the instance, and it resolves once the map is actually ready,
 * whereas a ref on the map component is not populated.
 */
function MapViewport({ bbox }: { bbox: BoundingBox }) {
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
    // map and re-downloading the style.
    map.fitBounds(
      [
        [bbox[0], bbox[1]],
        [bbox[2], bbox[3]],
      ],
      { padding: 56, duration: 600, maxZoom: 15 },
    );
  }, [map, bbox]);

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
  onActiveChange: ((index: number | null) => void) | undefined;
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
      onActiveChange(near ? index : null);
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
  /** Shared with the elevation chart, as an index into the profile samples. */
  profile?: Profile | null;
  activeIndex?: number | null;
  onActiveChange?: (index: number | null) => void;
}

export function RouteMap({
  styleUrl,
  coordinates,
  bbox,
  title,
  surface,
  profile = null,
  activeIndex = null,
  onActiveChange,
}: RouteMapProps) {
  const feature = useMemo(
    () => ({
      type: "Feature" as const,
      geometry: { type: "LineString" as const, coordinates },
      properties: {},
    }),
    [coordinates],
  );

  // One feature per class rather than one with a data-driven colour, because
  // `line-dasharray` cannot be driven by a property: the pattern belongs to the
  // layer, so each class needs its own.
  const surfaceFeatures = useMemo(
    () =>
      surfaceLines(coordinates, surface ?? []).map(({ kind, lines }) => ({
        kind,
        data: {
          type: "Feature" as const,
          geometry: { type: "MultiLineString" as const, coordinates: lines },
          properties: {},
        },
      })),
    [coordinates, surface],
  );

  // The position shared with the elevation chart. An empty collection keeps the
  // source mounted, so the marker appears without rebuilding the layer.
  const marker = useMemo(() => {
    const sample = profile && activeIndex !== null ? profile.samples[activeIndex] : undefined;

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
  }, [profile, activeIndex]);

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
        <MapViewport bbox={bbox} />
        <HoverLink profile={profile} onActiveChange={onActiveChange} />
        <NavigationControl position="top-right" showCompass={false} />
        <ScaleControl position="bottom-left" unit="metric" />
        <Source id={SOURCE_ID} type="geojson" data={feature}>
          {/*
           * The casing is drawn from the whole route and stays solid under the
           * classes, so the line the rider follows is continuous even where the
           * class above it is a row of dots.
           */}
          <Layer
            id="stage-casing"
            type="line"
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{ "line-color": "#ffffff", "line-opacity": 0.85, "line-width": 7 }}
          />
          {surfaceFeatures.length === 0 ? (
            <Layer
              id="stage-line"
              type="line"
              layout={{ "line-cap": "round", "line-join": "round" }}
              paint={{ "line-color": ROUTE_ACCENT, "line-width": SURFACE_LINE_WIDTH }}
            />
          ) : null}
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
