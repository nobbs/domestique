/** The MapLibre canvas that every map in the application is built on. */

import { type ReactNode, useState } from "react";
import type { MapLayerMouseEvent } from "react-map-gl/maplibre";
import { Map as MapLibre } from "react-map-gl/maplibre";
import "maplibre-gl/dist/maplibre-gl.css";
// Configures the shared worker pool; without it this map renders no tiles.
import "../../lib/maplibre";

export interface MapWidgetProps {
  /** The cartography to load. The map never chooses a style for its caller. */
  styleUrl: string;
  /** Layers and furniture drawn after the style has loaded. */
  children?: ReactNode;
  /** A useful name for the map's canvas. */
  ariaLabel?: string;
  /** The MapLibre layers that can answer pointer events. */
  interactiveLayerIds?: string[];
  cursor?: string;
  onClick?: (event: MapLayerMouseEvent) => void;
  onMouseMove?: (event: MapLayerMouseEvent) => void;
  onMouseOut?: () => void;
}

export function MapWidget({
  styleUrl,
  children,
  ariaLabel = "Map",
  interactiveLayerIds = [],
  cursor = "",
  onClick,
  onMouseMove,
  onMouseOut,
}: MapWidgetProps) {
  const [loaded, setLoaded] = useState(false);

  return (
    <div className="route-map">
      <MapLibre
        mapStyle={styleUrl}
        onError={(event) => console.error("map error:", event.error?.message ?? event)}
        onLoad={() => setLoaded(true)}
        style={{ width: "100%", height: "100%" }}
        aria-label={ariaLabel}
        attributionControl={false}
        interactiveLayerIds={interactiveLayerIds}
        cursor={cursor}
        {...(onClick ? { onClick } : {})}
        {...(onMouseMove ? { onMouseMove } : {})}
        {...(onMouseOut ? { onMouseOut } : {})}
      >
        {loaded ? children : null}
      </MapLibre>
    </div>
  );
}
