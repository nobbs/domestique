/**
 * What every wide-panel alternative is handed, and the furniture they share.
 *
 * The wide panel is the second half of the route-panel decision: the pill on
 * the map answers whether this is the ride, and everything taken out of it to
 * make that possible — the elevation profile, the forecast, the climbs in
 * full — has to land somewhere with room. This is that somewhere, sketched as
 * a sheet standing on the foot of the map the way Komoot's does.
 *
 * All four are given the same day and the same instruments. What differs is
 * how many of them can be seen at once, and against what axis.
 */

import type { ReactNode } from "react";
import type { Route } from "../../../api/types";
import type { Cell } from "../../../components/route/forecast-spike/cells";
import type { Climb } from "../../../lib/climbs";
import type { ForecastSample } from "../../../lib/forecastSamples";
import {
  formatAscent,
  formatDistance,
  formatElevation,
  formatGradient,
  formatMovingTime,
} from "../../../lib/format";
import type { Highlight } from "../../../lib/highlight";
import type { BandShare, Profile } from "../../../lib/profile";
import type { SurfaceSummary } from "../../../lib/surface";
import type { UnitSystem } from "../../../lib/units";

export interface SheetProps {
  route: Route;
  profile: Profile | null;
  surface: SurfaceSummary | null;
  /** Steepness in ride order, for anything drawing a ribbon. */
  runs: BandShare[];
  bands: BandShare[];
  climbs: Climb[];
  /** The day, already folded into one cell per reading. */
  cells: Cell[];
  samples: ForecastSample[];
  startAt: Date;
  /** The position shared with the map, in metres from the start. */
  activeMetres: number | null;
  onActiveChange: (metres: number | null) => void;
  highlight: Highlight | null;
  onHighlightChange: (highlight: Highlight | null) => void;
  unitSystem: UnitSystem;
}

/**
 * The sheet itself: a card standing on the foot of the map.
 *
 * Rounded and inset on all sides rather than run to the edges, so the map is
 * visibly continuous behind it and the panel reads as something laid on the
 * page rather than a region of it.
 */
export function Sheet({ children }: { children: ReactNode }) {
  return (
    <section
      aria-label="Route detail"
      className="rounded-xl bg-[var(--panel)] p-4 shadow-[var(--shadow)] ring-1 ring-black/5"
    >
      {children}
    </section>
  );
}

/** One measured fact, in the row every alternative leads with. */
export function Figure({ term, value }: { term: string; value: string }) {
  return (
    <div className="whitespace-nowrap">
      <span className="text-sm font-semibold tabular-nums">{value}</span>{" "}
      <span className="text-xs text-[var(--ink-2)]">{term}</span>
    </div>
  );
}

export function Figures({
  route,
  highestMetres,
  unitSystem,
}: {
  route: Route;
  highestMetres: number | null;
  unitSystem: UnitSystem;
}) {
  return (
    <div className="flex flex-wrap items-baseline gap-x-5 gap-y-1">
      <Figure term="" value={formatDistance(route.distanceMetres, unitSystem)} />
      <Figure term="moving" value={formatMovingTime(route.movingSeconds)} />
      <Figure term="up" value={formatAscent(route.ascentMetres, unitSystem)} />
      <Figure term="max" value={formatGradient(route.maxGradientPercent)} />
      <Figure
        term="high"
        value={highestMetres === null ? "—" : formatElevation(highestMetres, unitSystem)}
      />
    </div>
  );
}

/** `14:20`, in the reader's own zone, which is where they will be riding. */
export function clockAt(at: Date): string {
  return at.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}
