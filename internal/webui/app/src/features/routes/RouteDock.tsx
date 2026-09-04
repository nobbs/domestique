/**
 * The route, drawn against distance, along the foot of the map: a profile
 * stop and a forecast stop on one rail, folding to a pill when put away.
 */

import { Tabs } from "@base-ui/react/tabs";
import {
  IconArrowDownRight,
  IconArrowUpRight,
  IconCloud,
  IconInfoCircle,
  IconLayoutBottombarCollapse,
  IconMountain,
} from "@tabler/icons-react";
import type { ReactNode } from "react";
import { useState } from "react";
import type { Position } from "../../api/types";
import { StartTimePicker } from "../../components/StartTimePicker";
import { Popover, PopoverContent, PopoverTrigger } from "../../components/ui/popover";
import type { Climb } from "../../lib/climbs";
import { forecastResolution } from "../../lib/forecastResolution";
import { type ForecastSample, forecastLeadHours } from "../../lib/forecastSamples";
import { formatAscent, formatDistance, formatElevation } from "../../lib/format";
import type { Highlight } from "../../lib/highlight";
import type { MeasureKey } from "../../lib/measures";
import { useCoarsePointer } from "../../lib/mediaQuery";
import { groundSegments, steepnessEntries } from "../../lib/mix";
import { PADDING } from "../../lib/plotAxis";
import type { DistanceWindow, Profile } from "../../lib/profile";
import { gradientSharesBySign } from "../../lib/profile";
import type { SurfaceSummary } from "../../lib/surface";
import type { UnitSystem } from "../../lib/units";
import { ClimbMarkers } from "./ClimbMarkers";
import { ClimbsSidebar } from "./ClimbsSidebar";
import { ConditionsChoices, ConditionsKey } from "./ConditionsPicker";
import { ElevationProfile, profileReadout } from "./ElevationProfile";
import { ForecastStrip } from "./ForecastStrip";
import { GroundRibbon } from "./GroundRibbon";

/** `14:20`, in the reader's own zone, which is where they will be riding. */
function clockAt(at: Date): string {
  return at.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}

const GUTTER = { paddingLeft: PADDING.left, paddingRight: PADDING.right };

/**
 * The frame every stop shares: one line of figures across the top, a control
 * at its end where the stop has one, and the body beneath. The line is data
 * rather than a heading — a reader on the profile stop already knows it is the
 * profile; what they do not know is how high it goes.
 */
function Panel({
  line,
  lineLabel,
  control,
  info,
  gutter = true,
  children,
}: {
  line: string;
  /** Names the line for assistive tech and the e2e suite, where a plain readout is not enough. */
  lineLabel?: string;
  control?: ReactNode;
  /** What waits behind the ⓘ at the line's end: the figures a glance does not need. */
  info?: ReactNode;
  /** Whether the line sits over the chart's plotted area; off where there is no axis. */
  gutter?: boolean;
  children: ReactNode;
}) {
  const [lead, ...tail] = line.split(" · ");
  const rest = tail.length === 0 ? undefined : tail.join(" · ");

  return (
    <div className="grid gap-1.5">
      <div
        className="flex min-h-7 flex-wrap items-center justify-between gap-x-3 gap-y-1"
        style={gutter ? GUTTER : undefined}
      >
        <output aria-label={lineLabel} className="text-xs text-[var(--ink-2)] tabular-nums">
          <span className="text-sm font-semibold text-[var(--ink)]">{lead}</span>
          {rest === undefined ? null : ` · ${rest}`}
        </output>
        <div className="flex items-center gap-3">
          {control}
          {info === undefined ? null : (
            <Popover>
              <PopoverTrigger
                openOnHover
                delay={150}
                aria-label="More about this"
                className="rounded-full p-0.5 text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)] data-[popup-open]:text-[var(--ink)]"
              >
                <IconInfoCircle size={16} stroke={1.8} aria-hidden="true" />
              </PopoverTrigger>
              <PopoverContent align="end" className="w-auto p-3">
                {info}
              </PopoverContent>
            </Popover>
          )}
        </div>
      </div>
      {children}
    </div>
  );
}

/**
 * The steepness bands as a table, climbing and descending apart: how much of
 * the route goes up steeply is a different fact from how much comes down it.
 */
function SteepnessTable({
  coordinates,
  distanceMetres,
  unitSystem,
}: {
  coordinates: Position[];
  distanceMetres: number;
  unitSystem: UnitSystem;
}) {
  const rows = steepnessEntries(gradientSharesBySign(coordinates), distanceMetres);
  const cell = "px-2 py-1 text-right text-xs tabular-nums";

  return (
    <table className="border-collapse">
      <thead>
        <tr className="border-b border-[var(--rule)] text-[10px] tracking-[0.06em] text-[var(--ink-2)] uppercase">
          <th scope="col" className="px-2 py-1 text-left font-semibold">
            Steepness
          </th>
          <th scope="col" className="px-2 py-1 text-right font-semibold">
            <span className="inline-flex items-center gap-0.5">
              Up <IconArrowUpRight size={11} stroke={2} aria-hidden="true" />
            </span>
          </th>
          <th scope="col" className="px-2 py-1 text-right font-semibold">
            <span className="inline-flex items-center gap-0.5">
              Down <IconArrowDownRight size={11} stroke={2} aria-hidden="true" />
            </span>
          </th>
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr key={row.label}>
            <th
              scope="row"
              className="flex items-center gap-1.5 px-2 py-1 text-left text-xs font-normal"
            >
              <span
                aria-hidden="true"
                className="size-2.5 rounded-xs"
                style={{ backgroundColor: row.colour }}
              />
              {row.label}
            </th>
            <td className={cell}>
              {row.climbingMetres < 1 ? "–" : formatDistance(row.climbingMetres, unitSystem)}
            </td>
            <td className={`${cell} text-[var(--ink-2)]`}>
              {row.descendingMetres < 1 ? "–" : formatDistance(row.descendingMetres, unitSystem)}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export interface RouteDockProps {
  title: string;
  /** Already restricted to the stretch on show, when there is a zoom. */
  profile: Profile | null;
  /** The whole route's length, which the ribbon and the strip are laid along. */
  distanceMetres: number;
  /** The whole route's total climb, for the profile stop's resting line. */
  ascentMetres: number;
  surface: SurfaceSummary | null;
  climbs: Climb[];
  /** Opens the shared map/chart window on one climb, as the brackets do. */
  onSelectClimb: (climb: Climb) => void;
  /** The route's own geometry, which the strip's wind reading and the steepness table are measured against. */
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
  /** The forecast measure the map is washed in, and null for none. */
  measure: MeasureKey | null;
  onMeasureChange: (measure: MeasureKey | null) => void;
  unitSystem: UnitSystem;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function ProfileStop({
  profile,
  title,
  distanceMetres,
  ascentMetres,
  surface,
  climbs,
  onSelectClimb,
  coordinates,
  activeMetres,
  onActiveChange,
  zoomWindow,
  onZoomChange,
  highlight,
  onHighlightChange,
  unitSystem,
  climbsOpen,
  onClimbsOpenChange,
}: Pick<
  RouteDockProps,
  | "profile"
  | "title"
  | "distanceMetres"
  | "ascentMetres"
  | "surface"
  | "climbs"
  | "onSelectClimb"
  | "coordinates"
  | "activeMetres"
  | "onActiveChange"
  | "zoomWindow"
  | "onZoomChange"
  | "highlight"
  | "onHighlightChange"
  | "unitSystem"
> & { climbsOpen: boolean; onClimbsOpenChange: (open: boolean) => void }) {
  // A touch pointer arms the zoom by holding, so the hint names that gesture.
  const coarse = useCoarsePointer();
  const line = (() => {
    if (activeMetres !== null && profile) {
      return profileReadout({ profile, surface, activeMetres, unitSystem });
    }
    if (zoomWindow) {
      return `${formatDistance(zoomWindow.startMetres, unitSystem)}–${formatDistance(zoomWindow.endMetres, unitSystem)} shown · Escape returns`;
    }
    if (!profile) {
      return "";
    }
    const range = ` · ${formatElevation(profile.minElevationMetres, unitSystem)}–${formatElevation(profile.maxElevationMetres, unitSystem)}`;
    const hint = coarse ? "press and hold to look closer" : "drag across to look closer";
    return `${formatDistance(distanceMetres, unitSystem)} · ${formatAscent(ascentMetres, unitSystem)} up${range} · ${hint}`;
  })();

  const climbsSidebar = (
    <ClimbsSidebar
      climbs={climbs}
      open={climbsOpen}
      onOpenChange={onClimbsOpenChange}
      onSelect={onSelectClimb}
      unitSystem={unitSystem}
      fixedHeight
    />
  );

  return (
    <div className="flex h-full items-stretch gap-3">
      <div className="min-w-0 flex-1">
        <Panel
          line={line}
          lineLabel="Elevation summary"
          control={
            <div className="flex items-center gap-2">
              {zoomWindow && onZoomChange ? (
                <button
                  type="button"
                  aria-keyshortcuts="Escape"
                  onClick={() => onZoomChange(null)}
                  className="rounded-full border border-[var(--rule)] px-2 py-0.5 text-[11px] text-[var(--ink-2)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)]"
                >
                  Whole route
                </button>
              ) : null}
              {climbsOpen ? null : climbsSidebar}
            </div>
          }
          info={
            <SteepnessTable
              coordinates={coordinates}
              distanceMetres={distanceMetres}
              unitSystem={unitSystem}
            />
          }
        >
          {/*
           * `min-w-0`: a grid item's default min-width is its content's, and the
           * chart's own explicit pixel width would hold this open above the
           * climbs sidebar rather than shrinking back when it reopens.
           */}
          <div className="relative min-w-0">
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
              caption={false}
              zoomBack={false}
            />
            <ClimbMarkers climbs={climbs} totalMetres={distanceMetres} onSelect={onActiveChange} />
          </div>
          <div style={GUTTER}>
            <GroundRibbon
              segments={groundSegments(surface)}
              surface={surface}
              thin
              unmarked={["asphalt"]}
              labelled
              highlight={highlight}
              onHighlightChange={onHighlightChange}
            />
          </div>
        </Panel>
      </div>
      {climbsOpen ? climbsSidebar : null}
    </div>
  );
}

function ForecastStop({
  distanceMetres,
  coordinates,
  samples,
  startAt,
  onStartAtChange,
  movingSeconds,
  zoomWindow,
  measure,
  onMeasureChange,
  unitSystem,
}: Pick<
  RouteDockProps,
  | "distanceMetres"
  | "coordinates"
  | "samples"
  | "startAt"
  | "onStartAtChange"
  | "movingSeconds"
  | "zoomWindow"
  | "measure"
  | "onMeasureChange"
  | "unitSystem"
>) {
  const shown = zoomWindow ?? { startMetres: 0, endMetres: distanceMetres };
  const back = samples[samples.length - 1]?.arrivalAt;

  return (
    <Panel
      gutter={false}
      line={`Forecast${back === undefined ? "" : ` · back ${clockAt(back)}`}`}
      info={
        <p className="max-w-64 text-xs text-[var(--ink-2)]">
          {forecastResolution(forecastLeadHours(samples)).sentence}
        </p>
      }
      control={
        <div className="flex flex-wrap items-center gap-3">
          <ConditionsChoices
            measure={measure}
            onMeasureChange={onMeasureChange}
            samples={samples}
            movingSeconds={movingSeconds}
          />
          <StartTimePicker
            value={startAt}
            onChange={onStartAtChange}
            movingSeconds={movingSeconds}
            inline
          />
        </div>
      }
    >
      <ForecastStrip
        samples={samples}
        coordinates={coordinates}
        startMetres={shown.startMetres}
        endMetres={shown.endMetres}
        unitSystem={unitSystem}
        inset={false}
        caption={false}
      />
      <ConditionsKey measure={measure} samples={samples} unitSystem={unitSystem} />
    </Panel>
  );
}

const RAIL_TAB =
  "flex w-14 flex-col items-center gap-0.5 rounded-md px-1 py-1.5 text-[10px] leading-none text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)] data-[active]:bg-[var(--base)] data-[active]:font-semibold data-[active]:text-[var(--ink)]";

/** A stop on the rail, open or folded — matches the `Tabs.Tab` values below. */
type Stop = "profile" | "forecast";

const FOLDED_CONTROL =
  "flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)]";

export function RouteDock({
  title,
  profile,
  distanceMetres,
  ascentMetres,
  surface,
  climbs,
  onSelectClimb,
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
  measure,
  onMeasureChange,
  unitSystem,
  open,
  onOpenChange,
}: RouteDockProps) {
  const back = samples[samples.length - 1]?.arrivalAt;
  const [climbsOpen, setClimbsOpen] = useState(true);
  // The stop shown while open, and the one Show reopens on — kept across folds.
  const [stop, setStop] = useState<Stop>("profile");

  if (!open) {
    return (
      <div
        role="group"
        aria-label="Route detail, folded"
        className="flex h-9 w-full items-center gap-1 rounded-xl bg-[var(--panel)] px-2 shadow-[var(--shadow)] ring-1 ring-black/5"
      >
        <button
          type="button"
          aria-label="Show the profile"
          onClick={() => {
            setStop("profile");
            onOpenChange(true);
          }}
          className={FOLDED_CONTROL}
        >
          <IconMountain size={15} stroke={2} aria-hidden="true" />
          Profile
        </button>
        <button
          type="button"
          aria-label="Show the forecast"
          onClick={() => {
            setStop("forecast");
            onOpenChange(true);
          }}
          className={FOLDED_CONTROL}
        >
          <IconCloud size={15} stroke={2} aria-hidden="true" />
          Forecast
        </button>
        {back === undefined ? null : (
          <span className="ml-auto text-[10px] text-[var(--ink-2)]">back {clockAt(back)}</span>
        )}
      </div>
    );
  }

  return (
    <section
      aria-label="Route detail"
      className="relative w-full rounded-xl bg-[var(--panel)] p-4 shadow-[var(--shadow)] ring-1 ring-black/5"
    >
      <Tabs.Root
        value={stop}
        onValueChange={(value) => setStop(value as Stop)}
        orientation="vertical"
        className="flex gap-3"
      >
        <div className="flex shrink-0 flex-col border-r border-[var(--rule)] pr-2">
          <Tabs.List className="flex flex-col gap-0.5">
            <Tabs.Tab value="profile" className={RAIL_TAB}>
              <IconMountain size={15} stroke={2} aria-hidden="true" />
              Profile
            </Tabs.Tab>
            <Tabs.Tab value="forecast" className={RAIL_TAB}>
              <IconCloud size={15} stroke={2} aria-hidden="true" />
              Forecast
            </Tabs.Tab>
          </Tabs.List>
          <button
            type="button"
            aria-expanded
            aria-label="Hide the route detail"
            onClick={() => onOpenChange(false)}
            className={`${RAIL_TAB} mt-auto`}
          >
            <IconLayoutBottombarCollapse size={15} stroke={2} aria-hidden="true" />
            Hide
          </button>
        </div>
        {/* One height for every stop, so switching never moves the map's foot. */}
        <div className="h-52 min-w-0 flex-1 [&>[role=tabpanel]]:h-full">
          <Tabs.Panel value="profile" className="min-w-0">
            <ProfileStop
              profile={profile}
              title={title}
              distanceMetres={distanceMetres}
              ascentMetres={ascentMetres}
              surface={surface}
              climbs={climbs}
              onSelectClimb={onSelectClimb}
              coordinates={coordinates}
              activeMetres={activeMetres}
              onActiveChange={onActiveChange}
              zoomWindow={zoomWindow}
              onZoomChange={onZoomChange}
              highlight={highlight}
              onHighlightChange={onHighlightChange}
              unitSystem={unitSystem}
              climbsOpen={climbsOpen}
              onClimbsOpenChange={setClimbsOpen}
            />
          </Tabs.Panel>
          <Tabs.Panel value="forecast" className="min-w-0">
            <ForecastStop
              distanceMetres={distanceMetres}
              coordinates={coordinates}
              samples={samples}
              startAt={startAt}
              onStartAtChange={onStartAtChange}
              movingSeconds={movingSeconds}
              zoomWindow={zoomWindow}
              measure={measure}
              onMeasureChange={onMeasureChange}
              unitSystem={unitSystem}
            />
          </Tabs.Panel>
        </div>
      </Tabs.Root>
    </section>
  );
}
