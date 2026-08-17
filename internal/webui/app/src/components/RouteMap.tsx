/**
 * The route map.
 *
 * The basemap style is loaded from the operator-configured tile origin — the
 * one thing this otherwise Tailnet-private service fetches from outside. The
 * route itself is drawn from geometry the service already holds locally and is
 * never sent anywhere.
 */

import { setWorkerUrl } from "maplibre-gl";
import workerUrl from "maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url";
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
import type { BoundingBox, Position } from "../api/types";

// MapLibre builds its worker filename at runtime, which the bundler's static
// analysis cannot see, so the worker chunk is never emitted and tile parsing
// silently never starts. `?worker&url` makes it an explicit build input and
// yields a same-origin URL the Content-Security-Policy allows.
//
// It must be `?worker&url` rather than plain `?url`: the latter copies the file
// verbatim, leaving its `./maplibre-gl-shared.mjs` import unresolved, so the
// worker 404s on load and the map renders no tiles without reporting an error.
setWorkerUrl(workerUrl);

/** The brand accent, chosen to stay legible over both light and dark basemaps. */
const ROUTE_ACCENT = "#C8502E";
const SOURCE_ID = "stage-geometry";

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

export interface RouteMapProps {
  styleUrl: string;
  coordinates: Position[];
  bbox: BoundingBox;
  title: string;
}

export function RouteMap({ styleUrl, coordinates, bbox, title }: RouteMapProps) {
  const feature = useMemo(
    () => ({
      type: "Feature" as const,
      geometry: { type: "LineString" as const, coordinates },
      properties: {},
    }),
    [coordinates],
  );

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
        cooperativeGestures
      >
        <MapViewport bbox={bbox} />
        <NavigationControl position="top-right" showCompass={false} />
        <ScaleControl position="bottom-left" unit="metric" />
        <Source id={SOURCE_ID} type="geojson" data={feature}>
          <Layer
            id="stage-casing"
            type="line"
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{ "line-color": "#ffffff", "line-opacity": 0.85, "line-width": 7 }}
          />
          <Layer
            id="stage-line"
            type="line"
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{ "line-color": ROUTE_ACCENT, "line-width": 4 }}
          />
        </Source>
      </MapLibre>
    </div>
  );
}
