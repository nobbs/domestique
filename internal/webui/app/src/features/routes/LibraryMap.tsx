/** The route-library map assembled from reusable MapWidget layers. */

import { type ReactNode, useState } from "react";
import type { MapLayerMouseEvent } from "react-map-gl/maplibre";
import { ScaleControl } from "react-map-gl/maplibre";
import type { Basemap, BoundingBox } from "../../api/types";
import { BasemapPicker } from "../../components/map/BasemapPicker";
import { LIBRARY_HIT_LAYER, LibraryRoutes, type MapLine } from "../../components/map/LibraryRoutes";
import { MapControls } from "../../components/map/MapControls";
import { MapCredits } from "../../components/map/MapCredits";
import { MapOverlay } from "../../components/map/MapOverlay";
import { MapViewport } from "../../components/map/MapViewport";
import { MapWidget } from "../../components/map/MapWidget";
import type { Insets } from "../../lib/overlayInsets";
import type { UnitSystem } from "../../lib/units";

export type { MapLine } from "../../components/map/LibraryRoutes";

const DEFAULT_MAX_ZOOM = 14;

export interface LibraryMapProps {
  styleUrl: string;
  darkBasemap?: boolean;
  basemaps?: Basemap[];
  selectedBasemap?: string;
  onBasemapChange?: (name: string) => void;
  unitSystem?: UnitSystem;
  lines: MapLine[];
  selectedKey: string | null;
  bounds: BoundingBox | null;
  insets?: Insets;
  maxZoom?: number;
  /** The selected route's full layer stack, rendered over the library. */
  children?: ReactNode;
  extraCredit?: string | undefined;
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
  selectedKey,
  bounds,
  insets,
  maxZoom = DEFAULT_MAX_ZOOM,
  children,
  extraCredit,
  onPick,
  inertKey = null,
}: LibraryMapProps) {
  const [hoveredKey, setHoveredKey] = useState<string | null>(null);
  /*
   * Whether the reader unfolded the credit. `null` until they say, so it starts
   * folded.
   */
  const [creditChoice, setCreditChoice] = useState<boolean | null>(null);
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
          <MapOverlay>
            <MapCredits
              styleUrl={styleUrl}
              extra={extraCredit}
              choice={creditChoice}
              onChoiceChange={setCreditChoice}
            />
          </MapOverlay>
        </>
      }
    >
      <MapViewport bounds={bounds} maxZoom={maxZoom} {...(insets ? { insets } : {})} />
      <LibraryRoutes
        lines={lines}
        darkBasemap={darkBasemap}
        selectedKey={selectedKey}
        accentKey={hasOverlay ? null : selectedKey}
        hoveredKey={hoveredKey}
        {...(onPick ? { hitLayerId: LIBRARY_HIT_LAYER } : {})}
      />
      {children}
    </MapWidget>
  );
}
