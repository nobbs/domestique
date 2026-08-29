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

/** `14:20`, in the reader's own zone, which is where they will be riding. */
export function clockAt(at: Date): string {
  return at.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}

/**
 * When the ride sets off and when it is back.
 *
 * What is left of the header once the figures go. The route panel above is
 * already carrying how far, how much climbing and how long it takes, and a
 * sheet that repeated them would be spending its first row on a second copy
 * of the card two hundred pixels up.
 *
 * The window is not one of those figures. It is the frame the forecast is
 * drawn in — the reason a tile sits where it does — and the finish time is a
 * thing the panel above cannot say, because it only knows a duration and this
 * knows when the duration starts.
 */
export function RideWindow({ startAt, samples }: { startAt: Date; samples: ForecastSample[] }) {
  const back = samples[samples.length - 1]?.arrivalAt;

  return (
    <p className="text-xs text-[var(--ink-2)] tabular-nums">
      Setting off {clockAt(startAt)}
      {back === undefined ? null : ` · back ${clockAt(back)}`}
    </p>
  );
}
