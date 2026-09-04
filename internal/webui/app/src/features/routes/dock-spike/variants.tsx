/**
 * Four ways the dock could give each lane its own room.
 *
 * Stacked, the dock says one true thing — the rain lands on the gravel col —
 * at the cost of saying everything at once and having nowhere to put a fifth
 * lane. Each variant below takes a different position on what stays on show
 * while the rest waits behind a tab, so the choice is between ideas rather
 * than between margins. Storybook only: nothing here is imported by the app.
 */

import { Tabs } from "@base-ui/react/tabs";
import { Tooltip } from "@base-ui/react/tooltip";
import {
  IconCircleOff,
  IconCloud,
  IconCloudRain,
  IconMountain,
  type IconProps,
  IconRoad,
  IconStairs,
  IconTemperature,
  IconWind,
} from "@tabler/icons-react";
import type { ComponentType, ReactNode } from "react";
import { useState } from "react";
import { StartTimePicker } from "../../../components/StartTimePicker";
import { formatAscent, formatDistance, formatElevation } from "../../../lib/format";
import type { Highlight } from "../../../lib/highlight";
import type { Measure, MeasureKey } from "../../../lib/measures";
import {
  MEASURES,
  measureVariable,
  WIND_RELATION_KEY,
  windRelationVariable,
} from "../../../lib/measures";
import { groundSegments } from "../../../lib/mix";
import { PADDING } from "../../../lib/plotAxis";
import type { DistanceWindow } from "../../../lib/profile";
import {
  climbs,
  coordinates,
  profile,
  rideStart,
  route,
  surface,
  weatherSamples,
} from "../../../storybook/fixtures";
import { ClimbMarkers } from "../ClimbMarkers";
import { ClimbsSidebar } from "../ClimbsSidebar";
import { ConditionsPicker } from "../ConditionsPicker";
import { ElevationProfile } from "../ElevationProfile";
import { ForecastStrip } from "../ForecastStrip";
import { GroundRibbon } from "../GroundRibbon";

const UNITS = "metric";
const GUTTER = { paddingLeft: PADDING.left, paddingRight: PADDING.right };

/** The state every variant shares, so a press in one lane shows in the others. */
export interface DockState {
  highlight: Highlight | null;
  setHighlight: (next: Highlight | null) => void;
  measure: MeasureKey | null;
  setMeasure: (next: MeasureKey | null) => void;
  activeMetres: number | null;
  setActiveMetres: (next: number | null) => void;
  zoomWindow: DistanceWindow | null;
  setZoomWindow: (next: DistanceWindow | null) => void;
  startAt: Date | null;
  setStartAt: (next: Date | null) => void;
}

export function useDockState(): DockState {
  const [highlight, setHighlight] = useState<Highlight | null>(null);
  const [measure, setMeasure] = useState<MeasureKey | null>(null);
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const [zoomWindow, setZoomWindow] = useState<DistanceWindow | null>(null);
  const [startAt, setStartAt] = useState<Date | null>(new Date(rideStart.getTime() - 3_600_000));

  return {
    highlight,
    setHighlight,
    measure,
    setMeasure,
    activeMetres,
    setActiveMetres,
    zoomWindow,
    setZoomWindow,
    startAt,
    setStartAt,
  };
}

/**
 * The frame every stop shares: one line of figures across the top, a control
 * at its end where the stop has one, and the body beneath. The line is data
 * rather than a heading — a reader on the profile stop already knows it is the
 * profile; what they do not know is how high it goes.
 */
function Panel({
  line,
  control,
  gutter = true,
  children,
}: {
  line: string;
  control?: ReactNode;
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
        <output className="text-xs text-[var(--ink-2)] tabular-nums">
          {/* The first figure carries the line; the rest qualifies it. */}
          <span className="text-sm font-semibold text-[var(--ink)]">{lead}</span>
          {rest === undefined ? null : ` · ${rest}`}
        </output>
        {control}
      </div>
      {children}
    </div>
  );
}

/** `14:20`, in the reader's own zone. */
function clockAt(at: Date): string {
  return at.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}

/** What the profile stop says about itself: the figures, then the gesture. */
function profileLine(s: DockState): string {
  if (s.zoomWindow) {
    return `${formatDistance(s.zoomWindow.startMetres, UNITS)}–${formatDistance(s.zoomWindow.endMetres, UNITS)} shown · Escape returns`;
  }
  const range =
    profile === null
      ? ""
      : ` · ${formatElevation(profile.minElevationMetres, UNITS)}–${formatElevation(profile.maxElevationMetres, UNITS)}`;
  return `${formatDistance(route.distanceMetres, UNITS)} · ${formatAscent(route.ascentMetres, UNITS)} up${range} · drag across to look closer`;
}

/* ---- the conditions key, as chips ---------------------------------------- */

// ponytail: the cuts live privately in measures.ts; the real thing exports them.
const RANGES: Record<MeasureKey, readonly string[]> = {
  wind: ["under 15 km/h", "15–30 km/h", "30–45 km/h", "over 45 km/h"],
  temperature: ["under 5 °C", "5–12 °C", "12–20 °C", "20–27 °C", "over 27 °C"],
  rain: ["under 0.2 mm/h", "0.2–2 mm/h", "2–6 mm/h", "over 6 mm/h"],
  cloud: ["under 20 %", "20–50 %", "50–85 %", "over 85 %"],
};

const MEASURE_ICON: Record<MeasureKey, ComponentType<IconProps>> = {
  wind: IconWind,
  temperature: IconTemperature,
  rain: IconCloudRain,
  cloud: IconCloud,
};

const CHOICE =
  "flex items-center gap-1 rounded-full border border-[var(--rule)] px-2 py-0.5 text-[11px] leading-none text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)] aria-pressed:border-[var(--accent)] aria-pressed:font-semibold aria-pressed:text-[var(--ink)]";

/** The looks a key can wear; the story compares them, the rail picks one. */
export type Look =
  | "pill"
  | "swatch"
  | "square"
  | "bar"
  | "stroke"
  | "keyed"
  | "underline"
  | "dot"
  | "ramp";

/** A swatch of one shape beside a word. */
function Marked({ mark, label }: { mark: ReactNode; label: string }) {
  return (
    <Tooltip.Trigger className={`${TRIGGER} flex items-center gap-1 rounded px-0.5`}>
      {mark}
      {label}
    </Tooltip.Trigger>
  );
}

const TRIGGER =
  "text-[11px] leading-none text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)]";

/** One band; what it means and where it cuts wait under the pointer. */
function Band({
  look,
  colour,
  opacity = 1,
  label,
  detail,
  stroke = false,
}: {
  look: Look;
  colour: string;
  opacity?: number;
  label: string;
  detail: string;
  /** Whether this band is drawn on the route line rather than as a wash, for `keyed`. */
  stroke?: boolean;
}) {
  const fill = `color-mix(in srgb, ${colour} ${opacity * 100}%, transparent)`;
  const wash = `color-mix(in srgb, ${colour} ${opacity * 60}%, transparent)`;
  const border = "border border-[var(--rule)]";
  const marks = {
    swatch: (
      <span
        aria-hidden="true"
        className={`h-2.5 w-3.5 rounded-xs ${border}`}
        style={{ backgroundColor: fill }}
      />
    ),
    square: (
      <span
        aria-hidden="true"
        className={`size-2.5 rounded-xs ${border}`}
        style={{ backgroundColor: fill }}
      />
    ),
    bar: (
      <span
        aria-hidden="true"
        className={`h-2 w-4 rounded-full ${border}`}
        style={{ backgroundColor: fill }}
      />
    ),
    stroke: (
      <span
        aria-hidden="true"
        className="h-0.5 w-4 rounded-full"
        style={{ backgroundColor: opacity === 0 ? "var(--rule)" : colour }}
      />
    ),
    dot: (
      <span
        aria-hidden="true"
        className={`size-2 rounded-full ${border}`}
        style={{ backgroundColor: fill }}
      />
    ),
  };
  const shape = {
    pill: (
      <Tooltip.Trigger
        className={`${TRIGGER} rounded-full border border-[var(--rule)] px-2 py-0.5`}
        style={{ backgroundColor: wash }}
      >
        {label}
      </Tooltip.Trigger>
    ),
    swatch: <Marked mark={marks.swatch} label={label} />,
    square: <Marked mark={marks.square} label={label} />,
    bar: <Marked mark={marks.bar} label={label} />,
    stroke: <Marked mark={marks.stroke} label={label} />,
    keyed: <Marked mark={stroke ? marks.stroke : marks.bar} label={label} />,
    underline: (
      <Tooltip.Trigger
        className={`${TRIGGER} border-b-2 px-0.5 pb-0.5`}
        style={{ borderColor: opacity === 0 ? "var(--rule)" : colour }}
      >
        {label}
      </Tooltip.Trigger>
    ),
    dot: <Marked mark={marks.dot} label={label} />,
    ramp: (
      <Tooltip.Trigger
        className={`${TRIGGER} px-2 py-0.5 first:rounded-l-sm last:rounded-r-sm`}
        style={{ backgroundColor: wash, boxShadow: "inset 0 0 0 1px var(--rule)" }}
      >
        {label}
      </Tooltip.Trigger>
    ),
  }[look];

  return (
    <Tooltip.Root>
      {shape}
      <Tooltip.Portal>
        <Tooltip.Positioner sideOffset={6}>
          <Tooltip.Popup className="max-w-56 rounded-md bg-[var(--ink)] px-2 py-1 text-[11px] text-[var(--panel)] shadow-[var(--shadow)]">
            {detail}
          </Tooltip.Popup>
        </Tooltip.Positioner>
      </Tooltip.Portal>
    </Tooltip.Root>
  );
}

/** One key: a lead word and its bands, butted together for the ramp and spaced otherwise. */
function Key({ lead, look, children }: { lead: string; look: Look; children: ReactNode }) {
  return (
    <div className="flex items-center gap-x-1.5">
      <span className="text-[10px] tracking-[0.06em] text-[var(--ink-2)] uppercase">{lead}</span>
      <ul className={look === "ramp" ? "flex items-center" : "flex items-center gap-x-1.5"}>
        {children}
      </ul>
    </div>
  );
}

/** The key for one wash, on one line: the corridor, and for wind the route line too. */
export function WashKey({ measure, look }: { measure: Measure; look: Look }) {
  return (
    <Tooltip.Provider>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-[var(--ink-2)]">
        <Key lead="Corridor" look={look}>
          {measure.bands.map((band, index) => {
            const opacity = measure.opacity(index);
            const range = RANGES[measure.key][index] ?? "";
            return (
              <li key={band.label}>
                <Band
                  look={look}
                  colour={measureVariable(measure.key, index)}
                  opacity={opacity}
                  label={band.label}
                  detail={`${band.description} · ${range}${opacity === 0 ? " · not washed" : ""}`}
                />
              </li>
            );
          })}
        </Key>
        {measure.key === "wind" ? (
          <Key lead="Route line" look={look}>
            {WIND_RELATION_KEY.map((band) => (
              <li key={band.description}>
                <Band
                  look={look}
                  stroke
                  colour={windRelationVariable(band.stop)}
                  label={band.label}
                  detail={`${band.description} · replaces the steepness edging`}
                />
              </li>
            ))}
          </Key>
        ) : null}
      </div>
    </Tooltip.Provider>
  );
}

/** The choice of wash, for the panel's top line. */
function Choices(s: DockState) {
  return (
    <div role="group" aria-label="Conditions washed along the route" className="flex gap-1">
      <button
        type="button"
        aria-pressed={s.measure === null}
        onClick={() => s.setMeasure(null)}
        className={CHOICE}
      >
        <IconCircleOff size={12} stroke={2} aria-hidden="true" />
        Off
      </button>
      {MEASURES.map((entry) => {
        const Icon = MEASURE_ICON[entry.key];
        return (
          <button
            key={entry.key}
            type="button"
            aria-pressed={s.measure === entry.key}
            onClick={() => s.setMeasure(s.measure === entry.key ? null : entry.key)}
            className={CHOICE}
          >
            <Icon size={12} stroke={2} aria-hidden="true" />
            {entry.label}
          </button>
        );
      })}
    </div>
  );
}

function Conditions(s: DockState) {
  const chosen = MEASURES.find((entry) => entry.key === s.measure);
  return chosen === undefined ? null : <WashKey measure={chosen} look="swatch" />;
}

/* ---- the lanes, as they exist today, each wrapped once ------------------ */

function Profile(s: DockState) {
  return (
    <div className="relative">
      <ElevationProfile
        profile={profile}
        title={route.title}
        surface={surface}
        activeMetres={s.activeMetres}
        onActiveChange={s.setActiveMetres}
        zoomWindow={s.zoomWindow}
        onZoomChange={s.setZoomWindow}
        highlight={s.highlight}
        unitSystem={UNITS}
      />
      <ClimbMarkers
        climbs={climbs}
        totalMetres={route.distanceMetres}
        onSelect={s.setActiveMetres}
      />
    </div>
  );
}

function Ground(s: DockState) {
  return (
    <div style={GUTTER}>
      <GroundRibbon
        segments={groundSegments(surface)}
        surface={surface}
        labelled
        highlight={s.highlight}
        onHighlightChange={s.setHighlight}
      />
    </div>
  );
}

function Forecast(s: DockState) {
  const shown = s.zoomWindow ?? { startMetres: 0, endMetres: route.distanceMetres };
  const back = weatherSamples[weatherSamples.length - 1]?.arrivalAt;

  return (
    <Panel
      gutter={false}
      line={`Forecast${back === undefined ? "" : ` · back ${clockAt(back)}`}`}
      control={
        <div className="flex flex-wrap items-center gap-3">
          <Choices {...s} />
          <StartTimePicker
            value={s.startAt}
            onChange={s.setStartAt}
            movingSeconds={route.movingSeconds}
            inline
          />
        </div>
      }
    >
      {/* ponytail: the strip reserves the chart's gutters itself; undone here, parameterised for real. */}
      <div style={{ marginLeft: -PADDING.left, marginRight: -PADDING.right }}>
        <ForecastStrip
          samples={weatherSamples}
          coordinates={coordinates}
          startMetres={shown.startMetres}
          endMetres={shown.endMetres}
          unitSystem={UNITS}
        />
      </div>
      <Conditions {...s} />
    </Panel>
  );
}

/** Enough climbs to make the list scroll, which one fixture climb cannot. */
const MANY_CLIMBS = climbs.flatMap((climb) =>
  Array.from({ length: 9 }, (_, i) => {
    const startMetres = 200 + i * 300;
    return { ...climb, startMetres, endMetres: startMetres + 200, distanceMetres: 200 };
  }),
);

/** The profile over its ground, with the climbs beside, scrolling against their height. */
function ProfileWithClimbs(s: DockState) {
  return (
    <div className="flex h-full items-stretch gap-3">
      <div className="min-w-0 flex-1">
        <Panel line={profileLine(s)}>
          <Profile {...s} />
          <Ground {...s} />
        </Panel>
      </div>
      {/* Five whole rows of 28px: the rows snap, and a viewport that is a
          multiple of them never cuts one in half. */}
      <div className="relative w-80 shrink-0">
        <div className="absolute inset-0 flex [&>section]:flex [&>section]:flex-col [&_ol]:h-[140px] [&_ol]:flex-none">
          <ClimbsSidebar
            climbs={MANY_CLIMBS}
            open
            onOpenChange={() => {}}
            onSelect={() => {}}
            unitSystem={UNITS}
          />
        </div>
      </div>
    </div>
  );
}

function Climbs() {
  // The sidebar always open: the tab is the fold now.
  return (
    <div className="flex">
      <ClimbsSidebar
        climbs={climbs}
        open
        onOpenChange={() => {}}
        onSelect={() => {}}
        unitSystem={UNITS}
      />
    </div>
  );
}

/* ---- tab vocabulary ----------------------------------------------------- */

interface Lane {
  key: string;
  label: string;
  Icon: ComponentType<IconProps>;
  render: (s: DockState) => ReactNode;
}

const LANES: Lane[] = [
  { key: "profile", label: "Profile", Icon: IconMountain, render: Profile },
  { key: "ground", label: "Ground", Icon: IconRoad, render: Ground },
  { key: "forecast", label: "Forecast", Icon: IconCloud, render: Forecast },
  { key: "climbs", label: "Climbs", Icon: IconStairs, render: Climbs },
];

const TAB =
  "flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)] data-[active]:bg-[var(--base)] data-[active]:font-semibold data-[active]:text-[var(--ink)]";

function TabStrip({ lanes, trailing }: { lanes: Lane[]; trailing?: ReactNode }) {
  return (
    <div className="flex items-center gap-2">
      <Tabs.List className="flex gap-0.5 rounded-lg border border-[var(--rule)] p-0.5">
        {lanes.map(({ key, label, Icon }) => (
          <Tabs.Tab key={key} value={key} className={TAB}>
            <Icon size={13} stroke={2} aria-hidden="true" />
            {label}
          </Tabs.Tab>
        ))}
      </Tabs.List>
      {trailing}
    </div>
  );
}

function Panels({ lanes, state }: { lanes: Lane[]; state: DockState }) {
  return lanes.map(({ key, render }) => (
    <Tabs.Panel key={key} value={key} className="min-w-0">
      {render(state)}
    </Tabs.Panel>
  ));
}

function Shell({ children }: { children: ReactNode }) {
  return (
    <section
      aria-label="Route detail"
      className="w-full rounded-xl bg-[var(--panel)] p-4 shadow-[var(--shadow)] ring-1 ring-black/5"
    >
      {children}
    </section>
  );
}

/* ---- the variants ------------------------------------------------------- */

/**
 * A · Tabs. Everything behind a tab, one lane at a time at full height.
 * Cheapest to extend; loses the stacked "rain on the gravel col" reading.
 */
export function TabsDock(s: DockState) {
  return (
    <Shell>
      <Tabs.Root defaultValue="profile" className="grid gap-3">
        <TabStrip lanes={LANES} />
        <Panels lanes={LANES} state={s} />
      </Tabs.Root>
    </Shell>
  );
}

/**
 * B · Anchored. The profile is the axis everything else is read against, so
 * it never leaves; the tabs choose what lies beneath it.
 */
export function AnchoredDock(s: DockState) {
  const below = LANES.filter((lane) => lane.key !== "profile");

  return (
    <Shell>
      <div className="grid gap-2">
        <Profile {...s} />
        <Tabs.Root defaultValue="ground" className="grid gap-2">
          <div style={GUTTER}>
            <TabStrip lanes={below} />
          </div>
          <Panels lanes={below} state={s} />
        </Tabs.Root>
      </div>
    </Shell>
  );
}

/**
 * C · Rail. A vertical rail on the left, each stop an icon over a short word;
 * the lane it points at fills the width, at a height that is the same for
 * every stop. A tenth lane costs one more stop.
 * The ground stays under the profile and the climbs ride beside it rather than
 * taking stops of their own: both are only readable against the chart.
 */
const RAIL_LANES: Lane[] = [
  { key: "profile", label: "Profile", Icon: IconMountain, render: ProfileWithClimbs },
  ...LANES.filter((lane) => lane.key === "forecast"),
];

export function RailDock(s: DockState) {
  return (
    <Shell>
      <Tabs.Root defaultValue="profile" orientation="vertical" className="flex gap-3">
        <Tabs.List className="flex shrink-0 flex-col gap-0.5 border-r border-[var(--rule)] pr-2">
          {RAIL_LANES.map(({ key, label, Icon }) => (
            <Tabs.Tab
              key={key}
              value={key}
              className={`${TAB} w-14 flex-col gap-0.5 px-1 py-1.5 text-[10px] leading-none`}
            >
              <Icon size={15} stroke={2} aria-hidden="true" />
              {label}
            </Tabs.Tab>
          ))}
        </Tabs.List>
        {/* One height for every stop, so switching never moves the map's foot. */}
        <div className="h-52 min-w-0 flex-1 [&>[role=tabpanel]]:h-full">
          <Panels lanes={RAIL_LANES} state={s} />
        </div>
      </Tabs.Root>
    </Shell>
  );
}

/**
 * D · Split. The distance lanes stay stacked as today; only the side column
 * is tabbed, so the forecast's controls and the climbs stop fighting for the
 * foot of the dock.
 */
export function SplitDock(s: DockState) {
  const shown = s.zoomWindow ?? { startMetres: 0, endMetres: route.distanceMetres };
  const side: Lane[] = [
    { key: "climbs", label: "Climbs", Icon: IconStairs, render: Climbs },
    {
      key: "conditions",
      label: "Conditions",
      Icon: IconCloud,
      render: (state) => (
        <div className="grid gap-2">
          <StartTimePicker
            value={state.startAt}
            onChange={state.setStartAt}
            movingSeconds={route.movingSeconds}
            inline
          />
          <ConditionsPicker
            measure={state.measure}
            onMeasureChange={state.setMeasure}
            samples={weatherSamples}
            movingSeconds={route.movingSeconds}
          />
        </div>
      ),
    },
  ];

  return (
    <Shell>
      <div className="flex items-stretch gap-3">
        <div className="grid min-w-0 flex-1 gap-1.5">
          <Profile {...s} />
          <Ground {...s} />
          <ForecastStrip
            samples={weatherSamples}
            coordinates={coordinates}
            startMetres={shown.startMetres}
            endMetres={shown.endMetres}
            unitSystem={UNITS}
          />
        </div>
        <Tabs.Root
          defaultValue="climbs"
          className="grid w-80 shrink-0 content-start gap-2 border-l border-[var(--rule)] pl-3"
        >
          <TabStrip lanes={side} />
          <Panels lanes={side} state={s} />
        </Tabs.Root>
      </div>
    </Shell>
  );
}
