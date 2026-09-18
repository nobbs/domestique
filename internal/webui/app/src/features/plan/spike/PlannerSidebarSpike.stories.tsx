/**
 * Four looks for the planner's column beside the map, in the dashboard's own
 * language: inset blocks, segmented controls, panel marks, figures.
 * Storybook only, over a fixed sample plan; nothing here routes or saves.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  IconArrowsExchange,
  IconBike,
  IconChevronDown,
  IconClock,
  IconDeviceFloppy,
  IconDots,
  IconFlagCheck,
  IconGripVertical,
  IconMountain,
  IconPlayerPlay,
  IconPlus,
  IconRoad,
  IconRoute,
  IconTrash,
} from "@tabler/icons-react";
import { type ReactNode, useState } from "react";
import { Button } from "../../../components/Button";
import { Panel } from "../../../components/PanelHeading";
import { Segmented } from "../../../components/Segmented";
import { formatAscent, formatDistance, formatDuration } from "../../../lib/format";

type Profile = "trekking" | "fastbike" | "gravel";

const PROFILES = [
  { key: "trekking", label: "Trekking", icon: <IconBike size={14} stroke={1.8} /> },
  { key: "fastbike", label: "Road", icon: <IconRoad size={14} stroke={1.8} /> },
  { key: "gravel", label: "Gravel", icon: <IconRoute size={14} stroke={1.8} /> },
] as const satisfies readonly { key: Profile; label: string; icon: ReactNode }[];

const STATE = [
  { key: "draft", label: "Draft" },
  { key: "published", label: "Published" },
] as const;

interface Stop {
  id: number;
  latitude: number;
  longitude: number;
  /** Metres ridden from the start to this stop. */
  at: number;
  /** Metres climbed from the start to this stop. */
  climbed: number;
  /** What the geocoder calls the place; empty where it knows of none. */
  name: string;
}

const STOPS: Stop[] = [
  {
    id: 1,
    latitude: 49.0094,
    longitude: 8.4044,
    at: 0,
    climbed: 0,
    name: "Bahnhofplatz 1, Karlsruhe",
  },
  {
    id: 2,
    latitude: 49.0187,
    longitude: 8.4933,
    at: 9_400,
    climbed: 40,
    name: "Grötzingen, Karlsruhe",
  },
  {
    id: 3,
    latitude: 48.9988,
    longitude: 8.4761,
    at: 18_200,
    climbed: 330,
    name: "Turmberg, Karlsruhe",
  },
  { id: 4, latitude: 48.9926, longitude: 8.4728, at: 27_600, climbed: 470, name: "" },
  {
    id: 5,
    latitude: 49.0094,
    longitude: 8.4044,
    at: 34_800,
    climbed: 512,
    name: "Bahnhofplatz 1, Karlsruhe",
  },
];

const coordinates = (stop: Stop) => `${stop.latitude.toFixed(4)}, ${stop.longitude.toFixed(4)}`;
const DISTANCE = 34_800;
const ASCENT = 512;

// This service's own forward model, as `internal/ridemodel` computes a stage's
// moving time: its built-in pair, until a calibration replaces them.
const SECONDS_PER_KM = 145.3578;
const SECONDS_PER_ASCENT_M = 3.219;

function secondsTo(stop: Stop): number {
  return (stop.at / 1000) * SECONDS_PER_KM + stop.climbed * SECONDS_PER_ASCENT_M;
}

const roundToMinute = (seconds: number) => Math.round(seconds / 60) * 60;

const WASH = "bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]";

function stopLabel(index: number, count: number) {
  if (index === 0) return "Start";
  if (index === count - 1) return "Finish";
  return `Via ${index}`;
}

function StopMark({ index, count, size = 24 }: { index: number; count: number; size?: number }) {
  const last = index === count - 1;
  return (
    <span
      aria-hidden="true"
      className="grid shrink-0 place-items-center rounded-[8px] font-semibold text-[11px] text-white"
      style={{
        width: size,
        height: size,
        background: last || index === 0 ? "var(--accent)" : "var(--ink-2)",
      }}
    >
      {index === 0 ? (
        <IconPlayerPlay size={12} stroke={3} />
      ) : last ? (
        <IconFlagCheck size={13} stroke={2.5} />
      ) : (
        index + 1
      )}
    </span>
  );
}

/** The map the column sits on, so each variant is read at its real width. */
function Stage({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-[720px] gap-0 overflow-hidden rounded-2xl bg-[var(--base)] shadow-[var(--shadow)]">
      <div className="flex w-[360px] shrink-0 flex-col overflow-y-auto bg-[var(--panel)]">
        {children}
      </div>
      <div className="grid flex-1 place-items-center bg-[repeating-linear-gradient(45deg,color-mix(in_oklab,var(--ink-2)_6%,transparent)_0_12px,transparent_12px_24px)] text-[var(--ink-2)] text-sm">
        the map
      </div>
    </div>
  );
}

function Figures({ dense = false }: { dense?: boolean }) {
  return (
    <div className={`flex items-center gap-4 ${dense ? "" : "px-1"}`}>
      <span className="font-semibold text-xl tabular-nums tracking-tight">
        {formatDistance(DISTANCE)}
      </span>
      <span className="flex items-baseline gap-1 text-[var(--ink-2)]">
        <IconMountain size={14} stroke={1.8} aria-hidden="true" />
        <span className="font-medium tabular-nums">{formatAscent(ASCENT)}</span>
      </span>
      <span className="flex items-baseline gap-1 text-[var(--ink-2)]">
        <IconClock size={14} stroke={1.8} aria-hidden="true" />
        <span className="font-medium tabular-nums">
          {formatDuration(roundToMinute(secondsTo(STOPS[STOPS.length - 1] as Stop)))}
        </span>
      </span>
    </div>
  );
}

/* ── A. Brief ─────────────────────────────────────────────────────────────── */

function VariantA() {
  const [profile, setProfile] = useState<Profile>("gravel");
  return (
    <Stage>
      <div className="flex flex-1 flex-col gap-4 p-4">
        <div className="flex items-start gap-2">
          <input
            aria-label="Plan name"
            defaultValue="Turmberg loop"
            className="min-w-0 flex-1 rounded-[9px] border-transparent border-b-2 bg-transparent px-1 py-0.5 font-semibold text-lg outline-none focus:border-[var(--accent)]"
          />
          <Button variant="ghost" icon={<IconDots size={16} />} aria-label="Plan actions" />
        </div>
        <Figures />
        <Segmented label="Route type" items={PROFILES} value={profile} onChange={setProfile} />
        <section className="flex flex-col gap-2">
          <h4 className="flex items-center justify-between px-1 font-semibold text-sm">
            Waypoints
            <span className="font-normal text-[var(--ink-2)] text-xs">{STOPS.length} stops</span>
          </h4>
          <ul className={`flex flex-col overflow-hidden rounded-xl ${WASH}`}>
            {STOPS.map((stop, index) => (
              <li
                key={stop.id}
                className="flex items-center gap-2.5 border-[var(--panel)] border-b-2 px-3 py-2 last:border-b-0"
              >
                <StopMark index={index} count={STOPS.length} />
                <span className="min-w-0 flex-1">
                  {/* The name where the geocoder has one, the coordinate otherwise. */}
                  <span className="block truncate text-sm">
                    {stop.name === "" ? coordinates(stop) : stop.name}
                  </span>
                  <span className="text-[var(--ink-2)] text-xs tabular-nums">
                    {stopLabel(index, STOPS.length)}
                    {index === 0
                      ? ""
                      : ` · ${formatDistance(stop.at)} · ${formatDuration(roundToMinute(secondsTo(stop)))}`}
                  </span>
                </span>
                <IconGripVertical
                  size={15}
                  className="shrink-0 cursor-grab text-[var(--ink-2)]"
                  aria-hidden="true"
                />
                <Button
                  variant="ghost"
                  icon={<IconTrash size={15} />}
                  aria-label={`Delete ${stopLabel(index, STOPS.length)}`}
                />
              </li>
            ))}
          </ul>
          <p className="px-1 text-[var(--ink-2)] text-xs">
            Click the map to insert a stop, Alt-click to append.
          </p>
        </section>
      </div>
      <div className="sticky bottom-0 flex items-center gap-2 border-[var(--rule)] border-t bg-[var(--panel)] p-3">
        <Button icon={<IconDeviceFloppy size={16} stroke={1.6} />} className="flex-1">
          Save changes
        </Button>
        <Button variant="outline">Publish</Button>
      </div>
    </Stage>
  );
}

/* ── B. Itinerary ─────────────────────────────────────────────────────────── */

function VariantB() {
  const [profile, setProfile] = useState<Profile>("gravel");
  const [state, setState] = useState<"draft" | "published">("draft");
  return (
    <Stage>
      <div className="flex items-center gap-1 border-[var(--rule)] border-b px-3 py-2.5">
        <Button variant="ghost" icon={<IconChevronDown size={16} stroke={2} />}>
          Plans
        </Button>
        <Button variant="ghost" icon={<IconPlus size={16} stroke={2} />} className="ml-auto">
          New
        </Button>
      </div>
      <div className="flex flex-1 flex-col gap-4 p-4">
        <input
          aria-label="Plan name"
          defaultValue="Turmberg loop"
          className="w-full rounded-[9px] bg-transparent py-0.5 font-semibold text-lg outline-none focus:bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] focus:px-2"
        />
        <div className={`flex items-center justify-between gap-3 rounded-xl ${WASH} px-3.5 py-2.5`}>
          <Figures dense />
          <Button variant="ghost" icon={<IconArrowsExchange size={16} />} aria-label="Reverse" />
        </div>
        <Segmented
          label="Route type"
          size="sm"
          items={PROFILES}
          value={profile}
          onChange={setProfile}
        />
        <ol aria-label="Waypoints" className="relative flex flex-col">
          {/* The rail runs behind the marks, from the first to the last. */}
          <span
            aria-hidden="true"
            className="-translate-x-1/2 absolute top-4 bottom-4 left-3 w-0.5 bg-[var(--rule)]"
          />
          {STOPS.map((stop, index) => (
            <li key={stop.id} className="relative flex items-center gap-3 py-1.5">
              <StopMark index={index} count={STOPS.length} />
              <span className="min-w-0 flex-1 truncate text-sm">{coordinates(stop)}</span>
              <span className="shrink-0 text-[var(--ink-2)] text-xs tabular-nums">
                {index === 0
                  ? "start"
                  : `+${formatDistance(stop.at - (STOPS[index - 1]?.at ?? 0))}`}
              </span>
              <Button
                variant="ghost"
                icon={<IconTrash size={15} />}
                aria-label={`Delete ${stopLabel(index, STOPS.length)}`}
              />
            </li>
          ))}
        </ol>
      </div>
      <div className="sticky bottom-0 flex flex-col gap-2 border-[var(--rule)] border-t bg-[var(--panel)] p-3">
        <Segmented label="Plan state" size="sm" items={STATE} value={state} onChange={setState} />
        <Button icon={<IconDeviceFloppy size={16} stroke={1.6} />}>Save changes</Button>
      </div>
    </Stage>
  );
}

/* ── C. Cards ─────────────────────────────────────────────────────────────── */

function VariantC() {
  const [profile, setProfile] = useState<Profile>("gravel");
  return (
    <Stage>
      <div className="flex flex-1 flex-col gap-3 bg-[var(--base)] p-3">
        <Panel
          icon={<IconRoute size={18} stroke={1.8} />}
          title="Turmberg loop"
          subtitle="draft"
          aside={<Button variant="ghost" icon={<IconDots size={16} />} aria-label="Plan actions" />}
        >
          <Figures dense />
          <Segmented
            label="Route type"
            size="sm"
            items={PROFILES}
            value={profile}
            onChange={setProfile}
          />
        </Panel>
        <Panel
          icon={<IconFlagCheck size={18} stroke={1.8} />}
          title="Waypoints"
          subtitle={`${STOPS.length} stops`}
          className="flex-1"
        >
          <ul className={`-mx-4 flex flex-col overflow-hidden rounded-xl ${WASH}`}>
            {STOPS.map((stop, index) => (
              <li
                key={stop.id}
                className="flex items-center gap-2.5 border-[var(--panel)] border-b-2 py-1 pr-1.5 pl-3 text-sm last:border-b-0"
              >
                <StopMark index={index} count={STOPS.length} />
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate">
                    {stop.name === "" ? coordinates(stop) : stop.name}
                  </span>
                  <span className="truncate text-[var(--ink-2)] text-xs tabular-nums">
                    {stopLabel(index, STOPS.length)}
                    {index === 0
                      ? ""
                      : ` · ${formatDistance(stop.at)} · ${formatDuration(roundToMinute(secondsTo(stop)))}`}
                  </span>
                </span>
                <IconGripVertical
                  size={15}
                  className="shrink-0 cursor-grab text-[var(--ink-2)]"
                  aria-hidden="true"
                />
                <Button
                  variant="ghost"
                  icon={<IconTrash size={15} />}
                  aria-label={`Delete ${stopLabel(index, STOPS.length)}`}
                />
              </li>
            ))}
          </ul>
        </Panel>
        <div className="sticky bottom-0 flex gap-2">
          <Button icon={<IconDeviceFloppy size={16} stroke={1.6} />} className="flex-1">
            Save changes
          </Button>
          <Button variant="outline">Publish</Button>
        </div>
      </div>
    </Stage>
  );
}

/* ── D. Toolbar ───────────────────────────────────────────────────────────── */

function VariantD() {
  const [profile, setProfile] = useState<Profile>("gravel");
  const [state, setState] = useState<"draft" | "published">("draft");
  return (
    <Stage>
      <div className="flex flex-col gap-2.5 border-[var(--rule)] border-b p-3">
        <div className="flex items-center gap-2">
          <input
            aria-label="Plan name"
            defaultValue="Turmberg loop"
            className={`min-w-0 flex-1 rounded-[9px] ${WASH} px-2.5 py-1.5 font-medium text-sm outline-none focus:outline-2 focus:outline-[var(--accent)]`}
          />
          <Button
            variant="ghost"
            icon={<IconChevronDown size={16} stroke={2} />}
            aria-label="Plans"
          />
        </div>
        <Segmented
          label="Route type"
          size="sm"
          items={PROFILES}
          value={profile}
          onChange={setProfile}
        />
      </div>
      <div className="flex flex-1 flex-col gap-1 p-3">
        {STOPS.map((stop, index) => (
          <div
            key={stop.id}
            className="group flex items-center gap-2.5 rounded-[9px] px-2 py-1.5 hover:bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]"
          >
            <StopMark index={index} count={STOPS.length} size={22} />
            <span className="min-w-0 flex-1 truncate text-sm">{coordinates(stop)}</span>
            <span className="shrink-0 text-[var(--ink-2)] text-xs tabular-nums">
              {formatDistance(stop.at)}
            </span>
            <Button
              variant="ghost"
              icon={<IconTrash size={15} />}
              aria-label={`Delete ${stopLabel(index, STOPS.length)}`}
              className="opacity-0 group-hover:opacity-100"
            />
          </div>
        ))}
        <Button variant="ghost" icon={<IconPlus size={15} />} className="mt-1 justify-start">
          Click the map to add a stop
        </Button>
      </div>
      <div className="sticky bottom-0 flex flex-col gap-2 border-[var(--rule)] border-t bg-[var(--panel)] p-3">
        <div className={`flex items-center justify-between rounded-xl ${WASH} px-3.5 py-2.5`}>
          <Figures dense />
        </div>
        <div className="flex items-center gap-2">
          <Segmented
            label="Plan state"
            size="sm"
            items={STATE}
            value={state}
            onChange={setState}
            className="flex-1"
          />
          <Button icon={<IconDeviceFloppy size={16} stroke={1.6} />}>Save</Button>
        </div>
      </div>
    </Stage>
  );
}

const meta = {
  title: "Spikes/Planner Sidebar",
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <div className="bg-[var(--base)] p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const A_Brief: Story = { name: "A · Brief", render: () => <VariantA /> };
export const B_Itinerary: Story = { name: "B · Itinerary", render: () => <VariantB /> };
export const C_Cards: Story = { name: "C · Cards", render: () => <VariantC /> };
export const D_Toolbar: Story = { name: "D · Toolbar", render: () => <VariantD /> };
