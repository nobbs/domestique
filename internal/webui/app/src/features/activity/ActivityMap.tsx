/**
 * One ride's recorded track, on a canvas of its own: the library map belongs to
 * the entry page, and a ride is not part of the library.
 */

import { IconArrowsMaximize, IconArrowsMinimize } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { Layer, Source } from "react-map-gl/maplibre";
import { webUIConfigQuery } from "../../api/queries";
import type { ActivityTrackWorld, BoundingBox, Position } from "../../api/types";
import { Button } from "../../components/Button";
import { CartographyProvider } from "../../components/map/CartographyContext";
import { MapControls } from "../../components/map/MapControls";
import { MapViewport } from "../../components/map/MapViewport";
import type { MapStyle } from "../../components/map/MapWidget";
import { MapWidget } from "../../components/map/MapWidget";
import { basemapFor, useBasemapChoice, usePrefersDarkScheme } from "../../lib/basemap";
import { WINDOW_MAX_ZOOM } from "../../lib/cartography";
import type { DistanceWindow, Profile } from "../../lib/profile";
import { resolvesDark, useThemeChoice } from "../../lib/theme";
import { RouteOverlay } from "../routes/RouteOverlay";

/** As close as a whole ride is framed, so a short loop is not zoomed to the tarmac. */
const TRACK_MAX_ZOOM = 15;

/**
 * No vector basemap: `.route-map` already paints `var(--ground)` behind the
 * canvas, and a world ride draws over its own artwork instead of cartography.
 * Inline rather than a `data:` URL: MapLibre fetches a style URL, and the
 * Content-Security-Policy's `connect-src` admits this service's own origin
 * and each configured basemap's, never `data:`. One fixed object for every
 * world, so it never triggers `MapWidget`'s remount-on-style-change.
 */
const BLANK_STYLE: MapStyle = { version: 8, sources: {}, layers: [] };

const WORLD_ARTWORK_SOURCE_ID = "zwift-world-artwork";

/** The corners of an image source, in the order it wants them. */
type Corner = [number, number];
type ImageCorners = [Corner, Corner, Corner, Corner];

/**
 * Where a world's artwork's four corners go: top-left, top-right, bottom-right,
 * bottom-left of the *image*. Each quarter turn clockwise shifts that list by
 * one rather than touching a pixel — MapLibre draws whatever quadrilateral the
 * corners describe.
 */
function artworkCorners(world: ActivityTrackWorld): ImageCorners {
  const { north, west, south, east } = world.bounds;
  let corners: ImageCorners = [
    [west, north],
    [east, north],
    [east, south],
    [west, south],
  ];
  for (let turn = 0; turn < world.imageQuarterTurns; turn += 1) {
    const [first, ...rest] = corners;
    corners = [...rest, first];
  }

  return corners;
}

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
  // A world ride draws over its own artwork and needs no basemap, so no
  // config read.
  const config = useQuery({ ...webUIConfigQuery(), enabled: !world });
  const [themeChoice] = useThemeChoice();
  const [basemapChoice] = useBasemapChoice();
  const prefersDark = usePrefersDarkScheme();
  const basemap = config.data
    ? basemapFor(config.data, resolvesDark(themeChoice, prefersDark), basemapChoice)
    : null;

  // The route accent picks its light-basemap colours against a world's own
  // artwork unconditionally: that art is a bright illustration regardless of
  // the reader's own theme, unlike a real basemap that actually has a dark
  // style.
  const cartography = world
    ? { dark: false, styleUrl: BLANK_STYLE, ariaLabel: `Recorded track in ${world.name}` }
    : basemap
      ? { dark: basemap.dark, styleUrl: basemap.styleUrl, ariaLabel: "Recorded track" }
      : null;
  if (!cartography) {
    return null;
  }

  return (
    <CartographyProvider dark={cartography.dark}>
      <MapWidget
        styleUrl={cartography.styleUrl}
        ariaLabel={cartography.ariaLabel}
        furniture={
          <MapControls hideLocate={world !== null}>
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
          // A world ride is framed to its own recorded track, exactly as an
          // outdoor one is: the world's bounds below place the artwork, but
          // are the whole island, not the ride, and would zoom the camera
          // out to it every time regardless of how short the ride was.
          bounds={windowBounds ?? bounds}
          maxZoom={windowBounds ? WINDOW_MAX_ZOOM : TRACK_MAX_ZOOM}
          fitRevision={expanded ? 1 : 0}
        />
        {world ? (
          <Source
            id={WORLD_ARTWORK_SOURCE_ID}
            type="image"
            url={world.mapUrl}
            coordinates={artworkCorners(world)}
          >
            <Layer id={`${WORLD_ARTWORK_SOURCE_ID}-layer`} type="raster" />
          </Source>
        ) : null}
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
