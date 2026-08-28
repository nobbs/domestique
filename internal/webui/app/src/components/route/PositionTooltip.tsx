import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { Marker, useMap } from "react-map-gl/maplibre";
import { weatherQuery } from "../../api/queries";
import type { Position } from "../../api/types";
import type { ForecastSample } from "../../lib/forecastSamples";
import { formatDistance, formatElevation, formatWindSpeed } from "../../lib/format";
import type { ProfileSample } from "../../lib/profile";
import { bandColour, cumulativeMetres, gradientBand } from "../../lib/profile";
import type { SurfaceSummary } from "../../lib/surface";
import { SURFACE_STYLES, surfaceColour, surfaceKindAt } from "../../lib/surface";
import type { UnitSystem } from "../../lib/units";
import { BEARING_WINDOW_METRES, bearingAt, bearingIsMixed, windRelation } from "../../lib/wind";

/** Between the dot and the nearest edge of the box. */
const GAP_PIXELS = 10;
/** How close to the pane's edge the box may sit. */
const EDGE_PIXELS = 8;
/** How far in from the box's own corners the arrow is allowed, so it never sits on a curve. */
const ARROW_INSET_PIXELS = 14;

/** A guess at the box before its first measurement, near enough to place it once. */
const DEFAULT_SIZE = { width: 232, height: 46 };

/**
 * The application's own tones, on the ground they are drawn on.
 *
 * Not `var(--good)` and friends: those follow the page, and by the time these
 * are drawn the box has already inverted against the cartography, so its
 * interior is light exactly where the map is dark.
 */
const TONE = {
  good: { light: "#317a45", dark: "#82c38f" },
  alert: { light: "#b63132", dark: "#f2857d" },
  hold: { light: "#8f6100", dark: "#e5b562" },
} as const;

const GROUND = { light: "#1c2126", dark: "#eef0f3" } as const;
const INK = { light: "#f3f5f6", dark: "#101316" } as const;
const TRACK = { light: "#f3f5f64d", dark: "#1013164d" } as const;

function clamp(value: number, low: number, high: number): number {
  return high < low ? low : Math.min(Math.max(value, low), high);
}

/**
 * `formatDistance`, but honest about a true zero.
 *
 * The shared formatter reads zero as "nothing to report" and prints `—`, which
 * is right for a stage with no climbing and wrong at the finish of a ride,
 * where zero metres to go is exactly the fact this line exists to state.
 */
function tooltipDistance(metres: number, system: UnitSystem): string {
  return metres > 0 ? formatDistance(metres, system) : system === "imperial" ? "0 ft" : "0 m";
}

interface Wind {
  /** Degrees clockwise from "along the direction of travel". */
  rotation: number;
  colour: string;
  named: string;
  speed: string;
}

/**
 * The wind where the reader is pointing, against the direction of travel.
 *
 * The one weather fact worth the width here: over a few kilometres the
 * temperature does not move, while the relation inverts wherever the route
 * turns back on itself. A stretch that doubles back inside the bearing window
 * has no single relation to report, and says nothing rather than guessing —
 * the same silence `ForecastStrip` keeps for its `mixed` cells.
 */
function windAt(
  coordinates: Position[],
  distances: number[],
  atMetres: number,
  windFromDegrees: number,
  windSpeedKmh: number,
  onDarkGround: boolean,
  unitSystem: UnitSystem,
): Wind | null {
  if (bearingIsMixed(coordinates, distances, atMetres, BEARING_WINDOW_METRES)) {
    return null;
  }
  const bearing = bearingAt(coordinates, distances, atMetres, BEARING_WINDOW_METRES);
  if (bearing === null) {
    return null;
  }
  const reading = windRelation(bearing, windFromDegrees);
  const ground = onDarkGround ? "dark" : "light";
  // A crosswind pushes the rider neither way, so its along-travel component
  // would be a misleading nought; the wind's own speed is what there is to say.
  const speedKmh =
    reading.relation === "cross"
      ? windSpeedKmh
      : Math.abs(reading.componentKmhPerKmh) * windSpeedKmh;

  return {
    // A headwind arrow comes back at the reader; a tailwind runs away.
    rotation: (((windFromDegrees - bearing + 180) % 360) + 360) % 360,
    colour:
      reading.relation === "tail"
        ? TONE.good[ground]
        : reading.relation === "head"
          ? TONE.alert[ground]
          : TONE.hold[ground],
    named:
      reading.relation === "tail"
        ? "Tailwind"
        : reading.relation === "head"
          ? "Headwind"
          : "Crosswind",
    speed: formatWindSpeed(speedKmh, unitSystem),
  };
}

export interface PositionTooltipProps {
  /**
   * Where the box points: always the whole-route sample, the same one the dot
   * is drawn from. A windowed profile interpolates its own coordinates, and on
   * a bend those can differ by enough to leave the box beside the dot it means.
   */
  position: ProfileSample;
  /** What the box says: see `activeProfile` on `RouteOverlay` for which profile this is. */
  content: ProfileSample;
  /** The whole route's length, for the share of it already behind the reader. */
  endMetres: number;
  surfaceSummary: SurfaceSummary | null | undefined;
  /** The route's own coordinates, for the heading the wind is read against. */
  coordinates: Position[];
  /** The forecast requests for this ride, empty until a start time is picked. */
  samples: ForecastSample[];
  announce: boolean;
  /** Whether the ground under this is dark, which is what its colours follow. */
  darkBasemap: boolean;
  unitSystem: UnitSystem;
}

/**
 * The numbers for the position under the pointer, beside the dot itself.
 *
 * The profile readout below the map says the same thing, but it is inside the
 * section a reader can and does fold away to give the map the whole pane — and
 * even open, reading it means looking away from the point being asked about.
 *
 * Folding the profile also unmounts that readout's `aria-live` region, so
 * `announce` says whether this has to carry the announcement itself. Open, it
 * stays `aria-hidden`: the readout already says the same position, and a second
 * live region would say it twice.
 *
 * A child of the map because placement is judged in screen pixels against the
 * live camera, the same reason `HoverLink` is one — and because the box has to
 * be placed again whenever the camera moves the dot, not only when a fresh
 * hover picks a new one. Pointer events are off: the box floats over the very
 * line the pointer is following, and one that caught the mouse would freeze the
 * position it labels the moment the cursor drifted under it.
 */
export function PositionTooltip({
  position,
  content,
  endMetres,
  surfaceSummary,
  coordinates,
  samples,
  announce,
  darkBasemap,
  unitSystem,
}: PositionTooltipProps) {
  const { current: map } = useMap();
  const boxRef = useRef<HTMLDivElement | null>(null);
  const [size, setSize] = useState(DEFAULT_SIZE);
  // Read nowhere: setting it is what has the placement below recomputed against
  // the camera's current position rather than the one as of the last hover.
  const [, bumpOnCameraMove] = useState(0);

  // Shared with `ForecastStrip` by key rather than by prop: the strip has
  // usually asked already, and TanStack answers the second caller from the
  // cache instead of the network.
  const forecast = useQuery({ ...weatherQuery(samples), enabled: samples.length > 0 });

  const distances = useMemo(() => cumulativeMetres(coordinates), [coordinates]);

  useEffect(() => {
    const element = boxRef.current;
    if (!element || typeof ResizeObserver === "undefined") {
      return;
    }
    const observer = new ResizeObserver(() => {
      setSize({ width: element.offsetWidth, height: element.offsetHeight });
    });
    observer.observe(element);

    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (!map) {
      return;
    }
    const onCamera = () => bumpOnCameraMove((tick) => tick + 1);
    map.on("move", onCamera);
    map.on("resize", onCamera);

    return () => {
      map.off("move", onCamera);
      map.off("resize", onCamera);
    };
  }, [map]);

  if (!map) {
    return null;
  }

  const point = map.project([position.longitude, position.latitude]);
  const pane = map.getContainer();

  /*
   * Above the dot wherever there is room for it, and below where there is not.
   * The box is never placed diagonally: a tooltip's arrow sits on the edge that
   * faces what it points at, and a corner has no such edge.
   */
  const above = point.y - GAP_PIXELS - size.height >= EDGE_PIXELS;

  /*
   * The box is centred on the dot until that would hang it over the edge of the
   * pane, and is slid back inside when it would. The arrow then slides the
   * other way by the same amount, so it stays over the dot rather than riding
   * along with the box — which is the whole reason it is free to move at all.
   */
  const half = size.width / 2;
  const centreX = clamp(point.x, EDGE_PIXELS + half, pane.clientWidth - EDGE_PIXELS - half);
  const shiftX = centreX - point.x;
  const arrowLeft = clamp(half - shiftX, ARROW_INSET_PIXELS, size.width - ARROW_INSET_PIXELS);

  const ground = darkBasemap ? "dark" : "light";
  // What is *inside* the box stands on the opposite ground from the map, the
  // box having already inverted against the cartography.
  const onDarkGround = !darkBasemap;
  const inner = onDarkGround ? "dark" : "light";

  const kind = surfaceSummary ? surfaceKindAt(surfaceSummary, content.distanceMetres) : null;
  const grade = bandColour(gradientBand(content.gradientPercent), onDarkGround);
  const travelled = endMetres > 0 ? clamp(content.distanceMetres / endMetres, 0, 1) : 0;

  const nearest = forecast.data?.points?.length
    ? samples.reduce(
        (best, sample, index) =>
          Math.abs(sample.distanceMetres - content.distanceMetres) <
          Math.abs(best.sample.distanceMetres - content.distanceMetres)
            ? { sample, index }
            : best,
        { sample: samples[0] as ForecastSample, index: 0 },
      )
    : null;
  const reading = nearest ? forecast.data?.points?.[nearest.index] : undefined;
  const wind = reading
    ? windAt(
        coordinates,
        distances,
        content.distanceMetres,
        reading.windDirectionDegrees,
        reading.windSpeedKmh,
        onDarkGround,
        unitSystem,
      )
    : null;

  return (
    <Marker
      // `Marker` builds its anchor once and never revisits it, so the whole
      // subtree is rebuilt when the box changes which side it opens from.
      key={above ? "above" : "below"}
      longitude={position.longitude}
      latitude={position.latitude}
      anchor={above ? "bottom" : "top"}
      offset={[shiftX, above ? -GAP_PIXELS : GAP_PIXELS]}
      className="route-position-tooltip-marker"
    >
      <div className="relative">
        {/*
         * shadcn's own arrow — a ten-pixel square turned forty-five degrees with
         * two-pixel corners, in the box's colour. Drawn before the box and so
         * painted beneath it, which is what leaves the progress bar unbroken:
         * only the half past the edge is ever seen.
         */}
        <span
          aria-hidden="true"
          className={`route-position-tooltip-arrow absolute size-2.5 -translate-x-1/2 rotate-45 rounded-[2px] ${
            above ? "bottom-0 translate-y-1/2" : "top-0 -translate-y-1/2"
          }`}
          style={{ left: arrowLeft, background: GROUND[ground] }}
        />
        <div
          ref={boxRef}
          // The class carries no styling any more — the colours have to follow
          // the cartography, which only JavaScript knows — but it stays as the
          // name the suites reach for this box by.
          className="route-position-tooltip relative w-fit overflow-hidden rounded-lg text-xs tabular-nums shadow-[var(--shadow)]"
          style={{ background: GROUND[ground], color: INK[ground] }}
          aria-hidden={announce ? undefined : true}
          aria-live={announce ? "polite" : undefined}
        >
          <div className="grid gap-y-1 px-2.5 py-2">
            <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
              <span className="font-semibold">
                {tooltipDistance(content.distanceMetres, unitSystem)}
              </span>
              <span className="opacity-70">
                {formatElevation(content.elevationMetres, unitSystem)}
              </span>
              <span className="font-medium" style={{ color: grade }}>
                {content.gradientPercent.toFixed(1)}%
              </span>
              {kind ? (
                <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
                  <span
                    aria-hidden="true"
                    className="size-2.5 shrink-0 rounded-[3px]"
                    style={{ backgroundColor: surfaceColour(kind, onDarkGround) }}
                  />
                  {SURFACE_STYLES[kind].label}
                </span>
              ) : null}
            </div>
            {/*
             * A second row only once there is a forecast to put in it, so a
             * ride with no start time keeps the single-row box it had before.
             */}
            {wind ? (
              <div
                className="inline-flex items-center gap-1.5 font-medium whitespace-nowrap"
                style={{ color: wind.colour }}
              >
                <svg
                  viewBox="0 0 10 10"
                  className="size-3 shrink-0"
                  style={{ transform: `rotate(${wind.rotation}deg)` }}
                  aria-hidden="true"
                >
                  <title>{`${wind.named}, ${wind.speed}`}</title>
                  <path d="M5 0.5 L8 8 L5 6.3 L2 8 Z" fill="currentColor" />
                </svg>
                {wind.named} {wind.speed}
              </div>
            ) : null}
          </div>
          <div className="h-[3px] w-full" style={{ backgroundColor: TRACK[inner] }}>
            <div
              className="h-full"
              style={{ width: `${travelled * 100}%`, backgroundColor: grade }}
            />
          </div>
        </div>
      </div>
    </Marker>
  );
}
