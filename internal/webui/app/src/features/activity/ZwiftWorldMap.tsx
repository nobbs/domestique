/**
 * A ride through a virtual world, drawn over that world's own map artwork.
 *
 * Not a map of the ground: a Zwift ride's coordinates are the world's, and
 * Watopia's sit in open ocean. The artwork is served from this service's own
 * origin — Zwift's CDN sends no CORS header — and the line is placed on it by
 * the linear transform in `lib/zwiftWorld`, so there is no basemap, no zoom,
 * and nothing to geolocate against.
 */

import { IconArrowsMaximize, IconArrowsMinimize } from "@tabler/icons-react";
import { useCallback, useMemo, useState } from "react";
import type { ActivityTrackWorld, Position } from "../../api/types";
import { Button } from "../../components/Button";
import type { Profile } from "../../lib/profile";
import { sampleAt } from "../../lib/profile";
import type { ImageSize } from "../../lib/zwiftWorld";
import { worldPoint, worldPolyline } from "../../lib/zwiftWorld";

export interface ZwiftWorldMapProps {
  world: ActivityTrackWorld;
  coordinates: Position[];
  /** Turns a point on the artwork into a distance, and back for the cursor. */
  profile: Profile | null;
  activeMetres: number | null;
  onActiveChange: (metres: number | null) => void;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
}

export function ZwiftWorldMap({
  world,
  coordinates,
  profile,
  activeMetres,
  onActiveChange,
  expanded,
  onExpandedChange,
}: ZwiftWorldMapProps) {
  // The artwork's own pixel size, which the overlay's viewBox and aspect ratio
  // both come from: it is only known once the image has loaded.
  const [size, setSize] = useState<ImageSize | null>(null);
  const line = useMemo(
    () => (size ? worldPolyline(world, size, coordinates) : ""),
    [world, size, coordinates],
  );
  const cursor = useMemo(() => {
    if (!size || !profile || activeMetres === null) {
      return null;
    }
    const sample = sampleAt(profile, activeMetres);

    return sample ? worldPoint(world, size, [sample.longitude, sample.latitude]) : null;
  }, [world, size, profile, activeMetres]);
  // The nearest sample to the pointer, in the artwork's own pixels: the shared
  // cursor is addressed by distance, so a hover has to be turned back into one.
  const onPointerMove = useCallback(
    (event: React.PointerEvent<SVGSVGElement>) => {
      if (!size || !profile) {
        return;
      }
      const box = event.currentTarget.getBoundingClientRect();
      if (box.width === 0 || box.height === 0) {
        return;
      }
      const x = ((event.clientX - box.left) / box.width) * size.width;
      const y = ((event.clientY - box.top) / box.height) * size.height;
      let nearest: number | null = null;
      let nearestDistance = Number.POSITIVE_INFINITY;
      for (const sample of profile.samples) {
        const point = worldPoint(world, size, [sample.longitude, sample.latitude]);
        const distance = (point.x - x) ** 2 + (point.y - y) ** 2;
        if (distance < nearestDistance) {
          nearestDistance = distance;
          nearest = sample.distanceMetres;
        }
      }
      onActiveChange(nearest);
    },
    [world, size, profile, onActiveChange],
  );

  return (
    <div className="relative flex h-full w-full items-center justify-center bg-[var(--ground)]">
      <div
        className="relative max-h-full max-w-full"
        style={size ? { aspectRatio: `${size.width} / ${size.height}` } : undefined}
      >
        <img
          src={world.mapUrl}
          alt={`Map of ${world.name}`}
          className="h-full w-full object-contain"
          onLoad={(event) =>
            setSize({
              width: event.currentTarget.naturalWidth,
              height: event.currentTarget.naturalHeight,
            })
          }
        />
        {size ? (
          <svg
            className="absolute inset-0 h-full w-full"
            viewBox={`0 0 ${size.width} ${size.height}`}
            preserveAspectRatio="none"
            role="img"
            aria-label={`Recorded track in ${world.name}`}
            onPointerMove={onPointerMove}
            onPointerLeave={() => onActiveChange(null)}
          >
            <polyline
              points={line}
              fill="none"
              stroke="var(--accent)"
              strokeWidth={size.width / 200}
              strokeLinejoin="round"
              strokeLinecap="round"
            />
            {cursor ? (
              <circle
                cx={cursor.x}
                cy={cursor.y}
                r={size.width / 90}
                fill="var(--panel)"
                stroke="var(--accent)"
                strokeWidth={size.width / 300}
              />
            ) : null}
          </svg>
        ) : null}
      </div>
      <div className="absolute top-2 right-2">
        <Button
          variant="panel"
          icon={
            expanded ? <IconArrowsMinimize stroke={1.6} /> : <IconArrowsMaximize stroke={1.6} />
          }
          onClick={() => onExpandedChange(!expanded)}
          aria-label={expanded ? "Collapse map" : "Expand map"}
          title={expanded ? "Collapse map" : "Expand map"}
        />
      </div>
    </div>
  );
}
