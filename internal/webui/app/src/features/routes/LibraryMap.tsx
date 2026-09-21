/** The route-library map assembled from reusable MapWidget layers. */

import { type ReactNode, useState } from "react";
import { ScaleControl } from "react-map-gl/maplibre";
import type { Basemap, BoundingBox } from "../../api/types";
import { BasemapPicker } from "../../components/map/BasemapPicker";
import { CartographyProvider } from "../../components/map/CartographyContext";
import { MapControls } from "../../components/map/MapControls";
import { MapViewport } from "../../components/map/MapViewport";
import { MapWidget } from "../../components/map/MapWidget";
import { WeatherOverlayPicker } from "../../components/map/WeatherOverlayPicker";
import { ROUTE_MAX_ZOOM } from "../../lib/cartography";
import { MEASURES, type MeasureKey } from "../../lib/measures";
import type { Insets } from "../../lib/overlayInsets";
import { LIBRARY_LINE_LAYER, LibraryRoutes, type MapLine } from "./LibraryRoutes";
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
  /** False for a small preview: no scale, zoom, basemap or weather controls. */
  controls?: boolean;
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
  controls = true,
}: LibraryMapProps) {
  const [pickerOpen, setPickerOpen] = useState(false);
  const [weatherPickerOpen, setWeatherPickerOpen] = useState(false);
  const [overlays, setOverlays] = useState<ReadonlySet<MeasureKey>>(new Set());
  const [hoursAhead, setHoursAhead] = useState(0);
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

  return (
    <CartographyProvider dark={darkBasemap}>
      <MapWidget
        styleUrl={styleUrl}
        ariaLabel="Map of the route library"
        // Everything the cartography has no say over. It stays mounted while a
        // new basemap loads, so choosing one does not take the controls away
        // from under the hand that just used them.
        furniture={
          controls ? (
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
                <WeatherOverlayPicker
                  measures={MEASURES}
                  selected={overlays}
                  onToggle={toggleOverlay}
                  hoursAhead={hoursAhead}
                  onHoursAheadChange={setHoursAhead}
                  expanded={weatherPickerOpen}
                  onExpandedChange={setWeatherPickerOpen}
                />
              </MapControls>
            </>
          ) : null
        }
      >
        <MapViewport bounds={bounds} maxZoom={maxZoom} {...(insets ? { insets } : {})} />
        <LibraryRoutes lines={lines} pickedKey={pickedKey} overlaid={hasOverlay} />
        {/* After the library, whose line it is ordered beneath; that layer is
            always mounted and only hidden while a route is open. */}
        {SCALAR_OVERLAYS.map(({ measure, variable }) => (
          <ScalarOverlay
            key={measure.key}
            measure={measure}
            variable={variable}
            on={overlays.has(measure.key)}
            hoursAhead={hoursAhead}
            beforeId={LIBRARY_LINE_LAYER}
          />
        ))}
        <WindOverlay
          on={overlays.has("wind")}
          hoursAhead={hoursAhead}
          beforeId={LIBRARY_LINE_LAYER}
        />
        {children}
      </MapWidget>
    </CartographyProvider>
  );
}
