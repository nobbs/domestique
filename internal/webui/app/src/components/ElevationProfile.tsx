/**
 * The route's elevation profile: elevation against distance travelled, drawn as
 * a cross-section whose ground is banded by how steep it is.
 *
 * Steepness is an ordered measure, so it wears an ordinal ramp — one hue, light
 * to dark — with a scale legend. The bands fill the ground rather than tinting
 * the outline, which is what makes a steep kilometre read as a block of terrain
 * at a glance. The ramp is deliberately muted so a chart of solid columns still
 * sits quietly under the map.
 *
 * A drag across the plot picks a stretch of the route and the chart redraws it
 * across the full width. On a ninety kilometre stage a single climb is a few
 * millimetres of ink, and the question a rider actually has is about one climb.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Highlight } from "../lib/highlight";
import { gapsOutside, highlightLabel } from "../lib/highlight";
import type { DistanceWindow, Profile, ProfileSample } from "../lib/profile";
import { niceStep, sampleAt, steadyBands, ticksFor } from "../lib/profile";
import type { SurfaceSummary } from "../lib/surface";
import { SURFACE_STYLES, surfaceBandsWithin, surfaceKindAt } from "../lib/surface";
import { useElementWidth } from "../lib/useElementWidth";

/**
 * Hatch patterns for the steeper bands.
 *
 * With five classes, texture stops being the backup channel for identity and
 * becomes one that carries: the ramp cannot separate five steps by hue alone, so
 * every band above the gentlest wears a pattern of its own — one diagonal, then
 * the other, then the two crossed, then a tighter grid. Angle is what separates
 * the neighbours a colour-blind reader finds closest, and density is what is
 * left when the angles run out.
 *
 * It also survives greyscale print and forced-colours mode, where hue alone does
 * not, and it lets the ground sit at low opacity — the terrain is a backdrop for
 * the silhouette, not a block of paint — while the steep sections still read.
 *
 * The tiles are fine enough that a short pitch still shows several strokes; a
 * coarse hatch in a twelve-pixel column is indistinguishable from a solid block.
 */
const HATCHES: Record<number, { angle: number; crossed: boolean; size: number }> = {
  1: { angle: 45, crossed: false, size: 6 },
  2: { angle: 135, crossed: false, size: 6 },
  3: { angle: 45, crossed: true, size: 6 },
  4: { angle: 0, crossed: true, size: 4 },
};

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

/**
 * How far the pointer must travel before a scrub becomes a selection.
 *
 * A hand resting on a trackpad moves a pixel or two and a finger never lands
 * still; treating that as a range would zoom the chart every time somebody
 * looked at it. Eight pixels is past the tremble and well under the shortest
 * swipe anybody makes on purpose.
 */
const MIN_DRAG_PIXELS = 8;

/**
 * The shortest stretch a drag may settle on.
 *
 * Gradient is measured over a hundred metres, so a window much shorter than a
 * couple of those is one measurement drawn three hundred times. A selection
 * under it is grown about its middle rather than refused: the reader asked to
 * look closer at somewhere, and the answer to "closer than the data goes" is
 * the closest the data goes, not nothing.
 */
const MIN_WINDOW_METRES = 200;

export interface ElevationProfileProps {
  /** Already restricted to the stretch on show, when there is a zoom. */
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
   *
   * This is the whole stage's classification, not the window's: the readout asks
   * it about an absolute distance. Only the strip clips.
   */
  surface?: SurfaceSummary | null;
  /** The position shared with the map, in metres from the start of the route. */
  activeMetres: number | null;
  onActiveChange: (metres: number | null) => void;
  /**
   * The stretch on show, or null for the whole route.
   *
   * The chart does not own this. It is the same stretch the map dims around,
   * and a window kept in here would be thrown away every time the overview was
   * collapsed — which is the moment somebody wants the map to itself and the
   * highlight to stay.
   */
  zoomWindow?: DistanceWindow | null;
  /** Absent leaves the chart scrubbing by pointer and keyboard, and no zoom. */
  onZoomChange?: ((window: DistanceWindow | null) => void) | undefined;
  /**
   * The one class picked out of the key, or null for a chart with nothing
   * singled out.
   *
   * The chart does not own this either: the same selection lights the same
   * ground on the map, and the key that sets it sits outside the chart so both
   * legends can share a line.
   */
  highlight?: Highlight | null;
}

/**
 * One stretch of ground of a single band: the shape to fill, and where along the
 * route it sits, which is what lets the same run be lit or veiled by name.
 */
interface Run {
  band: number;
  column: string;
  startMetres: number;
  endMetres: number;
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
        startMetres: first.distanceMetres,
        endMetres: last.distanceMetres,
      });
    }
    start = index;
  }

  return runs;
}

/**
 * A distance in kilometres, with just enough decimals to tell it from its
 * neighbour. Whole kilometres are right for a whole route and useless for a
 * four hundred metre window, where every label would read the same number.
 */
function kilometreLabel(metres: number, stepKilometres: number): string {
  const decimals = Math.min(Math.max(Math.ceil(-Math.log10(stepKilometres)), 0), 3);

  return (metres / 1000).toFixed(decimals);
}

/**
 * A selection too short to plot is grown about its middle rather than refused,
 * and slid back inside the route rather than truncated — a window that ran off
 * the start would otherwise arrive shorter than the minimum it was grown to.
 */
function widened(window: DistanceWindow, totalMetres: number): DistanceWindow {
  const span = window.endMetres - window.startMetres;
  if (span >= MIN_WINDOW_METRES) {
    return window;
  }
  const wanted = Math.min(MIN_WINDOW_METRES, totalMetres);
  const middle = (window.startMetres + window.endMetres) / 2;
  const start = Math.min(Math.max(middle - wanted / 2, 0), Math.max(totalMetres - wanted, 0));

  return { startMetres: start, endMetres: start + wanted };
}

/**
 * Claims the pointer for a drag, where that is possible.
 *
 * A browser refuses the call for a pointer that has already gone up, and jsdom
 * implements no capture at all. Neither is worth failing a drag over: without
 * capture the drag simply ends when the pointer leaves the chart, which is the
 * behaviour there was before it.
 */
function capturePointer(element: Element, pointerId: number) {
  try {
    element.setPointerCapture(pointerId);
  } catch {
    // Nothing to do. The gesture works without it, less forgivingly.
  }
}

export function ElevationProfile({
  profile,
  title,
  surface = null,
  activeMetres,
  onActiveChange,
  zoomWindow = null,
  onZoomChange,
  highlight = null,
}: ElevationProfileProps) {
  const { ref, width } = useElementWidth<HTMLDivElement>();

  const plotWidth = Math.max(width, MIN_WIDTH) - PADDING.left - PADDING.right;
  const plotHeight = HEIGHT - PADDING.top - PADDING.bottom;
  // The plot and the strip beneath it, which the cursor, the scrub region and
  // the veil all treat as one lane because they describe one position.
  const laneHeight = surface ? plotHeight + SURFACE_STRIP_GAP + SURFACE_STRIP_HEIGHT : plotHeight;

  const drag = useRef<{ pointerId: number; originX: number; anchorMetres: number } | null>(null);
  const [selection, setSelection] = useState<DistanceWindow | null>(null);

  const geometry = useMemo(() => {
    if (!profile || plotWidth <= 0) {
      return null;
    }
    // A flat route still needs a band to draw in, so give it one.
    const span = Math.max(profile.maxElevationMetres - profile.minElevationMetres, 10);
    const low = profile.minElevationMetres;
    // A window of no length cannot happen, but dividing by one would put every
    // mark on the chart at the same place rather than say so.
    const shown = Math.max(profile.endMetres - profile.startMetres, 1);

    const x = (metres: number) => ((metres - profile.startMetres) / shown) * plotWidth;
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
      distanceTicks: ticksFor(profile.startMetres / 1000, profile.endMetres / 1000, 5),
      distanceStep: niceStep(shown / 1000, 5),
      surfaceBands: surface
        ? surfaceBandsWithin(surface, profile.startMetres, profile.endMetres)
        : [],
    };
  }, [profile, surface, plotWidth, plotHeight]);

  /**
   * The ground the picked class does not cover, which is what gets veiled.
   *
   * Kept apart from `geometry` so that picking a class redraws no terrain: the
   * columns and the strip are the same marks either way, and only the light on
   * them changes. Whichever kind of class was picked, the answer is a list of
   * stretches, so both are veiled by one rule.
   */
  const veiled = useMemo(() => {
    if (!highlight || !geometry || !profile) {
      return [];
    }
    const lit =
      highlight.type === "band"
        ? geometry.runs.filter((run) => run.band === highlight.band)
        : geometry.surfaceBands.filter((band) => band.kind === highlight.kind);

    return gapsOutside(lit, profile.startMetres, profile.endMetres);
  }, [geometry, highlight, profile]);

  /**
   * Where along the route the pointer is, in metres.
   *
   * Null for a box of no width, which is what an element reports before it is
   * laid out: there is no position to read off it, and dividing by it would
   * report NaN as a place on the route.
   */
  const metresAt = useCallback(
    (event: React.PointerEvent<HTMLDivElement>): number | null => {
      const bounds = event.currentTarget.getBoundingClientRect();
      if (!profile || bounds.width <= 0) {
        return null;
      }
      const ratio = (event.clientX - bounds.left) / bounds.width;
      const metres = profile.startMetres + ratio * (profile.endMetres - profile.startMetres);

      return Math.min(Math.max(metres, profile.startMetres), profile.endMetres);
    },
    [profile],
  );

  const endDrag = useCallback((event: React.PointerEvent<HTMLDivElement>) => {
    drag.current = null;
    setSelection(null);
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  }, []);

  const onPointerDown = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      // A right-click opens a menu; it must not leave a range half-drawn behind
      // it. A second finger is not a second selection either.
      if (!onZoomChange || !event.isPrimary || event.button !== 0) {
        return;
      }
      const metres = metresAt(event);
      if (metres === null) {
        return;
      }
      drag.current = { pointerId: event.pointerId, originX: event.clientX, anchorMetres: metres };
      setSelection(null);
      capturePointer(event.currentTarget, event.pointerId);
      // Report the anchor, so the readout is already showing the value under
      // the finger that is about to drag away from it.
      onActiveChange(metres);
    },
    [metresAt, onActiveChange, onZoomChange],
  );

  const onPointerMove = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      const metres = metresAt(event);
      if (metres === null) {
        return;
      }
      onActiveChange(metres);

      const started = drag.current;
      if (!started || started.pointerId !== event.pointerId) {
        return;
      }
      if (Math.abs(event.clientX - started.originX) < MIN_DRAG_PIXELS) {
        return;
      }
      setSelection({
        startMetres: Math.min(started.anchorMetres, metres),
        endMetres: Math.max(started.anchorMetres, metres),
      });
    },
    [metresAt, onActiveChange],
  );

  const onPointerUp = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      const started = drag.current;
      const chosen = selection;
      endDrag(event);
      if (!started || started.pointerId !== event.pointerId || !chosen || !profile) {
        return;
      }
      onZoomChange?.(widened(chosen, profile.totalDistanceMetres));
    },
    [endDrag, onZoomChange, profile, selection],
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
      const step =
        ((profile.endMetres - profile.startMetres) / Math.max(profile.samples.length - 1, 1)) * 4;
      const next = (activeMetres ?? profile.startMetres) + direction * step;
      onActiveChange(Math.min(Math.max(next, profile.startMetres), profile.endMetres));
    },
    [profile, activeMetres, onActiveChange],
  );

  /**
   * Escape returns to the whole route.
   *
   * A listener on the document rather than a key handler on the scrub, because
   * the gesture that zooms is a drag with a pointer and leaves focus wherever
   * the drag began. The way out of a view has to work from wherever the reader
   * is. It is registered only while zoomed, so it swallows nothing the rest of
   * the time.
   */
  useEffect(() => {
    if (!zoomWindow || !onZoomChange) {
      return;
    }
    const onEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !event.defaultPrevented) {
        onZoomChange(null);
      }
    };
    document.addEventListener("keydown", onEscape);

    return () => document.removeEventListener("keydown", onEscape);
  }, [zoomWindow, onZoomChange]);

  if (!profile || !geometry) {
    return (
      <p className="elevation-profile__absent">
        This route has no elevation data, so it has no profile to show.
      </p>
    );
  }

  const zoomed = zoomWindow !== null && onZoomChange !== undefined;
  const active = activeMetres === null ? null : sampleAt(profile, activeMetres);
  const activeKind = active && surface ? surfaceKindAt(surface, active.distanceMetres) : null;
  const activeSurface = activeKind ? SURFACE_STYLES[activeKind].label : null;
  const shownLabel =
    `${kilometreLabel(profile.startMetres, geometry.distanceStep)}–` +
    `${kilometreLabel(profile.endMetres, geometry.distanceStep)} km`;
  // The picked class is said in words as well as in light: a chart that is
  // mostly veiled has to explain itself to a reader who cannot see the veil.
  const picked = highlight ? highlightLabel(highlight) : "";
  const summary =
    `Elevation profile of ${title}` +
    (zoomed ? `, ${shownLabel}` : "") +
    `: ${((profile.endMetres - profile.startMetres) / 1000).toFixed(1)} kilometres, ` +
    `between ${Math.round(profile.minElevationMetres)} and ` +
    `${Math.round(profile.maxElevationMetres)} metres above sea level.` +
    (picked ? ` Only the ${picked} stretches are lit.` : "");

  return (
    <div className="elevation-profile" ref={ref} data-zoomed={zoomed ? "true" : undefined}>
      <svg
        width="100%"
        height={HEIGHT}
        viewBox={`0 0 ${plotWidth + PADDING.left + PADDING.right} ${HEIGHT}`}
        role="img"
        aria-label={summary}
      >
        <title>{summary}</title>
        <defs>
          {Object.entries(HATCHES).map(([band, hatch]) => (
            <pattern
              key={band}
              id={`elevation-hatch-${band}`}
              width={hatch.size}
              height={hatch.size}
              patternUnits="userSpaceOnUse"
              patternTransform={`rotate(${hatch.angle})`}
            >
              <rect
                className="elevation-profile__hatch-ground"
                data-band={band}
                width={hatch.size}
                height={hatch.size}
              />
              {/*
               * Centred in the tile, not on its edge: a stroke on the edge is
               * clipped in half, so half the intended width simply vanishes.
               */}
              <line
                className="elevation-profile__hatch-line"
                data-band={band}
                x1={hatch.size / 2}
                y1={0}
                x2={hatch.size / 2}
                y2={hatch.size}
              />
              {hatch.crossed ? (
                <line
                  className="elevation-profile__hatch-line"
                  data-band={band}
                  x1={0}
                  y1={hatch.size / 2}
                  x2={hatch.size}
                  y2={hatch.size / 2}
                />
              ) : null}
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
          {geometry.surfaceBands.map((band) => {
            const style = SURFACE_STYLES[band.kind];

            return (
              <line
                key={`${band.kind}-${band.startMetres}`}
                className="elevation-profile__surface"
                x1={geometry.x(band.startMetres)}
                x2={geometry.x(band.endMetres)}
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
          })}

          {geometry.distanceTicks.map((kilometres) => (
            <text
              key={kilometres}
              className="elevation-profile__tick"
              x={geometry.x(kilometres * 1000)}
              y={plotHeight + SURFACE_STRIP_GAP + SURFACE_STRIP_HEIGHT + 15}
              textAnchor="middle"
            >
              {kilometreLabel(kilometres * 1000, geometry.distanceStep)}
            </text>
          ))}

          {/*
           * Everything the picked class does not cover, veiled. The marks keep
           * the colour and the pattern that give them their meaning — brightening
           * a band's column would change the very thing being asked about — so
           * what the selection changes is the light on the rest.
           */}
          {veiled.map((gap) => (
            <rect
              key={gap.startMetres}
              className="elevation-profile__veil"
              x={Math.max(geometry.x(gap.startMetres), 0)}
              y={0}
              width={Math.max(
                geometry.x(gap.endMetres) - Math.max(geometry.x(gap.startMetres), 0),
                0,
              )}
              height={laneHeight}
            />
          ))}

          {/*
           * The stretch being chosen, with what is being left behind veiled
           * rather than hidden — the ride does not stop at the edges of the
           * window, and while the reader is still choosing, the chart should not
           * pretend it does. It is the same treatment the map gives the route
           * outside the window, so the two views say one thing one way.
           */}
          {selection ? (
            <g className="elevation-profile__selection">
              <rect
                className="elevation-profile__veil"
                x={0}
                y={0}
                width={Math.max(geometry.x(selection.startMetres), 0)}
                height={laneHeight}
              />
              <rect
                className="elevation-profile__veil"
                x={geometry.x(selection.endMetres)}
                y={0}
                width={Math.max(plotWidth - geometry.x(selection.endMetres), 0)}
                height={laneHeight}
              />
              {[selection.startMetres, selection.endMetres].map((metres) => (
                <line
                  key={metres}
                  className="elevation-profile__selection-edge"
                  x1={geometry.x(metres)}
                  x2={geometry.x(metres)}
                  y1={0}
                  y2={laneHeight}
                />
              ))}
            </g>
          ) : null}

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
                y2={laneHeight}
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
       * which a non-interactive <svg> cannot carry. The drag that zooms is a
       * pointer shortcut layered over it, not a second control — the way back is
       * the button in the footer, which is the half a keyboard can reach.
       */}
      <div
        className="elevation-profile__scrub"
        data-dragging={selection ? "true" : undefined}
        style={{
          left: PADDING.left,
          top: PADDING.top,
          width: plotWidth,
          // Down to the foot of the surface strip when there is one. The cursor
          // crosses both lanes because they describe one position, so a pointer
          // that wandered onto the strip must not fall off the instrument and
          // blank the readout it came to check.
          height: laneHeight,
        }}
        role="slider"
        tabIndex={0}
        aria-label={
          zoomed ? `Position along ${title}, ${shownLabel} shown` : `Position along ${title}`
        }
        aria-valuemin={Number((profile.startMetres / 1000).toFixed(1))}
        aria-valuemax={Number((profile.endMetres / 1000).toFixed(1))}
        // Falls back to the start of the stretch on show, not to zero: zoomed
        // into the far end of a route, zero is outside the range this slider
        // declares, and a value outside its own bounds is nothing assistive
        // technology can place.
        aria-valuenow={Number(((active?.distanceMetres ?? profile.startMetres) / 1000).toFixed(1))}
        aria-valuetext={
          active
            ? `${Math.round(active.elevationMetres)} metres at ${(active.distanceMetres / 1000).toFixed(1)} kilometres, ${active.gradientPercent.toFixed(1)} percent` +
              (activeSurface ? `, ${activeSurface.toLowerCase()}` : "")
            : "No position selected"
        }
        onKeyDown={onKeyDown}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={endDrag}
        onLostPointerCapture={endDrag}
        /*
         * With pointer capture this fires only once the pointer is released, so
         * a drag follows the hand off the chart and back. Without it — a browser
         * that refused the claim — the chart stops hearing about the pointer at
         * its own edge, and a drag that leaves has to end here: the pointerup it
         * is waiting for will be delivered somewhere else, and the band would
         * stay painted over a gesture nobody is making any more.
         */
        onPointerLeave={(event) => {
          const started = drag.current;
          if (started && event.currentTarget.hasPointerCapture?.(started.pointerId)) {
            return;
          }
          endDrag(event);
          onActiveChange(null);
        }}
        onBlur={() => onActiveChange(null)}
      />

      <div className="elevation-profile__status">
        {/*
         * Only while zoomed. A permanently present, permanently disabled way
         * back is a control that spends most of its life lying about what it
         * does. The span sits inside the button, so the name says what is
         * being left as well as where it goes.
         */}
        {zoomed && onZoomChange ? (
          <button
            type="button"
            className="elevation-profile__reset"
            aria-keyshortcuts="Escape"
            onClick={() => onZoomChange(null)}
          >
            Whole route
            <span className="elevation-profile__reset-span"> · showing {shownLabel}</span>
          </button>
        ) : null}
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
      </div>
    </div>
  );
}
