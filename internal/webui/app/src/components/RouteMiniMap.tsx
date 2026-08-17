/**
 * A card-sized basemap with the route drawn on it.
 *
 * It is deliberately non-interactive: the card is a link, so the map must not
 * capture clicks or scrolling. It carries no attribution control of its own
 * either — a grid of them would be unreadable — so the page that renders the
 * grid is responsible for attributing the tile source once.
 *
 * The SVG shape stays on top until the basemap has drawn, so a card shows its
 * route immediately rather than a grey box waiting on the network.
 */

import { useCallback, useEffect, useMemo, useState } from "react";
import { Layer, Map as MapLibre, Source, useMap } from "react-map-gl/maplibre";
import "maplibre-gl/dist/maplibre-gl.css";
import type { BoundingBox, Position } from "../api/types";
// Configures the shared worker pool; without it this map renders no tiles.
import "../lib/maplibre";
import { RouteThumbnail } from "./RouteThumbnail";

const ROUTE_ACCENT = "#C8502E";
const SOURCE_ID = "stage-preview";

/**
 * Reports when the basemap has actually drawn.
 *
 * This is a child of the map rather than an `onLoad` prop: that callback does
 * not fire reliably here, whereas `useMap` resolves once the instance exists
 * and its `idle` event marks the end of the first render.
 */
function DrawnSignal({ onDrawn }: { onDrawn: () => void }) {
  const { current: map } = useMap();

  useEffect(() => {
    if (!map) {
      return;
    }
    if (map.loaded()) {
      onDrawn();

      return;
    }
    map.on("idle", onDrawn);

    return () => {
      map.off("idle", onDrawn);
    };
  }, [map, onDrawn]);

  return null;
}

export interface RouteMiniMapProps {
  styleUrl: string;
  coordinates: Position[];
  bbox: BoundingBox;
  title: string;
}

export function RouteMiniMap({ styleUrl, coordinates, bbox, title }: RouteMiniMapProps) {
  const [drawn, setDrawn] = useState(false);
  const onDrawn = useCallback(() => setDrawn(true), []);

  const feature = useMemo(
    () => ({
      type: "Feature" as const,
      geometry: { type: "LineString" as const, coordinates },
      properties: {},
    }),
    [coordinates],
  );

  return (
    <span className="route-minimap" data-loaded={drawn}>
      <MapLibre
        mapStyle={styleUrl}
        initialViewState={{ bounds: bbox, fitBoundsOptions: { padding: 14 } }}
        style={{ width: "100%", height: "100%" }}
        interactive={false}
        attributionControl={false}
        aria-hidden="true"
      >
        <DrawnSignal onDrawn={onDrawn} />
        <Source id={SOURCE_ID} type="geojson" data={feature}>
          <Layer
            id="stage-preview-casing"
            type="line"
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{ "line-color": "#ffffff", "line-opacity": 0.8, "line-width": 4 }}
          />
          <Layer
            id="stage-preview-line"
            type="line"
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{ "line-color": ROUTE_ACCENT, "line-width": 2 }}
          />
        </Source>
      </MapLibre>
      <span className="route-minimap__shape">
        <RouteThumbnail coordinates={coordinates} title={title} />
      </span>
    </span>
  );
}
