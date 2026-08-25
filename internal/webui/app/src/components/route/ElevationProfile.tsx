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
import type { Highlight } from "../../lib/highlight";
import { gapsOutside, highlightLabel } from "../../lib/highlight";
import { useNarrowViewport } from "../../lib/mediaQuery";
import { PADDING, plotAxis } from "../../lib/plotAxis";
import type { DistanceWindow, Profile, ProfileSample } from "../../lib/profile";
import { niceStep, sampleAt, ticksFor } from "../../lib/profile";
import { MIN_DRAG_PIXELS, spanBetween, widened } from "../../lib/selection";
import type { SurfaceSummary } from "../../lib/surface";
import { SURFACE_STYLES, surfaceBandsWithin, surfaceKindAt } from "../../lib/surface";
import type { UnitSystem } from "../../lib/units";
import {
  distanceLabel,
  distanceUnitLabel,
  distanceValue,
  elevationUnitLabel,
  elevationValue,
} from "../../lib/units";
import { useElementWidth } from "../../lib/useElementWidth";
import { useEscapeKey } from "../../lib/useEscapeKey";
import { Button } from "../Button";

/**
 * How tall the drawn terrain is, in the two layouts there are.
 *
 * The chart is an SVG, so its height is also its coordinate system and cannot be
 * a rule in the stylesheet. It is one row of a card now rather than a panel of
 * its own, so both numbers are what a card can spare: enough for the ride to be
 * a shape rather than a smear, and no more than the four figures above it and
 * the two mixes below it can be read alongside.
 */
const PLOT_HEIGHT = { wide: 92, narrow: 74 } as const;
const bandPaint = [
  "fill-[var(--grade-0)] stroke-[var(--grade-0)]",
  "fill-[var(--grade-1)] stroke-[var(--grade-1)]",
  "fill-[var(--grade-2)] stroke-[var(--grade-2)]",
  "fill-[var(--grade-3)] stroke-[var(--grade-3)]",
  "fill-[var(--grade-4)] stroke-[var(--grade-4)]",
] as const;

/**
 * How long a finger has to stay put before the drag is armed.
 *
 * A card that scrolls cannot give every downward swipe over the chart to the
 * chart, and a finger that landed and moved on was scrolling. Long enough to be
 * a deliberate hold, short enough that holding does not feel like waiting.
 */
export const LONG_PRESS_MS = 350;

export interface ElevationProfileProps {
  /** Already restricted to the stretch on show, when there is a zoom. */
  profile: Profile | null;
  title: string;
  /**
   * The ground under the route: reported for the hovered position, and veiled
   * around whichever class the reader picked out of the chips.
   *
   * Nothing of it is drawn along the chart. It used to have a lane of its own at
   * the foot of the plot, back when the plot was a panel with room to spare;
   * inside the card it would be a second chart on the same axis, two rows above
   * the surface mix bar that already says the same thing at a glance.
   *
   * This is the whole route's classification, not the window's: the readout asks
   * it about an absolute distance.
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
  /** The units the axis, the tooltip, and the accessible readout report in. */
  unitSystem?: UnitSystem;
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
  // The samples carry the stage's classification; the chart does not re-derive
  // one from its own spacing, or it could paint a band the key has no chip for.
  const bands = samples.map((sample) => sample.band);
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
      const crest = slice
        .map(
          (sample, offset) =>
            `${offset === 0 ? "M" : "L"}${x(sample.distanceMetres).toFixed(1)},${y(sample.elevationMetres).toFixed(1)}`,
        )
        .join(" ");

      runs.push({
        band: bands[start] ?? 0,
        column: `${crest} L${x(last.distanceMetres).toFixed(1)},${plotHeight.toFixed(1)} L${x(first.distanceMetres).toFixed(1)},${plotHeight.toFixed(1)} Z`,
        startMetres: first.distanceMetres,
        endMetres: last.distanceMetres,
      });
    }
    start = index;
  }

  return runs;
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
  unitSystem = "metric",
}: ElevationProfileProps) {
  const { ref, width } = useElementWidth<HTMLDivElement>();

  const plotHeight = useNarrowViewport() ? PLOT_HEIGHT.narrow : PLOT_HEIGHT.wide;
  const height = plotHeight + PADDING.top + PADDING.bottom;

  // Measured through the shared axis rather than repeating its arithmetic, so
  // the chart and the forecast strip below it cannot disagree about how much
  // room the terrain has.
  const { plotWidth } = plotAxis(width, 0, 1);

  const drag = useRef<{ pointerId: number; originX: number; anchorMetres: number } | null>(null);
  /*
   * The hold that arms a finger's drag, and whether it has finished.
   *
   * The timer is a ref because clearing it is cleanup rather than rendering;
   * the flag is state because the chart says out loud that it has the gesture —
   * `touch-action` stops being `pan-y` and the plot stops handing downward
   * movement back to the card it is scrolling in.
   */
  const armed = useRef<number | null>(null);
  const [holding, setHolding] = useState(false);
  const [selection, setSelection] = useState<DistanceWindow | null>(null);

  const geometry = useMemo(() => {
    if (!profile || plotWidth <= 0) {
      return null;
    }
    // A flat route still needs a band to draw in, so give it one.
    const span = Math.max(profile.maxElevationMetres - profile.minElevationMetres, 10);
    const low = profile.minElevationMetres;
    // A window of no length cannot happen, but dividing by one would put every
    // mark on the chart at the same place rather than say so — the same
    // shortfall `plotAxis` itself guards against for `x`.
    const shown = Math.max(profile.endMetres - profile.startMetres, 1);

    // The same axis the forecast strip draws its cells against, so the two
    // never disagree about where a distance sits by a rounding error.
    const { x } = plotAxis(width, profile.startMetres, profile.endMetres);
    const y = (metres: number) => plotHeight - ((metres - low) / span) * plotHeight;

    return {
      x,
      y,
      runs: runsOf(profile.samples, x, y, plotHeight),
      elevationTicks: ticksFor(low, low + span, 3),
      distanceTicks: ticksFor(profile.startMetres / 1000, profile.endMetres / 1000, 5),
      distanceStep: niceStep(shown / 1000, 5),
      surfaceBands: surface
        ? surfaceBandsWithin(surface, profile.startMetres, profile.endMetres)
        : [],
    };
  }, [profile, surface, width, plotWidth, plotHeight]);

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

  /** Stands the pending hold down, whether or not it ever became a gesture. */
  const disarm = useCallback(() => {
    if (armed.current !== null) {
      window.clearTimeout(armed.current);
      armed.current = null;
    }
    setHolding(false);
  }, []);

  const endDrag = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      drag.current = null;
      setSelection(null);
      disarm();
      if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
        event.currentTarget.releasePointerCapture(event.pointerId);
      }
    },
    [disarm],
  );

  // Nothing is left ticking when the chart is folded away mid-hold.
  useEffect(() => disarm, [disarm]);

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
      const started = {
        pointerId: event.pointerId,
        originX: event.clientX,
        anchorMetres: metres,
      };
      setSelection(null);
      /*
       * Whatever the last press left ticking is dropped before this one arms.
       * A press does not always arrive after the release of the one before it —
       * a device with both a touchscreen and a trackpad can put a second
       * primary pointer down while a hold is still counting — and a timer from
       * an abandoned press would fire into this one, capturing a pointer for a
       * position the reader has already moved away from.
       */
      disarm();
      /*
       * A mouse arms the drag by pressing; a finger arms it by holding.
       *
       * The chart is one row of a card the reader scrolls, and a finger that
       * lands on the plot is far more often on its way past than asking about
       * the climb under it. Claiming that first touch would make the card
       * un-scrollable over its most interesting row. So the hold is the ask:
       * until it completes, the touch belongs to the card.
       */
      if (event.pointerType === "touch") {
        const element = event.currentTarget;
        armed.current = window.setTimeout(() => {
          armed.current = null;
          drag.current = started;
          setHolding(true);
          capturePointer(element, started.pointerId);
          onActiveChange(started.anchorMetres);
        }, LONG_PRESS_MS);

        return;
      }
      drag.current = started;
      capturePointer(event.currentTarget, event.pointerId);
      // Report the anchor, so the readout is already showing the value under
      // the pointer that is about to drag away from it.
      onActiveChange(metres);
    },
    [disarm, metresAt, onActiveChange, onZoomChange],
  );

  const onPointerMove = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      const metres = metresAt(event);
      if (metres === null) {
        return;
      }
      /*
       * A finger that moved before the hold completed was scrolling, so the
       * hold is abandoned rather than made easier to hit: an armed gesture the
       * reader did not ask for takes the card's scroll away mid-swipe.
       */
      if (armed.current !== null) {
        disarm();

        return;
      }
      // Only the pointer that can hover reports a position it is merely passing
      // over. A finger reports one once it has asked for the chart.
      if (event.pointerType !== "touch" || drag.current !== null) {
        onActiveChange(metres);
      }

      const started = drag.current;
      if (!started || started.pointerId !== event.pointerId) {
        return;
      }
      if (Math.abs(event.clientX - started.originX) < MIN_DRAG_PIXELS) {
        return;
      }
      setSelection(spanBetween(started.anchorMetres, metres));
    },
    [disarm, metresAt, onActiveChange],
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
   * The map carries the same way out, because the same window can be drawn on
   * either instrument and this chart is not on the page when the overview is
   * collapsed. Both simply ask for the whole route back, and the hook stands the
   * second one down, so whichever hears the key first the reader gets the same
   * view from one press.
   */
  useEscapeKey(zoomWindow !== null && onZoomChange !== undefined, () => onZoomChange?.(null));

  if (!profile || !geometry) {
    return (
      <p className="text-sm text-[var(--ink-2)]">
        This route has no elevation data, so it has no profile to show.
      </p>
    );
  }

  const zoomed = zoomWindow !== null && onZoomChange !== undefined;
  const active = activeMetres === null ? null : sampleAt(profile, activeMetres);
  const activeKind = active && surface ? surfaceKindAt(surface, active.distanceMetres) : null;
  const activeSurface = activeKind ? SURFACE_STYLES[activeKind].label : null;
  const distanceWord = unitSystem === "imperial" ? "miles" : "kilometres";
  const elevationWord = unitSystem === "imperial" ? "feet" : "metres";
  const shownLabel =
    `${distanceLabel(profile.startMetres, geometry.distanceStep, unitSystem)}–` +
    `${distanceLabel(profile.endMetres, geometry.distanceStep, unitSystem)} ${distanceUnitLabel(unitSystem)}`;
  // The picked class is said in words as well as in light: a chart that is
  // mostly veiled has to explain itself to a reader who cannot see the veil.
  const picked = highlight ? highlightLabel(highlight) : "";
  const summary =
    `Elevation profile of ${title}` +
    (zoomed ? `, ${shownLabel}` : "") +
    `: ${distanceValue(profile.endMetres - profile.startMetres, unitSystem).toFixed(1)} ${distanceWord}, ` +
    `between ${Math.round(elevationValue(profile.minElevationMetres, unitSystem))} and ` +
    `${Math.round(elevationValue(profile.maxElevationMetres, unitSystem))} ${elevationWord} above sea level.` +
    (picked ? ` Only the ${picked} stretches are lit.` : "");

  return (
    <div className="relative" ref={ref} data-zoomed={zoomed ? "true" : undefined}>
      <svg
        className="block overflow-visible"
        width="100%"
        height={height}
        viewBox={`0 0 ${plotWidth + PADDING.left + PADDING.right} ${height}`}
        role="img"
        aria-label={summary}
      >
        <title>{summary}</title>
        <g transform={`translate(${PADDING.left} ${PADDING.top})`}>
          {geometry.elevationTicks.map((metres) => (
            <g key={metres}>
              <line
                className="stroke-[var(--rule)] [stroke-width:1]"
                x1={0}
                x2={plotWidth}
                y1={geometry.y(metres)}
                y2={geometry.y(metres)}
              />
              <text
                className="fill-[var(--ink-2)] text-xs [font-variant-numeric:tabular-nums]"
                x={-8}
                y={geometry.y(metres)}
                textAnchor="end"
                dominantBaseline="middle"
              >
                {Math.round(elevationValue(metres, unitSystem))}
              </text>
            </g>
          ))}

          {geometry.runs.map((run, index) => (
            <path
              // Runs are positional slices of one profile; there is no id to key on.
              // biome-ignore lint/suspicious/noArrayIndexKey: positional by nature
              key={`column-${index}`}
              className={`[fill-opacity:0.5] [stroke-linejoin:round] [stroke-width:2.4] forced-colors:forced-color-adjust-none ${bandPaint[run.band] ?? bandPaint[0]}`}
              data-band={run.band}
              d={run.column}
            />
          ))}
          {geometry.distanceTicks.map((kilometres) => (
            <text
              key={kilometres}
              className="fill-[var(--ink-2)] text-xs [font-variant-numeric:tabular-nums]"
              x={geometry.x(kilometres * 1000)}
              y={plotHeight + 15}
              textAnchor="middle"
            >
              {distanceLabel(kilometres * 1000, geometry.distanceStep, unitSystem)}
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
              className="fill-[var(--panel)] opacity-60 forced-colors:fill-[Canvas]"
              data-testid="profile-veil"
              x={Math.max(geometry.x(gap.startMetres), 0)}
              y={0}
              width={Math.max(
                geometry.x(gap.endMetres) - Math.max(geometry.x(gap.startMetres), 0),
                0,
              )}
              height={plotHeight}
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
            <g>
              <rect
                className="fill-[var(--panel)] opacity-60 forced-colors:fill-[Canvas]"
                data-testid="profile-veil"
                x={0}
                y={0}
                width={Math.max(geometry.x(selection.startMetres), 0)}
                height={plotHeight}
              />
              <rect
                className="fill-[var(--panel)] opacity-60 forced-colors:fill-[Canvas]"
                data-testid="profile-veil"
                x={geometry.x(selection.endMetres)}
                y={0}
                width={Math.max(plotWidth - geometry.x(selection.endMetres), 0)}
                height={plotHeight}
              />
              {[selection.startMetres, selection.endMetres].map((metres) => (
                <line
                  key={metres}
                  className="stroke-[var(--accent)] [stroke-dasharray:3_3] [stroke-width:1.5]"
                  x1={geometry.x(metres)}
                  x2={geometry.x(metres)}
                  y1={0}
                  y2={plotHeight}
                />
              ))}
            </g>
          ) : null}

          {active ? (
            <g data-testid="profile-cursor">
              <line
                className="stroke-[var(--ink-2)] [stroke-width:1]"
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
                className={`stroke-[var(--panel)] [paint-order:stroke] [stroke-width:2] forced-colors:forced-color-adjust-none ${bandPaint[active.band] ?? bandPaint[0]}`}
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
        className="absolute touch-pan-y cursor-crosshair select-none [-webkit-touch-callout:none] data-[holding=true]:touch-none data-[dragging=true]:cursor-ew-resize focus-visible:rounded-sm focus-visible:outline-2 focus-visible:outline-offset-3 focus-visible:outline-[var(--accent)]"
        data-dragging={selection ? "true" : undefined}
        // Until a finger has asked for the chart, a swipe over it is the card's
        // to scroll — which is a rule the stylesheet cannot write, because only
        // the chart knows whether the hold completed.
        data-holding={holding ? "true" : undefined}
        style={{
          left: PADDING.left,
          top: PADDING.top,
          width: plotWidth,
          height: plotHeight,
        }}
        role="slider"
        tabIndex={0}
        aria-label={
          zoomed ? `Position along ${title}, ${shownLabel} shown` : `Position along ${title}`
        }
        aria-valuemin={Number(
          distanceLabel(profile.startMetres, geometry.distanceStep, unitSystem),
        )}
        aria-valuemax={Number(distanceLabel(profile.endMetres, geometry.distanceStep, unitSystem))}
        // Falls back to the start of the stretch on show, not to zero: zoomed
        // into the far end of a route, zero is outside the range this slider
        // declares, and a value outside its own bounds is nothing assistive
        // technology can place.
        //
        // The same adaptive precision the axis labels use, not a fixed decimal:
        // a mile is coarse enough that a fixed tenth can hold still for a
        // couple of hundred metres of drag, which is a slider that stopped
        // reporting position rather than one reporting it coarsely.
        aria-valuenow={Number(
          distanceLabel(
            active?.distanceMetres ?? profile.startMetres,
            geometry.distanceStep,
            unitSystem,
          ),
        )}
        aria-valuetext={
          active
            ? `${Math.round(elevationValue(active.elevationMetres, unitSystem))} ${elevationWord} at ${distanceLabel(active.distanceMetres, geometry.distanceStep, unitSystem)} ${distanceWord}, ${active.gradientPercent.toFixed(1)} percent` +
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

      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
        {/*
         * Only while zoomed. A permanently present, permanently disabled way
         * back is a control that spends most of its life lying about what it
         * does. The span sits inside the button, so the name says what is
         * being left as well as where it goes.
         */}
        {zoomed && onZoomChange ? (
          <Button variant="standard" aria-keyshortcuts="Escape" onClick={() => onZoomChange(null)}>
            Whole route
            <span className="tabular-nums"> · showing {shownLabel}</span>
          </Button>
        ) : null}
        <p className="text-xs text-[var(--ink-2)] tabular-nums" aria-live="polite">
          {active ? (
            <>
              <strong className="font-semibold text-[var(--ink)]">
                {Math.round(elevationValue(active.elevationMetres, unitSystem))}{" "}
                {elevationUnitLabel(unitSystem)}
              </strong>
              <span>
                {" "}
                at {distanceLabel(active.distanceMetres, geometry.distanceStep, unitSystem)}{" "}
                {distanceUnitLabel(unitSystem)}
              </span>
              <span> · {active.gradientPercent.toFixed(1)}%</span>
              {activeSurface ? <span> · {activeSurface}</span> : null}
            </>
          ) : (
            <span>
              {Math.round(elevationValue(profile.minElevationMetres, unitSystem))}–
              {Math.round(elevationValue(profile.maxElevationMetres, unitSystem))}{" "}
              {elevationUnitLabel(unitSystem)} above sea level
            </span>
          )}
        </p>
      </div>
    </div>
  );
}
