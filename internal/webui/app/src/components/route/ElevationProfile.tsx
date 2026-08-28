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
 *
 * The division of labour here is the thing to understand before changing it:
 * **Recharts draws, the overlay interacts.** Every mark — axes, gridlines,
 * terrain, veils, cursor — is a declarative Recharts child. Everything a reader
 * *does* belongs to the `role="slider"` overlay at the foot of this file, which
 * reads its position off its own bounding box and so neither knows nor cares
 * what drew the terrain beneath it. Recharts' own mouse handlers are
 * deliberately unused: routing interaction through them costs the long-press
 * that keeps a finger's swipe belonging to the scrolling card, arrow-key
 * stepping, and the spoken value at each step, and buys nothing back.
 *
 * The chart's geometry is pinned to the shared `plotAxis` rather than left to
 * Recharts to decide, and `PADDING` reaches it by two routes: the top and right
 * are margins, while the left and bottom are the room the axes reserve for
 * themselves — `YAxis width` is `PADDING.left` and `XAxis height` is
 * `PADDING.bottom`, so those two margins are zero rather than counting the same
 * space twice. Together that lands the plot area on exactly the pixels the
 * shared axis computes, which is what lets the forecast strip below keep
 * landing its cells under the terrain they describe — and what lets the overlay
 * be positioned from the same numbers.
 *
 * The width is measured and passed explicitly rather than handed to Recharts'
 * `ResponsiveContainer`. One measurement, taken once, serves the chart and the
 * overlay, so the two cannot disagree; and `ResponsiveContainer` renders
 * nothing at all in a layout-less DOM, which would leave the jsdom suite
 * asserting against an empty chart.
 */

import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ReferenceArea,
  ReferenceDot,
  ReferenceLine,
  XAxis,
  YAxis,
} from "recharts";
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
 * It is one row of a card rather than a panel of its own, so both numbers are
 * what a card can spare: enough for the ride to be a shape rather than a smear,
 * and no more than the four figures above it and the two mixes below it can be
 * read alongside.
 */
const PLOT_HEIGHT = { wide: 92, narrow: 74 } as const;

/** The ordinal ramp the terrain is banded with, as values a gradient can take. */
const BAND_COLOURS = [
  "var(--grade-0)",
  "var(--grade-1)",
  "var(--grade-2)",
  "var(--grade-3)",
  "var(--grade-4)",
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
   * Nothing of it is drawn along the chart — inside the card it would be a
   * second chart on the same axis, two rows above the surface mix bar that
   * already says the same thing at a glance.
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

/** One stretch of ground of a single steepness band. */
interface BandRun {
  band: number;
  startMetres: number;
  endMetres: number;
}

/**
 * Groups consecutive samples of one band into runs.
 *
 * A run only has to know where it starts and ends, because the terrain is
 * filled by one gradient across the whole plot rather than by a shape per run.
 * Runs share their boundary, so neighbouring bands meet exactly — neither
 * overlapping into a darker seam nor leaving a sliver of surface.
 */
function bandRunsOf(samples: ProfileSample[]): BandRun[] {
  const runs: BandRun[] = [];
  for (const sample of samples) {
    const open = runs[runs.length - 1];
    if (!open) {
      runs.push({
        band: sample.band,
        startMetres: sample.distanceMetres,
        endMetres: sample.distanceMetres,
      });
      continue;
    }
    if (open.band === sample.band) {
      open.endMetres = sample.distanceMetres;
      continue;
    }
    /*
     * The sample that changed band belongs to both runs: the outgoing one is
     * carried up to it and the incoming one begins there. Ending the outgoing
     * run at the sample before instead would step the colour a sample early and
     * leave the two runs meeting at a distance neither of them is.
     */
    open.endMetres = sample.distanceMetres;
    runs.push({
      band: sample.band,
      startMetres: sample.distanceMetres,
      endMetres: sample.distanceMetres,
    });
  }

  return runs;
}

/**
 * The runs, as hard-edged stops across the plotted stretch.
 *
 * Two stops land on every boundary — the outgoing colour and the incoming one —
 * which is what makes the ramp step rather than blend.
 */
function stopsOf(runs: BandRun[], startMetres: number, endMetres: number) {
  const span = Math.max(endMetres - startMetres, 1);
  const at = (metres: number) => `${(((metres - startMetres) / span) * 100).toFixed(3)}%`;

  return runs.flatMap((run, index) => [
    { key: `${index}-in`, offset: at(run.startMetres), band: run.band },
    { key: `${index}-out`, offset: at(run.endMetres), band: run.band },
  ]);
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
  const gradientId = useId();
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
    if (!profile) {
      return null;
    }
    // A flat route still needs a band to draw in, so give it one.
    const span = Math.max(profile.maxElevationMetres - profile.minElevationMetres, 10);
    const low = profile.minElevationMetres;
    const shown = Math.max(profile.endMetres - profile.startMetres, 1);
    const runs = bandRunsOf(profile.samples);

    return {
      low,
      high: low + span,
      runs,
      stops: stopsOf(runs, profile.startMetres, profile.endMetres),
      elevationTicks: ticksFor(low, low + span, 3),
      // The axis carries metres, so its ticks do too.
      distanceTicks: ticksFor(profile.startMetres / 1000, profile.endMetres / 1000, 5).map(
        (kilometres) => kilometres * 1000,
      ),
      distanceStep: niceStep(shown / 1000, 5),
      surfaceBands: surface
        ? surfaceBandsWithin(surface, profile.startMetres, profile.endMetres)
        : [],
    };
  }, [profile, surface]);

  /**
   * The ground the picked class does not cover, which is what gets veiled.
   *
   * Kept apart from `geometry` so that picking a class redraws no terrain: the
   * bands are the same marks either way, and only the light on them changes.
   * Whichever kind of class was picked, the answer is a list of stretches, so
   * both are veiled by one rule.
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
      <div>
        <AreaChart
          data={profile.samples}
          width={plotWidth + PADDING.left + PADDING.right}
          height={height}
          /*
           * The labelled graphic is the chart's own `<svg>` rather than a
           * wrapper around it, because the strip below is checked against this
           * one's `viewBox` — the two plot the same stretch and must measure it
           * identically, which is a claim only the drawn surfaces can make.
           */
          role="img"
          aria-label={summary}
          // Pinned to the shared axis; see the note at the top of this file.
          margin={{ top: PADDING.top, right: PADDING.right, bottom: 0, left: 0 }}
          /*
           * The slider below is this chart's accessibility layer, and it is the
           * one that knows the position is shared with the map. Recharts' own
           * puts a second tab stop in front of it with a second keyboard model
           * for the same instrument, which is one control too many however it
           * is spelled.
           */
          accessibilityLayer={false}
        >
          <defs>
            <linearGradient id={gradientId} x1="0" y1="0" x2="1" y2="0">
              {geometry.stops.map((stop) => (
                <stop
                  key={stop.key}
                  offset={stop.offset}
                  stopColor={BAND_COLOURS[stop.band] ?? BAND_COLOURS[0]}
                  // Which band this stop carries. The terrain is one shape, so
                  // these are the only marks that hold the ramp: they are what
                  // the forced-colours audit reads the encoding off.
                  data-band={stop.band}
                />
              ))}
            </linearGradient>
          </defs>

          <CartesianGrid vertical={false} stroke="var(--rule)" />
          <XAxis
            dataKey="distanceMetres"
            type="number"
            domain={[profile.startMetres, profile.endMetres]}
            ticks={geometry.distanceTicks}
            tickFormatter={(metres: number) =>
              distanceLabel(metres, geometry.distanceStep, unitSystem)
            }
            tick={{ fill: "var(--ink-2)", fontSize: 12 }}
            tickLine={false}
            axisLine={false}
            height={PADDING.bottom}
          />
          <YAxis
            type="number"
            domain={[geometry.low, geometry.high]}
            ticks={geometry.elevationTicks}
            tickFormatter={(metres: number) =>
              String(Math.round(elevationValue(metres, unitSystem)))
            }
            tick={{ fill: "var(--ink-2)", fontSize: 12 }}
            tickLine={false}
            axisLine={false}
            width={PADDING.left}
            // Recharts thins explicit ticks it thinks will collide; the axis was
            // asked for three and must draw three.
            interval={0}
          />

          <Area
            dataKey="elevationMetres"
            type="linear"
            fill={`url(#${gradientId})`}
            fillOpacity={0.5}
            stroke={`url(#${gradientId})`}
            strokeWidth={2.4}
            isAnimationActive={false}
            // The cursor below is ours, and shared with the map; Recharts' own
            // would be a second one nothing else knows about.
            activeDot={false}
            dot={false}
            /*
             * A forced palette repaints everything in two colours, which here
             * would replace the encoding with a flat block: the colour of a
             * stretch *is* which band it is. This is one of the few marks whose
             * colour carries the information rather than decorating it, so it
             * keeps its own.
             */
            style={{ forcedColorAdjust: "none" }}
          />

          {/*
           * Everything the picked class does not cover, veiled. The marks keep
           * the colour that gives them their meaning — brightening a band would
           * change the very thing being asked about — so what the selection
           * changes is the light on the rest.
           */}
          {veiled.map((gap) => (
            <ReferenceArea
              key={gap.startMetres}
              x1={gap.startMetres}
              x2={gap.endMetres}
              fill="var(--panel)"
              fillOpacity={0.6}
              data-veil=""
              ifOverflow="hidden"
              data-testid="profile-veil"
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
            <>
              <ReferenceArea
                x1={profile.startMetres}
                x2={selection.startMetres}
                fill="var(--panel)"
                fillOpacity={0.6}
                data-veil=""
                data-testid="profile-veil"
              />
              <ReferenceArea
                x1={selection.endMetres}
                x2={profile.endMetres}
                fill="var(--panel)"
                fillOpacity={0.6}
                data-veil=""
                data-testid="profile-veil"
              />
              {[selection.startMetres, selection.endMetres].map((metres) => (
                <ReferenceLine
                  key={metres}
                  x={metres}
                  stroke="var(--accent)"
                  strokeWidth={1.5}
                  strokeDasharray="3 3"
                />
              ))}
            </>
          ) : null}

          {active ? (
            <>
              <ReferenceLine x={active.distanceMetres} stroke="var(--ink-2)" strokeWidth={1} />
              <ReferenceDot
                x={active.distanceMetres}
                y={active.elevationMetres}
                r={4}
                fill={BAND_COLOURS[active.band] ?? BAND_COLOURS[0]}
                stroke="var(--panel)"
                strokeWidth={2}
                // The marker carries the band of the position it marks, so its
                // colour is information too — exempted for the same reason the
                // terrain is.
                style={{ forcedColorAdjust: "none" }}
              />
            </>
          ) : null}
        </AreaChart>
      </div>

      {/*
       * The scrubbing surface is a slider rather than a decorated graphic: it
       * genuinely picks a position along the route, so that role gives keyboard
       * users arrow-key stepping and screen readers the value at each step,
       * which a non-interactive chart cannot carry. The drag that zooms is a
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
