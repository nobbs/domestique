/**
 * The route's elevation profile: elevation against distance travelled.
 *
 * One series, so there is no legend — the heading names what is plotted. The
 * line carries the only colour on the chart; ticks, labels, and the readout use
 * text tokens, and the grid is a hairline a step off the surface so the terrain
 * is the loudest thing in the frame.
 */

import { useCallback, useMemo, useState } from "react";
import type { Position } from "../api/types";
import { buildProfile, ticksFor } from "../lib/profile";
import { useElementWidth } from "../lib/useElementWidth";

const HEIGHT = 148;
const PADDING = { top: 12, right: 12, bottom: 22, left: 46 };
const MIN_WIDTH = 240;

export interface ElevationProfileProps {
  coordinates: Position[];
  title: string;
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

    const points = profile.samples.map((sample) => ({
      x: x(sample.distanceMetres),
      y: y(sample.elevationMetres),
    }));
    const line = points
      .map(
        (point, index) => `${index === 0 ? "M" : "L"}${point.x.toFixed(1)},${point.y.toFixed(1)}`,
      )
      .join(" ");

    return {
      x,
      y,
      points,
      line,
      area: `${line} L${plotWidth.toFixed(1)},${plotHeight} L0,${plotHeight} Z`,
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

          <path className="elevation-profile__area" d={geometry.area} />
          <path className="elevation-profile__line" d={geometry.line} />

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
          height: plotHeight,
        }}
        role="slider"
        tabIndex={0}
        aria-label={`Position along ${title}`}
        aria-valuemin={0}
        aria-valuemax={Number((profile.totalDistanceMetres / 1000).toFixed(1))}
        aria-valuenow={active ? Number((active.distanceMetres / 1000).toFixed(1)) : 0}
        aria-valuetext={
          active
            ? `${Math.round(active.elevationMetres)} metres at ${(active.distanceMetres / 1000).toFixed(1)} kilometres`
            : "No position selected"
        }
        onKeyDown={onKeyDown}
        onPointerMove={onPointerMove}
        onPointerLeave={() => setHovered(null)}
        onBlur={() => setHovered(null)}
      />

      <p className="elevation-profile__readout" aria-live="polite">
        {active ? (
          <>
            <strong>{Math.round(active.elevationMetres)} m</strong>
            <span> at {(active.distanceMetres / 1000).toFixed(1)} km</span>
          </>
        ) : (
          <span>
            {Math.round(profile.minElevationMetres)}–{Math.round(profile.maxElevationMetres)} m ·
            metres above sea level against kilometres ridden
          </span>
        )}
      </p>
    </div>
  );
}
