/**
 * The three decisions that only make sense together.
 *
 * A card whose height barely moves, a dock that therefore spans the whole
 * width, and the climbs table living in that dock as a sidebar rather than in
 * the card. Each is arguable alone and none of them is judgeable alone: the
 * card can only be short if the climbs leave, the dock can only be full width
 * if the card is short, and the sidebar only has room if the dock is wide.
 *
 * The chart, the ribbon and the climb markers are the real components on a
 * generated route, because what is being judged is how much room they get.
 * The map underneath is a stand-in — a panel reads differently floating over
 * ground than it does on a page, and a flat white background would flatter
 * every one of these.
 */

import { IconChevronRight, IconLayoutSidebarRightCollapse } from "@tabler/icons-react";
import { useMemo, useState } from "react";
import { ClimbMarkers } from "../../../components/route/ClimbMarkers";
import { ElevationProfile } from "../../../components/route/ElevationProfile";
import { GroundRibbon } from "../../../components/route/GroundRibbon";
import type { Climb } from "../../../lib/climbs";
import type { Highlight } from "../../../lib/highlight";
import { groundSegments } from "../../../lib/mix";
import { buildProfile } from "../../../lib/profile";
import { CLIMBS, SPIKE_COORDINATES, SPIKE_DISTANCE_METRES, SPIKE_SURFACE, TITLE } from "./data";
import { AirCard, SlideCard } from "./variants";

/** Where the spike's climbs sit along the route, for the markers over the chart. */
const MARKED: Climb[] = [
  {
    startMetres: 653,
    endMetres: 8_353,
    distanceMetres: 7_700,
    ascentMetres: 898,
    averageGradePercent: 12,
    maxGradePercent: 18,
  },
  {
    startMetres: 20_200,
    endMetres: 29_800,
    distanceMetres: 9_600,
    ascentMetres: 894,
    averageGradePercent: 9.3,
    maxGradePercent: 11,
  },
  {
    startMetres: 40_700,
    endMetres: 48_500,
    distanceMetres: 7_800,
    ascentMetres: 884,
    averageGradePercent: 11,
    maxGradePercent: 19,
  },
];

/**
 * The climbs, in the dock, as a column beside the chart rather than a block
 * under it.
 *
 * Beside is the whole argument. Under, the table competes with the profile for
 * the dock's height and pushes the forecast off the bottom; beside, it costs
 * width the full-width dock has and height it does not. Folded it keeps its
 * summary, so the count is still readable without opening anything.
 */
function ClimbsSidebar({
  open,
  onOpenChange,
  onSelect,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (metres: number) => void;
}) {
  if (!open) {
    return (
      <button
        type="button"
        onClick={() => onOpenChange(true)}
        className="flex shrink-0 items-center gap-1.5 self-stretch rounded-lg border border-[var(--rule)] px-2 text-[11px] text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)]"
      >
        <IconLayoutSidebarRightCollapse size={14} stroke={2} aria-hidden="true" />
        <span className="[writing-mode:vertical-rl] rotate-180">3 climbs</span>
      </button>
    );
  }

  return (
    <section className="w-80 shrink-0 self-stretch border-l border-[var(--rule)] pl-3">
      <button
        type="button"
        onClick={() => onOpenChange(false)}
        className="flex w-full items-baseline gap-2 rounded py-0.5 text-left text-xs hover:bg-[var(--base)]"
      >
        <span className="text-[10px] font-semibold tracking-[0.08em] text-[var(--ink-2)] uppercase">
          Climbs
        </span>
        <span className="min-w-0 flex-1 truncate text-[11px] text-[var(--ink-2)]">
          biggest 7.7 km at 12%
        </span>
        <IconChevronRight
          size={12}
          stroke={2}
          aria-hidden="true"
          className="shrink-0 rotate-90 text-[var(--ink-2)]"
        />
      </button>
      <div className="mt-1 text-[11px] tabular-nums">
        <div
          className="grid gap-2 px-1.5 pb-1 text-[10px] text-[var(--ink-2)]"
          style={{ gridTemplateColumns: "0.75rem 3.5rem 2.5rem 2.5rem 3.5rem minmax(0,1fr)" }}
        >
          <span />
          <span className="text-right">Length</span>
          <span className="text-right">Avg</span>
          <span className="text-right">Max</span>
          <span className="text-right">Ascent</span>
          <span className="text-right">Starts</span>
        </div>
        {CLIMBS.map((climb, index) => (
          <button
            key={climb.ordinal}
            type="button"
            onClick={() => onSelect(MARKED[index]?.startMetres ?? 0)}
            className="grid w-full gap-2 rounded px-1.5 py-1 text-left hover:bg-[var(--base)]"
            style={{ gridTemplateColumns: "0.75rem 3.5rem 2.5rem 2.5rem 3.5rem minmax(0,1fr)" }}
          >
            <span className="text-[var(--ink-2)]">{climb.ordinal}</span>
            <span className="text-right">{climb.length}</span>
            <span className="text-right">{climb.average}</span>
            <span className="text-right">{climb.steepest}</span>
            <span className="text-right">{climb.ascent}</span>
            <span className="text-right text-[var(--ink-2)]">{climb.starts}</span>
          </button>
        ))}
      </div>
    </section>
  );
}

export function Workspace({ card }: { card: "slide" | "fold" }) {
  const [highlight, setHighlight] = useState<Highlight | null>(null);
  const [active, setActive] = useState<number | null>(null);
  const [sidebar, setSidebar] = useState(true);
  const profile = useMemo(() => buildProfile(SPIKE_COORDINATES), []);
  const Card = card === "slide" ? SlideCard : AirCard;

  return (
    // The map's stand-in: enough tone that a floating panel reads as floating.
    <div className="relative h-[46rem] overflow-hidden bg-[var(--base)]">
      <div
        aria-hidden="true"
        className="absolute inset-0 opacity-60"
        style={{
          backgroundImage:
            "linear-gradient(var(--rule) 1px, transparent 1px), linear-gradient(90deg, var(--rule) 1px, transparent 1px)",
          backgroundSize: "48px 48px",
        }}
      />
      <div className="absolute top-3 left-3">
        <Card withClimbs={false} highlight={highlight} onHighlightChange={setHighlight} />
      </div>
      {/*
       * Edge to edge, which is the point: the card no longer reaches down here,
       * so nothing has to be left clear on the left.
       */}
      <div className="absolute inset-x-3 bottom-3 rounded-xl bg-[var(--panel)] p-3 shadow-[var(--shadow)] ring-1 ring-black/5">
        <div className="flex items-stretch gap-3">
          <div className="min-w-0 flex-1">
            <p className="pb-1 text-xs text-[var(--ink-2)]">
              180 m–1,090 m · drag across to look closer
            </p>
            <div className="relative">
              <ElevationProfile
                profile={profile}
                title={TITLE}
                surface={SPIKE_SURFACE}
                activeMetres={active}
                onActiveChange={setActive}
                highlight={highlight}
                unitSystem="metric"
              />
              <ClimbMarkers
                climbs={MARKED}
                totalMetres={SPIKE_DISTANCE_METRES}
                onSelect={setActive}
              />
            </div>
            <div className="pt-2">
              <GroundRibbon
                segments={groundSegments(SPIKE_SURFACE)}
                surface={SPIKE_SURFACE}
                highlight={highlight}
                onHighlightChange={setHighlight}
              />
            </div>
          </div>
          <ClimbsSidebar open={sidebar} onOpenChange={setSidebar} onSelect={setActive} />
        </div>
      </div>
    </div>
  );
}
