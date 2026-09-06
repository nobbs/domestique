/**
 * One ride's recorded track, on a canvas of its own: the library map belongs to
 * the entry page, and a ride is not part of the library.
 */

import { useQuery } from "@tanstack/react-query";
import { webUIConfigQuery } from "../../api/queries";
import type { BoundingBox, Position } from "../../api/types";
import { CartographyProvider } from "../../components/map/CartographyContext";
import { MapViewport } from "../../components/map/MapViewport";
import { MapWidget } from "../../components/map/MapWidget";
import { basemapFor, useBasemapChoice, usePrefersDarkScheme } from "../../lib/basemap";
import type { Profile } from "../../lib/profile";
import { resolvesDark, useThemeChoice } from "../../lib/theme";
import { RouteOverlay } from "../routes/RouteOverlay";

/** As close as a ride is framed, so a short loop is not zoomed to the tarmac. */
const TRACK_MAX_ZOOM = 15;

export interface ActivityMapProps {
  coordinates: Position[];
  bounds: BoundingBox;
  profile: Profile | null;
  activeMetres: number | null;
  onActiveChange: (metres: number | null) => void;
}

export function ActivityMap({
  coordinates,
  bounds,
  profile,
  activeMetres,
  onActiveChange,
}: ActivityMapProps) {
  const config = useQuery(webUIConfigQuery());
  const [themeChoice] = useThemeChoice();
  const [basemapChoice] = useBasemapChoice();
  const prefersDark = usePrefersDarkScheme();
  const basemap = config.data
    ? basemapFor(config.data, resolvesDark(themeChoice, prefersDark), basemapChoice)
    : null;

  if (!basemap) {
    return null;
  }

  return (
    <CartographyProvider dark={basemap.dark}>
      <MapWidget styleUrl={basemap.styleUrl} ariaLabel="Recorded track">
        <MapViewport bounds={bounds} maxZoom={TRACK_MAX_ZOOM} />
        <RouteOverlay
          coordinates={coordinates}
          profile={profile}
          activeMetres={activeMetres}
          onActiveChange={onActiveChange}
        />
      </MapWidget>
    </CartographyProvider>
  );
}
