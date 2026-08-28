/**
 * The MapLibre canvas that every map in the application is built on.
 *
 * What it is handed arrives in two parts, because a change of style asks
 * something different of each. Layers cannot outlive the style holding them,
 * so they wait for it and are built again when it is replaced. Furniture is
 * DOM standing beside the canvas and is never taken down at all.
 */

import {
  type ComponentType,
  type CSSProperties,
  createContext,
  type ReactNode,
  useContext,
  useState,
} from "react";
import type { MapLayerMouseEvent } from "react-map-gl/maplibre";
import { Map as MapLibre } from "react-map-gl/maplibre";
import "maplibre-gl/dist/maplibre-gl.css";
// Configures the shared worker pool; without it this map renders no tiles.
import "../../lib/maplibre";

/** What `MapWidget` hands whichever canvas implementation it renders. */
export interface MapImplementationProps {
  mapStyle: string;
  onLoad?: () => void;
  onIdle?: () => void;
  style?: CSSProperties;
  "aria-label"?: string;
  attributionControl?: false;
  interactiveLayerIds?: string[];
  cursor?: string;
  onClick?: (event: MapLayerMouseEvent) => void;
  onMouseMove?: (event: MapLayerMouseEvent) => void;
  onMouseOut?: () => void;
  children?: ReactNode;
}

/**
 * The canvas `MapWidget` renders, real MapLibre unless a story overrides it.
 *
 * Live tiles and WebGL rasterization make the real canvas non-deterministic
 * across runs, so Chromatic cannot snapshot a story that mounts it. Storybook
 * cannot mock `react-map-gl/maplibre` itself — its entry file re-exports
 * everything via `export *`, a shape Storybook's automock explicitly refuses
 * to transform — so the seam lives here instead: `ChromeMap` in
 * `src/storybook/mapMock.tsx` provides a deterministic placeholder through
 * this context for the stories that opt in via that decorator; every other
 * consumer (the app itself, and stories reviewing real route geometry) gets
 * the real canvas untouched.
 */
export const MapImplementationContext = createContext<ComponentType<MapImplementationProps> | null>(
  null,
);

/**
 * The real canvas, wrapped rather than handed to the context directly: MapLibre's
 * export is a `ForwardRefExoticComponent`, not the plain `ComponentType` the context
 * expects, and forwarding its props through a typed function catches a prop this
 * component starts relying on that MapLibre no longer accepts — a plain cast to
 * `ComponentType` would not.
 */
function RealMap(props: MapImplementationProps) {
  return <MapLibre {...props} />;
}

export interface MapWidgetProps {
  /** The cartography to load. The map never chooses a style for its caller. */
  styleUrl: string;
  /**
   * Anything the style has a say over: sources, layers, and the camera that
   * frames them. Held back until the style has loaded, and mounted again from
   * scratch when a new one replaces it, because none of it can be added to a
   * style that is not there.
   */
  children?: ReactNode;
  /**
   * Controls, overlays, and the rest of the furniture around the map.
   *
   * Mounted with the map and left alone through every change of style. It is
   * ordinary DOM that owes the cartography nothing, and holding it back with
   * the layers made the whole corner of the map blink each time a reader
   * chose a different basemap.
   */
  furniture?: ReactNode;
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
  furniture,
  ariaLabel = "Map",
  interactiveLayerIds = [],
  cursor = "",
  onClick,
  onMouseMove,
  onMouseOut,
}: MapWidgetProps) {
  const [loadedStyleUrl, setLoadedStyleUrl] = useState<string | null>(null);
  const MapComponent = useContext(MapImplementationContext) ?? RealMap;

  return (
    <div className="route-map">
      <MapComponent
        mapStyle={styleUrl}
        onLoad={() => setLoadedStyleUrl(styleUrl)}
        // `idle`, not `styledata`, which cannot answer this question. Changing
        // the basemap fires `styledata` several times, and the only one that
        // reports `isStyleLoaded()` — the first, microseconds after the swap —
        // is still describing the style being replaced. So the layers came back
        // on the strength of the outgoing style having been ready, and every
        // later `styledata` reported false. A swap that missed that one
        // accidental true never saw another event, and the routes stayed gone
        // for good.
        //
        // `idle` is emitted once the new style is loaded and everything it
        // asked for has been drawn, which is the first moment the layers below
        // can actually be added.
        onIdle={() => setLoadedStyleUrl(styleUrl)}
        style={{ width: "100%", height: "100%" }}
        aria-label={ariaLabel}
        attributionControl={false}
        interactiveLayerIds={interactiveLayerIds}
        cursor={cursor}
        {...(onClick ? { onClick } : {})}
        {...(onMouseMove ? { onMouseMove } : {})}
        {...(onMouseOut ? { onMouseOut } : {})}
      >
        {furniture}
        {loadedStyleUrl === styleUrl ? children : null}
      </MapComponent>
    </div>
  );
}
