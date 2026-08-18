/**
 * The route's elevation profile: elevation against distance travelled, drawn as
 * a cross-section whose ground is banded by how steep it is.
 *
 * Steepness is an ordered measure, so it wears an ordinal ramp — one hue, light
 * to dark — with a scale legend. The bands fill the ground rather than tinting
 * the outline, which is what makes a steep kilometre read as a block of terrain
 * at a glance. The ramp is deliberately muted so a chart of solid columns still
 * sits quietly under the map.
 */

import { useCallback, useMemo } from "react";
import type { Profile, ProfileSample } from "../lib/profile";
import { GRADIENT_BANDS, sampleAt, ticksFor } from "../lib/profile";
import type { SurfaceSummary } from "../lib/surface";
import { SURFACE_STYLES, surfaceKindAt } from "../lib/surface";
import { useElementWidth } from "../lib/useElementWidth";

/**
 * Hatch patterns for the steeper bands.
 *
 * Texture is the backup channel for identity: it survives colour blindness,
 * greyscale print, and forced-colours mode, where hue alone does not. It also
 * lets the ground sit at low opacity — the terrain is a backdrop for the
 * silhouette, not a block of paint — while the steep sections still read.
 */
const HATCH_ANGLES: Record<number, number> = { 1: 45, 2: 135 };
// Fine enough that a short pitch still shows several strokes; a coarse hatch
// in a twelve-pixel band is indistinguishable from a solid block.
const HATCH_SIZE = 6;

const HEIGHT = 159;
const PADDING = { top: 12, right: 12, bottom: 33, left: 46 };
const MIN_WIDTH = 240;

/**
 * The surface lane at the foot of the plot.
 *
 * It is thin on purpose: it carries one categorical measure along an axis that
 * already belongs to the terrain above it, and a thick band would read as a
 * second chart competing with the first. The gap keeps it from touching the
 * ground of the profile, which would make the two look like one shape.
 */
const SURFACE_STRIP_HEIGHT = 7;
const SURFACE_STRIP_GAP = 4;

export interface ElevationProfileProps {
  profile: Profile | null;
  title: string;
  /**
   * The ground under the route: drawn as a strip along the foot of the chart,
   * and reported for the hovered position.
   *
   * The chart's own paint is not touched by it. The bands mean gradient, and a
   * second measure fighting for the same ground would make both unreadable — so
   * the surface gets a lane of its own directly beneath the terrain, sharing the
   * distance axis. Reading upwards from any point on the strip lands on the
   * climb that is made of that surface, which is the question the two answer
   * together and neither answers alone.
   */
  surface?: SurfaceSummary | null;
  /**
   * The position shared with the map, in metres from the start of the route.
   *
   * Ground rather than a sample index, because an index only means a place to
   * whoever holds that exact array of samples — and the map holds none.
   */
  activeMetres: number | null;
  onActiveChange: (metres: number | null) => void;
}

interface Run {
  band: number;
  column: string;
}

/** A run shorter than this is absorbed into its neighbour. */
const MIN_RUN_SAMPLES = 3;

/**
 * The band of each sample, with momentary flicker removed.
 *
 * Where a gradient hovers on a threshold it crosses back and forth every few
 * metres. Drawn literally that produces a barcode of alternating colour that
 * says nothing about the terrain — the bands are meant to show *sustained*
 * steepness, so a run too short to be sustained takes its neighbour's band.
 */
export function steadyBands(samples: ProfileSample[]): number[] {
  const bands = samples.map((sample) => sample.band);

  let start = 0;
  for (let index = 1; index <= bands.length; index++) {
    if (index < bands.length && bands[index] === bands[start]) {
      continue;
    }
    if (index - start < MIN_RUN_SAMPLES) {
      // A short run takes the preceding band where there is one, and otherwise
      // the following one, so a brief opening run is smoothed like any other.
      // A profile that is a single run has no neighbour and stands as it is.
      const neighbour = start > 0 ? bands[start - 1] : bands[index];
      if (neighbour !== undefined) {
        for (let fill = start; fill < index; fill++) {
          bands[fill] = neighbour;
        }
      }
    }
    start = index;
  }

  return bands;
}

/**
 * Splits the ground under the profile into columns of one gradient band each.
 *
 * A column follows the terrain along its top and drops to the baseline, so the
 * chart reads as a cross-section of the ride: the steep parts are visibly
 * darker blocks of ground rather than a recoloured hairline.
 *
 * Runs share their boundary sample, so neighbouring columns meet exactly —
 * neither overlapping into a darker seam nor leaving a sliver of surface.
 */
function runsOf(
  samples: ProfileSample[],
  x: (metres: number) => number,
  y: (metres: number) => number,
  plotHeight: number,
): Run[] {
  const bands = steadyBands(samples);
  const runs: Run[] = [];
  let start = 0;

  for (let index = 1; index <= samples.length; index++) {
    if (index < samples.length && bands[index] === bands[start]) {
      continue;
    }
    const slice = samples.slice(start, Math.min(index + 1, samples.length));
    const first = slice[0];
    const last = slice[slice.length - 1];
    if (slice.length >= 2 && first && last) {
      const ridge = slice
        .map(
          (sample, offset) =>
            `${offset === 0 ? "M" : "L"}${x(sample.distanceMetres).toFixed(1)},${y(sample.elevationMetres).toFixed(1)}`,
        )
        .join(" ");

      runs.push({
        band: bands[start] ?? 0,
        column: `${ridge} L${x(last.distanceMetres).toFixed(1)},${plotHeight.toFixed(1)} L${x(first.distanceMetres).toFixed(1)},${plotHeight.toFixed(1)} Z`,
      });
    }
    start = index;
  }

  return runs;
}

export function ElevationProfile({
  profile,
  title,
  surface = null,
  activeMetres,
  onActiveChange,
}: ElevationProfileProps) {
  const { ref, width } = useElementWidth<HTMLDivElement>();

  const plotWidth = Math.max(width, MIN_WIDTH) - PADDING.left - PADDING.right;
  const plotHeight = HEIGHT - PADDING.top - PADDING.bottom;

  const geometry = useMemo(() => {
    if (!profile || plotWidth <= 0) {
      return null;
    }
    // A flat route still needs a band to draw in, so give it one.
    const span = Math.max(profile.maxElevationMetres - profile.minElevationMetres, 10);
    const low = profile.minElevationMetres;

    const x = (metres: number) => (metres / profile.totalDistanceMetres) * plotWidth;
    const y = (metres: number) => plotHeight - ((metres - low) / span) * plotHeight;

    const ridge = profile.samples
      .map(
        (sample, index) =>
          `${index === 0 ? "M" : "L"}${x(sample.distanceMetres).toFixed(1)},${y(sample.elevationMetres).toFixed(1)}`,
      )
      .join(" ");

    return {
      x,
      y,
      ridge,
      runs: runsOf(profile.samples, x, y, plotHeight),
      elevationTicks: ticksFor(low, low + span, 3),
      distanceTicks: ticksFor(0, profile.totalDistanceMetres / 1000, 5),
    };
  }, [profile, plotWidth, plotHeight]);

  const onPointerMove = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      const bounds = event.currentTarget.getBoundingClientRect();
      // A box of no width is an element before it has been laid out. There is no
      // position to read off it, and dividing by it would report NaN as a place
      // on the route.
      if (!profile || bounds.width <= 0) {
        return;
      }
      const ratio = (event.clientX - bounds.left) / bounds.width;
      const metres = ratio * profile.totalDistanceMetres;
      onActiveChange(Math.min(Math.max(metres, 0), profile.totalDistanceMetres));
    },
    [profile, onActiveChange],
  );

  const onKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLDivElement>) => {
      if (!profile) {
        return;
      }
      const direction = event.key === "ArrowRight" ? 1 : event.key === "ArrowLeft" ? -1 : 0;
      if (direction === 0) {
        return;
      }
      event.preventDefault();
      // Four samples' worth of ground, so a keyboard covers the chart in about
      // the same number of presses it always did.
      const step = (profile.totalDistanceMetres / Math.max(profile.samples.length - 1, 1)) * 4;
      const next = (activeMetres ?? 0) + direction * step;
      onActiveChange(Math.min(Math.max(next, 0), profile.totalDistanceMetres));
    },
    [profile, activeMetres, onActiveChange],
  );

  if (!profile || !geometry) {
    return (
      <p className="elevation-profile__absent">
        This route has no elevation data, so it has no profile to show.
      </p>
    );
  }

  const active = activeMetres === null ? null : sampleAt(profile, activeMetres);
  const activeKind = active && surface ? surfaceKindAt(surface, active.distanceMetres) : null;
  const activeSurface = activeKind ? SURFACE_STYLES[activeKind].label : null;
  const summary =
    `Elevation profile of ${title}: ` +
    `${(profile.totalDistanceMetres / 1000).toFixed(1)} kilometres, ` +
    `between ${Math.round(profile.minElevationMetres)} and ` +
    `${Math.round(profile.maxElevationMetres)} metres above sea level.`;

  return (
    <div className="elevation-profile" ref={ref}>
      <svg
        width="100%"
        height={HEIGHT}
        viewBox={`0 0 ${plotWidth + PADDING.left + PADDING.right} ${HEIGHT}`}
        role="img"
        aria-label={summary}
      >
        <title>{summary}</title>
        <defs>
          {Object.entries(HATCH_ANGLES).map(([band, angle]) => (
            <pattern
              key={band}
              id={`elevation-hatch-${band}`}
              width={HATCH_SIZE}
              height={HATCH_SIZE}
              patternUnits="userSpaceOnUse"
              patternTransform={`rotate(${angle})`}
            >
              <rect
                className="elevation-profile__hatch-ground"
                data-band={band}
                width={HATCH_SIZE}
                height={HATCH_SIZE}
              />
              {/*
               * Centred in the tile, not on its edge: a stroke on the edge is
               * clipped in half, so half the intended width simply vanishes.
               */}
              <line
                className="elevation-profile__hatch-line"
                data-band={band}
                x1={HATCH_SIZE / 2}
                y1={0}
                x2={HATCH_SIZE / 2}
                y2={HATCH_SIZE}
              />
            </pattern>
          ))}
        </defs>
        <g transform={`translate(${PADDING.left} ${PADDING.top})`}>
          {geometry.elevationTicks.map((metres) => (
            <g key={metres}>
              <line
                className="elevation-profile__grid"
                x1={0}
                x2={plotWidth}
                y1={geometry.y(metres)}
                y2={geometry.y(metres)}
              />
              <text
                className="elevation-profile__tick"
                x={-8}
                y={geometry.y(metres)}
                textAnchor="end"
                dominantBaseline="middle"
              >
                {Math.round(metres)}
              </text>
            </g>
          ))}

          {geometry.runs.map((run, index) => (
            <path
              // Runs are positional slices of one profile; there is no id to key on.
              // biome-ignore lint/suspicious/noArrayIndexKey: positional by nature
              key={`column-${index}`}
              className="elevation-profile__column"
              data-band={run.band}
              d={run.column}
            />
          ))}
          {/* One ridge over the columns, so the silhouette stays crisp. */}
          <path className="elevation-profile__ridge" d={geometry.ridge} />

          {/*
           * The ground the route is made of, in the order it is ridden, on the
           * distance axis the terrain above already uses. Each stretch is drawn
           * with the colour and the dash pattern it wears on the map, at this
           * strip's own width, so the same stretch reads the same way in both
           * places.
           */}
          {surface
            ? surface.bands.map((band) => {
                const style = SURFACE_STYLES[band.kind];
                const start = geometry.x(band.startMetres);
                const end = geometry.x(band.endMetres);

                return (
                  <line
                    key={`${band.kind}-${band.startMetres}`}
                    className="elevation-profile__surface"
                    x1={start}
                    x2={end}
                    y1={plotHeight + SURFACE_STRIP_GAP + SURFACE_STRIP_HEIGHT / 2}
                    y2={plotHeight + SURFACE_STRIP_GAP + SURFACE_STRIP_HEIGHT / 2}
                    stroke={style.colour}
                    strokeWidth={SURFACE_STRIP_HEIGHT}
                    {...(style.dashes.length > 0
                      ? {
                          strokeDasharray: style.dashes
                            .map((dash) => dash * SURFACE_STRIP_HEIGHT)
                            .join(" "),
                        }
                      : {})}
                  />
                );
              })
            : null}

          {geometry.distanceTicks.map((kilometres) => (
            <text
              key={kilometres}
              className="elevation-profile__tick"
              x={geometry.x(kilometres * 1000)}
              y={plotHeight + SURFACE_STRIP_GAP + SURFACE_STRIP_HEIGHT + 15}
              textAnchor="middle"
            >
              {kilometres}
            </text>
          ))}

          {active ? (
            <g className="elevation-profile__cursor">
              {/*
               * The cursor runs through the surface strip as well: the point on
               * the climb and the ground under it are one position, and a line
               * stopping at the axis would make them look like two.
               */}
              <line
                x1={geometry.x(active.distanceMetres)}
                x2={geometry.x(active.distanceMetres)}
                y1={0}
                y2={surface ? plotHeight + SURFACE_STRIP_GAP + SURFACE_STRIP_HEIGHT : plotHeight}
              />
              <circle
                cx={geometry.x(active.distanceMetres)}
                cy={geometry.y(active.elevationMetres)}
                r={4}
                data-band={active.band}
              />
            </g>
          ) : null}
        </g>
      </svg>

      {/*
       * The scrubbing surface is a slider rather than a decorated graphic: it
       * genuinely picks a position along the route, so that role gives keyboard
       * users arrow-key stepping and screen readers the value at each step,
       * which a non-interactive <svg> cannot carry.
       */}
      <div
        className="elevation-profile__scrub"
        style={{
          left: PADDING.left,
          top: PADDING.top,
          width: plotWidth,
          // Down to the foot of the surface strip when there is one. The cursor
          // crosses both lanes because they describe one position, so a pointer
          // that wandered onto the strip must not fall off the instrument and
          // blank the readout it came to check.
          height: surface ? plotHeight + SURFACE_STRIP_GAP + SURFACE_STRIP_HEIGHT : plotHeight,
        }}
        role="slider"
        tabIndex={0}
        aria-label={`Position along ${title}`}
        aria-valuemin={0}
        aria-valuemax={Number((profile.totalDistanceMetres / 1000).toFixed(1))}
        aria-valuenow={active ? Number((active.distanceMetres / 1000).toFixed(1)) : 0}
        aria-valuetext={
          active
            ? `${Math.round(active.elevationMetres)} metres at ${(active.distanceMetres / 1000).toFixed(1)} kilometres, ${active.gradientPercent.toFixed(1)} percent` +
              (activeSurface ? `, ${activeSurface.toLowerCase()}` : "")
            : "No position selected"
        }
        onKeyDown={onKeyDown}
        onPointerMove={onPointerMove}
        onPointerLeave={() => onActiveChange(null)}
        onBlur={() => onActiveChange(null)}
      />

      <div className="elevation-profile__footer">
        <p className="elevation-profile__readout" aria-live="polite">
          {active ? (
            <>
              <strong>{Math.round(active.elevationMetres)} m</strong>
              <span> at {(active.distanceMetres / 1000).toFixed(1)} km</span>
              <span> · {active.gradientPercent.toFixed(1)}%</span>
              {activeSurface ? <span> · {activeSurface}</span> : null}
            </>
          ) : (
            <span>
              {Math.round(profile.minElevationMetres)}–{Math.round(profile.maxElevationMetres)} m
              above sea level
            </span>
          )}
        </p>

        <ul className="elevation-profile__scale">
          {GRADIENT_BANDS.map((band, index) => (
            <li key={band.label}>
              <span
                className="elevation-profile__swatch"
                data-band={index}
                data-hatched={HATCH_ANGLES[index] !== undefined}
                aria-hidden="true"
              />
              {band.label}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
