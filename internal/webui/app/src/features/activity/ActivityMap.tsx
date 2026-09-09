/**
 * One ride's recorded track, on a canvas of its own: the library map belongs to
 * the entry page, and a ride is not part of the library.
 */

import { IconArrowsMaximize, IconArrowsMinimize } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { webUIConfigQuery } from "../../api/queries";
import type { ActivityTrackWorld, BoundingBox, Position } from "../../api/types";
import { Button } from "../../components/Button";
import { CartographyProvider } from "../../components/map/CartographyContext";
import { MapControls } from "../../components/map/MapControls";
import { MapViewport } from "../../components/map/MapViewport";
import { MapWidget } from "../../components/map/MapWidget";
import { basemapFor, useBasemapChoice, usePrefersDarkScheme } from "../../lib/basemap";
import { WINDOW_MAX_ZOOM } from "../../lib/cartography";
import type { DistanceWindow, Profile } from "../../lib/profile";
import { resolvesDark, useThemeChoice } from "../../lib/theme";
import { RouteOverlay } from "../routes/RouteOverlay";
import { ZwiftWorldMap } from "./ZwiftWorldMap";

/** As close as a whole ride is framed, so a short loop is not zoomed to the tarmac. */
const TRACK_MAX_ZOOM = 15;

export interface ActivityMapProps {
  coordinates: Position[];
  /** The whole track's box; overridden by `windowBounds` while zoomed. */
  bounds: BoundingBox;
  /** Framed instead of `bounds`, and at `WINDOW_MAX_ZOOM`, while a stretch is zoomed. */
  windowBounds?: BoundingBox | null;
  profile: Profile | null;
  /**
   * The profile the elevation chart is actually drawing, windowed while zoomed.
   * See `RouteOverlay`'s own `activeProfile` for why it is kept apart from
   * `profile`.
   */
  activeProfile?: Profile | null;
  activeMetres: number | null;
  onActiveChange: (metres: number | null) => void;
  /** The stretch on show; a drag along the track can set it through `onZoomChange`. */
  zoomWindow?: DistanceWindow | null;
  onZoomChange?: (window: DistanceWindow | null) => void;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
  /**
   * The virtual world this ride was ridden in. Present replaces the whole
   * cartography with that world's artwork: its coordinates are not the
   * ground's, so no basemap they were drawn over could be true.
   */
  world?: ActivityTrackWorld | null | undefined;
}

export function ActivityMap({
  coordinates,
  bounds,
  windowBounds = null,
  profile,
  activeProfile = null,
  activeMetres,
  onActiveChange,
  zoomWindow = null,
  onZoomChange,
  expanded,
  onExpandedChange,
  world = null,
}: ActivityMapProps) {
  // A world ride draws over artwork and needs no basemap, so no config read.
  const config = useQuery({ ...webUIConfigQuery(), enabled: !world });
  const [themeChoice] = useThemeChoice();
  const [basemapChoice] = useBasemapChoice();
  const prefersDark = usePrefersDarkScheme();
  const basemap = config.data
    ? basemapFor(config.data, resolvesDark(themeChoice, prefersDark), basemapChoice)
    : null;

  if (world) {
    return (
      <ZwiftWorldMap
        world={world}
        coordinates={coordinates}
        profile={profile}
        activeMetres={activeMetres}
        onActiveChange={onActiveChange}
        expanded={expanded}
        onExpandedChange={onExpandedChange}
      />
    );
  }
  if (!basemap) {
    return null;
  }

  return (
    <CartographyProvider dark={basemap.dark}>
      <MapWidget
        styleUrl={basemap.styleUrl}
        ariaLabel="Recorded track"
        furniture={
          <MapControls>
            <Button
              variant="panel"
              icon={
                expanded ? <IconArrowsMinimize stroke={1.6} /> : <IconArrowsMaximize stroke={1.6} />
              }
              onClick={() => onExpandedChange(!expanded)}
              aria-label={expanded ? "Collapse map" : "Expand map"}
              title={expanded ? "Collapse map" : "Expand map"}
            />
          </MapControls>
        }
      >
        <MapViewport
          bounds={windowBounds ?? bounds}
          maxZoom={windowBounds ? WINDOW_MAX_ZOOM : TRACK_MAX_ZOOM}
          fitRevision={expanded ? 1 : 0}
        />
        <RouteOverlay
          coordinates={coordinates}
          profile={profile}
          activeProfile={activeProfile}
          activeMetres={activeMetres}
          onActiveChange={onActiveChange}
          zoomWindow={zoomWindow}
          onZoomChange={onZoomChange}
        />
      </MapWidget>
    </CartographyProvider>
  );
}
