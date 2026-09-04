/**
 * Five ways the profile stop could fold its climbs away without the vertical
 * tab the user dislikes.
 *
 * Each variant mocks the dock's profile stop on its own: the real
 * `ElevationProfile` and `ClimbMarkers` on the left, the real `ClimbsSidebar`
 * on the right when open, and a variant-drawn control for the folded state.
 * Storybook only: nothing here is imported by the app.
 */

import { Tooltip } from "@base-ui/react/tooltip";
import {
  IconChevronLeft,
  IconChevronRight,
  IconCloud,
  IconLayoutBottombarCollapse,
  IconMountain,
  IconStairs,
} from "@tabler/icons-react";
import { type ReactNode, useState } from "react";
import type { Climb } from "../../../lib/climbs";
import { PADDING } from "../../../lib/plotAxis";
import { climbs, profile, route } from "../../../storybook/fixtures";
import { ClimbMarkers } from "../ClimbMarkers";
import { ClimbsSidebar, climbSentence } from "../ClimbsSidebar";
import { ElevationProfile } from "../ElevationProfile";

const UNITS = "metric";

/** Enough climbs to make the list scroll, which one fixture climb cannot. */
const MANY_CLIMBS = climbs.flatMap((climb) =>
  Array.from({ length: 9 }, (_, i) => {
    const startMetres = 1_500 + i * 4_200;
    return { ...climb, startMetres, endMetres: startMetres + climb.distanceMetres };
  }),
);

const COUNT = `${MANY_CLIMBS.length} climbs`;

const CONTROL =
  "text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)]";

/** The fixed-height stop panel every variant folds inside. */
function Panel({ children }: { children: ReactNode }) {
  return (
    <div className="relative flex h-52 items-stretch gap-3 rounded-xl bg-[var(--panel)] p-4 shadow-[var(--shadow)] ring-1 ring-black/5">
      {children}
    </div>
  );
}

/** The chart half every variant shares: the profile with its climb brackets. */
function Chart({
  activeMetres,
  onActiveChange,
  onMarkerSelect,
}: {
  activeMetres: number | null;
  onActiveChange: (metres: number | null) => void;
  /** Overrides what a bracket does; defaults to scrubbing the chart. */
  onMarkerSelect?: (metres: number) => void;
}) {
  return (
    <div className="relative min-w-0 flex-1">
      <ElevationProfile
        profile={profile}
        title={route.title}
        activeMetres={activeMetres}
        onActiveChange={onActiveChange}
        unitSystem={UNITS}
        caption={false}
      />
      <ClimbMarkers
        climbs={MANY_CLIMBS}
        totalMetres={route.distanceMetres}
        onSelect={onMarkerSelect ?? onActiveChange}
      />
    </div>
  );
}

function useChartState() {
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  return {
    activeMetres,
    onActiveChange: setActiveMetres,
    // Matches the real dock's own onSelectClimb: scrub to the climb's middle.
    onSelectClimb: (climb: Climb) => setActiveMetres((climb.startMetres + climb.endMetres) / 2),
  };
}

/** A · Count chip — folded, a small pill in the panel's corner; no column at all. */
export function CountChipFold() {
  const [open, setOpen] = useState(false);
  const chart = useChartState();

  return (
    <Panel>
      <div className="relative min-w-0 flex-1">
        <Chart {...chart} />
        {open ? null : (
          <button
            type="button"
            aria-expanded={false}
            aria-label={`Show ${COUNT}`}
            onClick={() => setOpen(true)}
            style={{ top: PADDING.top - 6 }}
            className={`absolute right-0 flex items-center gap-1 rounded-full border border-[var(--rule)] bg-[var(--panel)] px-2 py-0.5 text-[11px] ${CONTROL}`}
          >
            <IconStairs size={13} stroke={2} aria-hidden="true" />
            {COUNT}
          </button>
        )}
      </div>
      {open ? (
        <ClimbsSidebar
          climbs={MANY_CLIMBS}
          open
          onOpenChange={setOpen}
          onSelect={chart.onSelectClimb}
          unitSystem={UNITS}
          fixedHeight
        />
      ) : null}
    </Panel>
  );
}

/** B · Header only — folded, the header row survives and shrinks to fit; the table does not. */
export function HeaderOnlyFold() {
  const [open, setOpen] = useState(false);
  const chart = useChartState();
  const summary = climbSentence(MANY_CLIMBS, UNITS);

  return (
    <Panel>
      <Chart {...chart} />
      {open ? (
        <ClimbsSidebar
          climbs={MANY_CLIMBS}
          open
          onOpenChange={setOpen}
          onSelect={chart.onSelectClimb}
          unitSystem={UNITS}
          fixedHeight
        />
      ) : (
        <section className="w-40 shrink-0 self-stretch border-l border-[var(--rule)] pl-3">
          <h3>
            <button
              type="button"
              aria-expanded={false}
              aria-label={`Show ${COUNT}`}
              onClick={() => setOpen(true)}
              className={`flex w-full items-baseline gap-2 rounded py-0.5 text-left ${CONTROL}`}
            >
              <span className="text-[10px] font-semibold tracking-[0.08em] text-[var(--ink-2)] uppercase">
                {COUNT}
              </span>
              <span className="min-w-0 flex-1 truncate text-[11px] text-[var(--ink-2)]">
                {summary}
              </span>
              <IconChevronRight size={12} stroke={2} aria-hidden="true" className="shrink-0" />
            </button>
          </h3>
        </section>
      )}
    </Panel>
  );
}

const RAIL_ITEM =
  "flex w-14 flex-col items-center gap-0.5 rounded-md px-1 py-1.5 text-[10px] leading-none aria-pressed:bg-[var(--base)] aria-pressed:font-semibold aria-pressed:text-[var(--ink)]";

/** C · Rail item — folded, nothing beside the chart; a third rail item opens and closes it. */
export function RailItemFold() {
  const [open, setOpen] = useState(false);
  const chart = useChartState();

  return (
    <Panel>
      <div className="flex shrink-0 flex-col border-r border-[var(--rule)] pr-2">
        <div className="flex flex-col gap-0.5" aria-hidden="true">
          <div className={`${RAIL_ITEM} text-[var(--ink)]`}>
            <IconMountain size={15} stroke={2} aria-hidden="true" />
            Profile
          </div>
          <div className={`${RAIL_ITEM} text-[var(--ink-2)]`}>
            <IconCloud size={15} stroke={2} aria-hidden="true" />
            Forecast
          </div>
        </div>
        <button
          type="button"
          aria-pressed={open}
          aria-label={open ? `Hide ${COUNT}` : `Show ${COUNT}`}
          onClick={() => setOpen((was) => !was)}
          className={`${RAIL_ITEM} ${CONTROL}`}
        >
          <IconStairs size={15} stroke={2} aria-hidden="true" />
          Climbs
        </button>
        <div className={`${RAIL_ITEM} mt-auto text-[var(--ink-2)]`} aria-hidden="true">
          <IconLayoutBottombarCollapse size={15} stroke={2} aria-hidden="true" />
          Hide
        </div>
      </div>
      <Chart {...chart} />
      {open ? (
        <ClimbsSidebar
          climbs={MANY_CLIMBS}
          open
          onOpenChange={setOpen}
          onSelect={chart.onSelectClimb}
          unitSystem={UNITS}
          fixedHeight
        />
      ) : null}
    </Panel>
  );
}

/** D · Brackets only — folded, no control besides the brackets themselves and a tiny text link. */
export function BracketsOnlyFold() {
  const [open, setOpen] = useState(false);
  const chart = useChartState();

  return (
    <Panel>
      <div className="relative min-w-0 flex-1">
        <Chart
          {...chart}
          onMarkerSelect={(metres) => {
            chart.onActiveChange(metres);
            setOpen(true);
          }}
        />
        {open ? null : (
          <button
            type="button"
            aria-expanded={false}
            aria-label={`Show ${COUNT}`}
            onClick={() => setOpen(true)}
            style={{ top: PADDING.top - 2 }}
            className={`absolute right-0 text-[11px] underline decoration-dotted underline-offset-2 ${CONTROL}`}
          >
            {COUNT}
          </button>
        )}
      </div>
      {open ? (
        <ClimbsSidebar
          climbs={MANY_CLIMBS}
          open
          onOpenChange={setOpen}
          onSelect={chart.onSelectClimb}
          unitSystem={UNITS}
          fixedHeight
        />
      ) : null}
    </Panel>
  );
}

/** E · Edge handle — folded, a slim full-height strip with only a chevron; the count is a tooltip. */
export function EdgeHandleFold() {
  const [open, setOpen] = useState(false);
  const chart = useChartState();

  return (
    <Panel>
      <Chart {...chart} />
      {open ? (
        <ClimbsSidebar
          climbs={MANY_CLIMBS}
          open
          onOpenChange={setOpen}
          onSelect={chart.onSelectClimb}
          unitSystem={UNITS}
          fixedHeight
        />
      ) : (
        <Tooltip.Provider>
          <Tooltip.Root>
            <Tooltip.Trigger
              type="button"
              aria-expanded={false}
              aria-label={`Show ${COUNT}`}
              title={COUNT}
              onClick={() => setOpen(true)}
              className={`flex w-5 shrink-0 items-center justify-center self-stretch rounded-lg border border-[var(--rule)] ${CONTROL}`}
            >
              <IconChevronLeft size={14} stroke={2} aria-hidden="true" />
            </Tooltip.Trigger>
            <Tooltip.Portal>
              <Tooltip.Positioner sideOffset={6}>
                <Tooltip.Popup
                  role="tooltip"
                  className="rounded-md bg-[var(--ink)] px-2 py-1 text-[11px] text-[var(--panel)] shadow-[var(--shadow)]"
                >
                  {COUNT}
                </Tooltip.Popup>
              </Tooltip.Positioner>
            </Tooltip.Portal>
          </Tooltip.Root>
        </Tooltip.Provider>
      )}
    </Panel>
  );
}
