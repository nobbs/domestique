/** The route-library map assembled from reusable MapWidget layers. */

import { type ReactNode, useState } from "react";
import type { MapLayerMouseEvent } from "react-map-gl/maplibre";
import { ScaleControl } from "react-map-gl/maplibre";
import type { Basemap, BoundingBox } from "../../api/types";
import { BasemapPicker } from "../../components/map/BasemapPicker";
import { MapControls } from "../../components/map/MapControls";
import { MapViewport } from "../../components/map/MapViewport";
import { MapWidget } from "../../components/map/MapWidget";
import { ROUTE_MAX_ZOOM } from "../../lib/cartography";
import type { Insets } from "../../lib/overlayInsets";
import type { UnitSystem } from "../../lib/units";
import { LIBRARY_HIT_LAYER, LibraryRoutes, type MapLine } from "./LibraryRoutes";

export type { MapLine } from "./LibraryRoutes";

export interface LibraryMapProps {
  styleUrl: string;
  darkBasemap?: boolean;
  basemaps?: Basemap[];
  selectedBasemap?: string;
  onBasemapChange?: (name: string) => void;
  unitSystem?: UnitSystem;
  lines: MapLine[];
  pickedKey: string | null;
  bounds: BoundingBox | null;
  insets?: Insets;
  maxZoom?: number;
  /** The selected route's full layer stack, rendered over the library. */
  children?: ReactNode;
  onPick?: (key: string) => void;
  inertKey?: string | null;
}

function keyAt(event: MapLayerMouseEvent): string | null {
  const key = event.features?.[0]?.properties?.key;

  return typeof key === "string" ? key : null;
}

export function LibraryMap({
  styleUrl,
  darkBasemap = false,
  basemaps = [],
  selectedBasemap = "",
  onBasemapChange,
  unitSystem = "metric",
  lines,
  pickedKey,
  bounds,
  insets,
  maxZoom = ROUTE_MAX_ZOOM,
  children,
  onPick,
  inertKey = null,
}: LibraryMapProps) {
  const [hoveredKey, setHoveredKey] = useState<string | null>(null);
  const [pickerOpen, setPickerOpen] = useState(false);
  const hasOverlay = children !== null && children !== undefined;

  return (
    <MapWidget
      styleUrl={styleUrl}
      ariaLabel="Map of the route library"
      interactiveLayerIds={onPick ? [LIBRARY_HIT_LAYER] : []}
      cursor={hoveredKey !== null && hoveredKey !== inertKey ? "pointer" : ""}
      onMouseMove={(event) => setHoveredKey(keyAt(event))}
      onMouseOut={() => setHoveredKey(null)}
      onClick={(event) => {
        const key = keyAt(event);
        if (key !== null) {
          onPick?.(key);
        }
      }}
      // Everything the cartography has no say over. It stays mounted while a
      // new basemap loads, so choosing one does not take the controls away
      // from under the hand that just used them.
      furniture={
        <>
          <ScaleControl position="bottom-left" unit={unitSystem} />
          <MapControls>
            {onBasemapChange ? (
              <BasemapPicker
                basemaps={basemaps}
                selectedName={selectedBasemap}
                onSelect={onBasemapChange}
                expanded={pickerOpen}
                onExpandedChange={setPickerOpen}
              />
            ) : null}
          </MapControls>
        </>
      }
    >
      <MapViewport bounds={bounds} maxZoom={maxZoom} {...(insets ? { insets } : {})} />
      <LibraryRoutes
        lines={lines}
        darkBasemap={darkBasemap}
        pickedKey={pickedKey}
        accentKey={hasOverlay ? null : pickedKey}
        hoveredKey={hoveredKey}
        {...(onPick ? { hitLayerId: LIBRARY_HIT_LAYER } : {})}
      />
      {children}
    </MapWidget>
  );
}
