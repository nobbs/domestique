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

import { useCallback, useMemo, useState } from "react";
import type { Position } from "../api/types";
import type { ProfileSample } from "../lib/profile";
import { buildProfile, GRADIENT_BANDS, ticksFor } from "../lib/profile";
import { useElementWidth } from "../lib/useElementWidth";

const HEIGHT = 148;
const PADDING = { top: 12, right: 12, bottom: 22, left: 46 };
const MIN_WIDTH = 240;

export interface ElevationProfileProps {
  coordinates: Position[];
  title: string;
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
function steadyBands(samples: ProfileSample[]): number[] {
  const bands = samples.map((sample) => sample.band);

  let start = 0;
  for (let index = 1; index <= bands.length; index++) {
    if (index < bands.length && bands[index] === bands[start]) {
      continue;
    }
    if (index - start < MIN_RUN_SAMPLES && start > 0) {
      const previous = bands[start - 1] ?? 0;
      for (let fill = start; fill < index; fill++) {
        bands[fill] = previous;
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

export function ElevationProfile({ coordinates, title }: ElevationProfileProps) {
  const { ref, width } = useElementWidth<HTMLDivElement>();
  const [hovered, setHovered] = useState<number | null>(null);

  const profile = useMemo(() => buildProfile(coordinates), [coordinates]);

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
      if (!profile) {
        return;
      }
      const bounds = event.currentTarget.getBoundingClientRect();
      const ratio = (event.clientX - bounds.left) / bounds.width;
      const index = Math.round(ratio * (profile.samples.length - 1));
      setHovered(Math.min(Math.max(index, 0), profile.samples.length - 1));
    },
    [profile],
  );

  const onKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLDivElement>) => {
      if (!profile) {
        return;
      }
      const step = event.key === "ArrowRight" ? 1 : event.key === "ArrowLeft" ? -1 : 0;
      if (step === 0) {
        return;
      }
      event.preventDefault();
      setHovered((current) => {
        const next = (current ?? 0) + step * 4;

        return Math.min(Math.max(next, 0), profile.samples.length - 1);
      });
    },
    [profile],
  );

  if (!profile || !geometry) {
    return (
      <p className="elevation-profile__absent">
        This route has no elevation data, so it has no profile to show.
      </p>
    );
  }

  const active = hovered === null ? null : profile.samples[hovered];
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

          {geometry.distanceTicks.map((kilometres) => (
            <text
              key={kilometres}
              className="elevation-profile__tick"
              x={geometry.x(kilometres * 1000)}
              y={plotHeight + 15}
              textAnchor="middle"
            >
              {kilometres}
            </text>
          ))}

          {active ? (
            <g className="elevation-profile__cursor">
              <line
                x1={geometry.x(active.distanceMetres)}
                x2={geometry.x(active.distanceMetres)}
                y1={0}
                y2={plotHeight}
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
        style={{ left: PADDING.left, top: PADDING.top, width: plotWidth, height: plotHeight }}
        role="slider"
        tabIndex={0}
        aria-label={`Position along ${title}`}
        aria-valuemin={0}
        aria-valuemax={Number((profile.totalDistanceMetres / 1000).toFixed(1))}
        aria-valuenow={active ? Number((active.distanceMetres / 1000).toFixed(1)) : 0}
        aria-valuetext={
          active
            ? `${Math.round(active.elevationMetres)} metres at ${(active.distanceMetres / 1000).toFixed(1)} kilometres, ${active.gradientPercent.toFixed(1)} percent`
            : "No position selected"
        }
        onKeyDown={onKeyDown}
        onPointerMove={onPointerMove}
        onPointerLeave={() => setHovered(null)}
        onBlur={() => setHovered(null)}
      />

      <div className="elevation-profile__footer">
        <p className="elevation-profile__readout" aria-live="polite">
          {active ? (
            <>
              <strong>{Math.round(active.elevationMetres)} m</strong>
              <span> at {(active.distanceMetres / 1000).toFixed(1)} km</span>
              <span> · {active.gradientPercent.toFixed(1)}%</span>
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
              <span className="elevation-profile__swatch" data-band={index} aria-hidden="true" />
              {band.label}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
