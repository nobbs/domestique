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
import { IconCloud, IconMountain, type IconProps, IconRoad, IconStairs } from "@tabler/icons-react";
import type { ComponentType, ReactNode } from "react";
import { useState } from "react";
import { StartTimePicker } from "../../../components/StartTimePicker";
import type { Highlight } from "../../../lib/highlight";
import type { MeasureKey } from "../../../lib/measures";
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

  return (
    <div className="grid gap-2">
      <div className="flex flex-wrap items-center justify-between gap-2" style={GUTTER}>
        <span className="text-[11px] text-[var(--ink-2)]">Forecast along the ride</span>
        <StartTimePicker
          value={s.startAt}
          onChange={s.setStartAt}
          movingSeconds={route.movingSeconds}
          inline
        />
      </div>
      <ForecastStrip
        samples={weatherSamples}
        coordinates={coordinates}
        startMetres={shown.startMetres}
        endMetres={shown.endMetres}
        unitSystem={UNITS}
      />
      <div style={GUTTER}>
        <ConditionsPicker
          measure={s.measure}
          onMeasureChange={s.setMeasure}
          samples={weatherSamples}
          movingSeconds={route.movingSeconds}
        />
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
 * the lane it points at fills the width. A tenth lane costs one more stop.
 */
export function RailDock(s: DockState) {
  return (
    <Shell>
      <Tabs.Root defaultValue="profile" orientation="vertical" className="flex gap-3">
        <Tabs.List className="flex shrink-0 flex-col gap-0.5 border-r border-[var(--rule)] pr-2">
          {LANES.map(({ key, label, Icon }) => (
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
        <div className="min-w-0 flex-1">
          <Panels lanes={LANES} state={s} />
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
