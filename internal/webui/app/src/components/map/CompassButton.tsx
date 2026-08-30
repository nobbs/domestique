/**
 * The map's orientation, and the way back: the needle tracks the camera's
 * bearing and pitch live, and a press eases the view to north and level.
 * Rotation is already on the map's gestures (right-click drag, two-finger
 * twist), so without this a rotated map has no way home short of
 * counter-dragging.
 */

import { IconNavigationFilled } from "@tabler/icons-react";
import { useEffect, useState } from "react";
import { useMap } from "react-map-gl/maplibre";
import { usePrefersReducedMotion } from "../../lib/mediaQuery";
import { Button } from "../Button";

const RESET_DURATION_MS = 600;

/**
 * Where the arrow is spun and centred, in its own 24-unit viewBox, tuned by
 * eye: the glyph's reach is asymmetric, so the point it visually turns about
 * is well below its box's centre.
 */
const PIVOT = { x: 12, y: 16 };
const NEEDLE_SCALE = 0.7;

/** The camera pose the compass mirrors: tilt flattens it, bearing turns it. */
function poseTransform(bearing: number, pitch: number): string {
  return `rotateX(${pitch}deg) rotateZ(${-bearing}deg)`;
}

/** ViewBox units as a fraction of the box, whatever size the box renders at. */
function fraction(units: number): string {
  return `${(units / 24) * 100}%`;
}

const TICK_ANGLES = Array.from({ length: 12 }, (_, i) => i * 30);

/** The ring the needle turns in. Under pitch it foreshortens as the horizon does. */
function TickRing({ bearing, pitch }: { bearing: number; pitch: number }) {
  return (
    <svg
      viewBox="0 0 24 24"
      className="absolute inset-0 size-full"
      style={{ transform: poseTransform(bearing, pitch) }}
      aria-hidden="true"
    >
      {TICK_ANGLES.map((angle) => {
        const radians = (angle * Math.PI) / 180;
        const [sin, cos] = [Math.sin(radians), Math.cos(radians)];
        const cardinal = angle % 90 === 0;
        const inner = cardinal ? 8.25 : 9.5;

        return (
          <line
            key={angle}
            x1={12 + inner * sin}
            y1={12 - inner * cos}
            x2={12 + 11 * sin}
            y2={12 - 11 * cos}
            stroke={cardinal ? "var(--ink)" : "var(--ink-2)"}
            strokeWidth={cardinal ? 2 : 1.4}
            strokeLinecap="round"
          />
        );
      })}
    </svg>
  );
}

/** A red needle turning inside a ring of ticks, mirroring the camera. */
export function CompassIcon({ bearing, pitch = 0 }: { bearing: number; pitch?: number }) {
  return (
    <span className="relative inline-block size-6">
      <TickRing bearing={bearing} pitch={pitch} />
      <IconNavigationFilled
        color="var(--alert)"
        className="absolute inset-0 size-full"
        style={{
          // Right to left: shrink and spin about the pivot, then carry the
          // pivot to the box's centre — the icon is centred by its pivot,
          // not by its bounding box.
          transform: `translate(${fraction(12 - PIVOT.x)}, ${fraction(12 - PIVOT.y)}) ${poseTransform(bearing, pitch)} scale(${NEEDLE_SCALE})`,
          transformOrigin: `${fraction(PIVOT.x)} ${fraction(PIVOT.y)}`,
        }}
      />
    </span>
  );
}

export function CompassButton() {
  const { current: map } = useMap();
  const reducedMotion = usePrefersReducedMotion();
  const [pose, setPose] = useState({ bearing: 0, pitch: 0 });

  useEffect(() => {
    if (!map) {
      return;
    }
    const read = () => setPose({ bearing: map.getBearing(), pitch: map.getPitch() });
    read();
    map.on("rotate", read);
    map.on("pitch", read);

    return () => {
      map.off("rotate", read);
      map.off("pitch", read);
    };
  }, [map]);

  return (
    <Button
      variant="panel"
      icon={<CompassIcon bearing={pose.bearing} pitch={pose.pitch} />}
      // Pitch comes along: the same drag that rotates also tilts, and a reader
      // asking for north up is asking for the map they started with.
      onClick={() =>
        map?.easeTo({ bearing: 0, pitch: 0, duration: reducedMotion ? 0 : RESET_DURATION_MS })
      }
      disabled={!map}
      aria-label="Reset the view to north"
      title="Reset the view to north"
    />
  );
}
