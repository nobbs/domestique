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

import { IconChevronsRight } from "@tabler/icons-react";
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
 * The sheet itself: a card standing on the foot of the map, or a pill when it
 * has been put away.
 *
 * Rounded and inset on all sides rather than run to the edges, so the map is
 * visibly continuous behind it and the panel reads as something laid on the
 * page rather than a region of it.
 *
 * Folded, it gives back its whole height — around a third of the map — and
 * leaves a pill centred on the foot. Centred rather than in a corner because
 * the dock is the full width of the page: it belongs to the middle, and a
 * reader who put it away looks for it where it went, not where a corner
 * control happens to live.
 */
export function Sheet({
  open = true,
  onOpenChange,
  summary,
  children,
}: {
  /** Whether the dock is unfolded. Folded, it is a pill on the map's foot. */
  open?: boolean | undefined;
  onOpenChange?: ((open: boolean) => void) | undefined;
  /** What the pill says while the dock is away. */
  summary?: string | undefined;
  children: ReactNode;
}) {
  if (onOpenChange !== undefined && !open) {
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
        {summary ?? "Route detail"}
      </button>
    );
  }

  return (
    <section
      aria-label="Route detail"
      className="relative w-full rounded-xl bg-[var(--panel)] p-4 shadow-[var(--shadow)] ring-1 ring-black/5"
    >
      {onOpenChange === undefined ? null : (
        // On the top edge, centred: that edge is the seam the dock folds
        // along, and it is where the pill will be. The control does not move
        // when the thing it controls goes away — the same rule the climbs
        // divider follows one panel over.
        //
        // Shaped like the drawer's own swipe handle, which is the vocabulary
        // this application already uses for the top edge of a sheet: a wide
        // flat pill rather than the dot it was, so it reads as the sheet's
        // handle rather than as a stray control that happens to sit there.
        // The chevron stays, though — the drawer's bar promises a drag, and
        // this only ever takes a press, so it says which way the sheet goes
        // instead of inviting one.
        <button
          type="button"
          aria-expanded
          aria-label="Hide the route detail"
          onClick={() => onOpenChange(false)}
          className="absolute -top-3 left-1/2 flex h-6 w-14 -translate-x-1/2 items-center justify-center rounded-full border border-[var(--rule)] bg-[var(--panel)] text-[var(--ink-2)] shadow-[var(--shadow)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)]"
        >
          <IconChevronsRight size={15} stroke={2} aria-hidden="true" className="rotate-90" />
        </button>
      )}
      {children}
    </section>
  );
}

/**
 * The reading nearest a point on the route, which is the weather met there.
 *
 * The join two of these alternatives are built on. A climb has a distance and
 * the forecast has a distance, so "col two, half past eleven, twenty degrees
 * and clouding over" is a sentence this service can already assemble and
 * currently never does.
 */
export function cellAt(cells: Cell[], metres: number): Cell | null {
  return cells.reduce<Cell | null>((nearest, cell) => {
    if (nearest === null) {
      return cell;
    }

    return Math.abs(cell.sample.distanceMetres - metres) <
      Math.abs(nearest.sample.distanceMetres - metres)
      ? cell
      : nearest;
  }, null);
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
