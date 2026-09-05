/** The route-library map assembled from reusable MapWidget layers. */

import { type ReactNode, useState } from "react";
import type { MapLayerMouseEvent } from "react-map-gl/maplibre";
import { ScaleControl } from "react-map-gl/maplibre";
import type { Basemap, BoundingBox } from "../../api/types";
import { BasemapPicker } from "../../components/map/BasemapPicker";
import { CartographyProvider } from "../../components/map/CartographyContext";
import { MapControls } from "../../components/map/MapControls";
import { MapViewport } from "../../components/map/MapViewport";
import { MapWidget } from "../../components/map/MapWidget";
import { OverlayToggle } from "../../components/map/OverlayToggle";
import { ROUTE_MAX_ZOOM } from "../../lib/cartography";
import { MEASURES, type MeasureKey } from "../../lib/measures";
import type { Insets } from "../../lib/overlayInsets";
import { MEASURE_ICON } from "./ConditionsPicker";
import {
  LIBRARY_HIT_LAYER,
  LIBRARY_LINE_LAYER,
  LibraryRoutes,
  type MapLine,
} from "./LibraryRoutes";
import { ScalarOverlay } from "./ScalarOverlay";
import { WindOverlay } from "./WindOverlay";

export type { MapLine } from "./LibraryRoutes";

/** The model's variable for each scalar measure, in the units its bands take. */
const SCALAR_VARIABLE: Partial<Record<MeasureKey, string>> = {
  temperature: "temperature_2m",
  rain: "precipitation",
  cloud: "cloud_cover",
};

const SCALAR_OVERLAYS = MEASURES.flatMap((measure) => {
  const variable = SCALAR_VARIABLE[measure.key];

  return variable ? [{ measure, variable }] : [];
});

export interface LibraryMapProps {
  styleUrl: string;
  darkBasemap?: boolean;
  basemaps?: Basemap[];
  selectedBasemap?: string;
  onBasemapChange?: (name: string) => void;
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
  const [overlays, setOverlays] = useState<ReadonlySet<MeasureKey>>(new Set());
  const toggleOverlay = (key: MeasureKey, on: boolean) =>
    setOverlays((current) => {
      const next = new Set(current);
      if (on) {
        next.add(key);
      } else {
        next.delete(key);
      }

      return next;
    });
  const hasOverlay = children !== null && children !== undefined;
  // An opened route has the map to itself: the library is put away, so there is
  // nothing under the pointer to light, point at, or land a pick on.
  const pickable = onPick !== undefined && !hasOverlay;
  const focusedKey = hasOverlay ? null : hoveredKey;

  return (
    <CartographyProvider dark={darkBasemap}>
      <MapWidget
        styleUrl={styleUrl}
        ariaLabel="Map of the route library"
        interactiveLayerIds={pickable ? [LIBRARY_HIT_LAYER] : []}
        cursor={focusedKey !== null && focusedKey !== inertKey ? "pointer" : ""}
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
            <ScaleControl position="bottom-left" unit="metric" />
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
              {MEASURES.map((measure) => {
                const Icon = MEASURE_ICON[measure.key];

                return (
                  <OverlayToggle
                    key={measure.key}
                    on={overlays.has(measure.key)}
                    onChange={(on) => toggleOverlay(measure.key, on)}
                    icon={<Icon stroke={1.6} />}
                    subject={measure.label.toLowerCase()}
                    title={`${measure.label} now, from ICON-D2`}
                  />
                );
              })}
            </MapControls>
          </>
        }
      >
        <MapViewport bounds={bounds} maxZoom={maxZoom} {...(insets ? { insets } : {})} />
        <LibraryRoutes
          lines={lines}
          pickedKey={pickedKey}
          overlaid={hasOverlay}
          hoveredKey={focusedKey}
          {...(onPick ? { hitLayerId: LIBRARY_HIT_LAYER } : {})}
        />
        {/* After the library, whose line it is ordered beneath; that layer is
            always mounted and only hidden while a route is open. */}
        {SCALAR_OVERLAYS.map(({ measure, variable }) => (
          <ScalarOverlay
            key={measure.key}
            measure={measure}
            variable={variable}
            on={overlays.has(measure.key)}
            beforeId={LIBRARY_LINE_LAYER}
          />
        ))}
        <WindOverlay on={overlays.has("wind")} beforeId={LIBRARY_LINE_LAYER} />
        {children}
      </MapWidget>
    </CartographyProvider>
  );
}
