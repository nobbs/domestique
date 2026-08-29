/**
 * The route, drawn against distance, along the foot of the map.
 *
 * Almost everything this service knows about a route is a function of one
 * variable: how far along it you are. Height is, steepness is, the ground is,
 * and — once a start time is chosen — so is the weather, because when you
 * arrive somewhere is decided by how far away it is. Plotted against separate
 * axes those are readings a reader has to align by hand; plotted against the
 * same one they align themselves, and the dock can say the thing no single
 * instrument does: *the rain arrives on the second col, and the col is gravel*.
 *
 * It costs height, which is why it folds. Away, it leaves a pill centred on the
 * map's foot — centred because the dock is the full width of the page and
 * belongs to the middle, and because a reader who put it away looks for it
 * where it went.
 *
 * The route panel says what the route *is*; this says what it *does*, and the
 * two do not repeat each other. The panel totals each mix per class — thirteen
 * kilometres of gravel, which decides the bike — and this keeps the order it is
 * ridden in — the gravel falling on the second col, which decides the day.
 * `gradientShares` and `gradientMix` are separate functions for exactly that.
 */

import { IconChevronsRight } from "@tabler/icons-react";
import type { Position } from "../../api/types";
import { StartTimePicker } from "../../components/StartTimePicker";
import type { Climb } from "../../lib/climbs";
import type { ForecastSample } from "../../lib/forecastSamples";
import type { Highlight } from "../../lib/highlight";
import { groundSegments } from "../../lib/mix";
import { PADDING } from "../../lib/plotAxis";
import type { DistanceWindow, Profile } from "../../lib/profile";
import type { SurfaceSummary } from "../../lib/surface";
import type { UnitSystem } from "../../lib/units";
import { ClimbMarkers } from "./ClimbMarkers";
import { ElevationProfile } from "./ElevationProfile";
import { ForecastFrame } from "./ForecastFrame";
import { ForecastStrip } from "./ForecastStrip";
import { GroundRibbon } from "./GroundRibbon";

/** `14:20`, in the reader's own zone, which is where they will be riding. */
function clockAt(at: Date): string {
  return at.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}

export interface RouteDockProps {
  title: string;
  /** The stretch on show: the whole route, or the window the reader chose. */
  profile: Profile | null;
  /** The whole route's length, which the ribbon and the strip are laid along. */
  distanceMetres: number;
  surface: SurfaceSummary | null;
  climbs: Climb[];
  /** The route's own geometry, which the strip's wind reading is measured against. */
  coordinates: Position[];
  samples: ForecastSample[];
  startAt: Date | null;
  onStartAtChange: (next: Date | null) => void;
  /** The ride's predicted moving time, which the departure's horizon depends on. */
  movingSeconds?: number | undefined;
  activeMetres: number | null;
  onActiveChange: (metres: number | null) => void;
  zoomWindow: DistanceWindow | null;
  onZoomChange: (window: DistanceWindow | null) => void;
  highlight: Highlight | null;
  onHighlightChange: (highlight: Highlight | null) => void;
  unitSystem: UnitSystem;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Whether the ground key beneath the ribbon is shown. */
  groundLabelled: boolean;
  onGroundLabelledChange: (labelled: boolean) => void;
  forecastOpen: boolean;
  onForecastOpenChange: (open: boolean) => void;
}

export function RouteDock({
  title,
  profile,
  distanceMetres,
  surface,
  climbs,
  coordinates,
  samples,
  startAt,
  onStartAtChange,
  movingSeconds,
  activeMetres,
  onActiveChange,
  zoomWindow,
  onZoomChange,
  highlight,
  onHighlightChange,
  unitSystem,
  open,
  onOpenChange,
  groundLabelled,
  onGroundLabelledChange,
  forecastOpen,
  onForecastOpenChange,
}: RouteDockProps) {
  const back = samples[samples.length - 1]?.arrivalAt;
  const shown = zoomWindow ?? { startMetres: 0, endMetres: distanceMetres };

  if (!open) {
    return (
      <button
        type="button"
        aria-expanded={false}
        onClick={() => onOpenChange(true)}
        className="flex items-center gap-1.5 rounded-full bg-[var(--panel)] py-1.5 pr-3.5 pl-3 text-xs shadow-[var(--shadow)] ring-1 ring-black/5 hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
      >
        <IconChevronsRight
          size={13}
          stroke={2}
          aria-hidden="true"
          className="-rotate-90 text-[var(--ink-2)]"
        />
        Profile, ground and forecast
        {back === undefined ? null : ` · back ${clockAt(back)}`}
      </button>
    );
  }

  return (
    <section
      aria-label="Route detail"
      className="relative w-full rounded-xl bg-[var(--panel)] p-4 shadow-[var(--shadow)] ring-1 ring-black/5"
    >
      {/*
       * On the top edge, centred: that edge is the seam the dock folds along,
       * and it is where the pill will be. The control does not move when the
       * thing it controls goes away. Shaped like the drawer's own swipe handle,
       * which is the vocabulary this application already uses for the top edge
       * of a sheet — with a chevron, because a bare bar promises a drag and
       * this only ever takes a press.
       */}
      <button
        type="button"
        aria-expanded
        aria-label="Hide the route detail"
        onClick={() => onOpenChange(false)}
        className="absolute -top-3 left-1/2 flex h-6 w-14 -translate-x-1/2 items-center justify-center rounded-full border border-[var(--rule)] bg-[var(--panel)] text-[var(--ink-2)] shadow-[var(--shadow)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)]"
      >
        <IconChevronsRight size={15} stroke={2} aria-hidden="true" className="rotate-90" />
      </button>
      <div className="grid gap-1.5">
        <div className="relative">
          <ElevationProfile
            profile={profile}
            title={title}
            surface={surface}
            activeMetres={activeMetres}
            onActiveChange={onActiveChange}
            zoomWindow={zoomWindow}
            onZoomChange={onZoomChange}
            highlight={highlight}
            unitSystem={unitSystem}
          />
          <ClimbMarkers climbs={climbs} totalMetres={distanceMetres} onSelect={onActiveChange} />
        </div>
        {/*
         * Ground only. The chart above already paints the area under it by
         * steepness band, so a gradient ribbon here would be the same fact
         * drawn twice one row apart — which reads as two measurements until you
         * work out that it is not.
         */}
        <div className="relative">
          <button
            type="button"
            aria-expanded={groundLabelled}
            aria-label={groundLabelled ? "Hide the ground key" : "Show the ground key"}
            onClick={() => onGroundLabelledChange(!groundLabelled)}
            className="absolute top-0 left-0 rounded p-0.5 text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)]"
          >
            <IconChevronsRight
              size={12}
              stroke={2}
              aria-hidden="true"
              className={groundLabelled ? "rotate-90 transition-transform" : "transition-transform"}
            />
          </button>
          <div style={{ paddingLeft: PADDING.left, paddingRight: PADDING.right }}>
            <GroundRibbon
              segments={groundSegments(surface)}
              surface={surface}
              labelled={groundLabelled}
              highlight={highlight}
              onHighlightChange={onHighlightChange}
            />
          </div>
        </div>
        <ForecastFrame
          caption={`Forecast${back === undefined ? "" : ` · back ${clockAt(back)}`}`}
          open={forecastOpen}
          onOpenChange={onForecastOpenChange}
          controls={
            <StartTimePicker
              value={startAt}
              onChange={onStartAtChange}
              movingSeconds={movingSeconds}
              inline
            />
          }
        >
          <ForecastStrip
            samples={samples}
            coordinates={coordinates}
            startMetres={shown.startMetres}
            endMetres={shown.endMetres}
            unitSystem={unitSystem}
          />
        </ForecastFrame>
      </div>
    </section>
  );
}
