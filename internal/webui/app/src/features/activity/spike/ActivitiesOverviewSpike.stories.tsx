/**
 * Three wide layouts for the activities overview in the dashboard's language:
 * a figure strip, segmented filters, panels, inset lists and olive/slate grounds.
 * Storybook only, over the index spike's synthetic rides.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { IconBike, IconCloudRain, IconHeartbeat, IconRoute } from "@tabler/icons-react";
import { type ReactNode, useMemo, useState } from "react";
import { Panel } from "../../../components/PanelHeading";
import { RideCalendar } from "../../../components/RideCalendar";
import { RouteGlyph } from "../../../components/RouteGlyph";
import { Segmented } from "../../../components/Segmented";
import { formatAscent, formatDistance, formatDuration } from "../../../lib/format";
import { temperatureColour, weatherIcon } from "../../../lib/weather";
import { RIDES as BASE, type IndexRide } from "./indexData";

const RIDES: IndexRide[] = BASE.map((ride, index) =>
  index % 5 === 2 ? { ...ride, indoor: true, provider: "zwift", weather: undefined } : ride,
) as IndexRide[];

type Ground = "all" | "outdoor" | "indoor";
const GROUNDS = [
  { key: "all", label: "All" },
  { key: "outdoor", label: "Outdoor" },
  { key: "indoor", label: "Indoor" },
] as const;

const WASH = "bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]";
const glyph = (Icon: typeof IconRoute) => <Icon size={18} stroke={1.8} />;
const groundColour = (ride: IndexRide) =>
  ride.indoor ? "var(--ground-indoor)" : "var(--ground-outdoor)";

function fmt(iso: string, options: Intl.DateTimeFormatOptions) {
  return new Date(iso).toLocaleString("en-GB", { timeZone: "Europe/Berlin", ...options });
}

function mondayOf(iso: string): string {
  const date = new Date(iso);
  const day = (date.getUTCDay() + 6) % 7;
  return new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate() - day))
    .toISOString()
    .slice(0, 10);
}

function totals(rides: IndexRide[]) {
  return rides.reduce(
    (sum, ride) => ({
      distance: sum.distance + ride.distanceMetres,
      moving: sum.moving + ride.movingSeconds,
      ascent: sum.ascent + ride.ascentMetres,
      count: sum.count + 1,
    }),
    { distance: 0, moving: 0, ascent: 0, count: 0 },
  );
}

function groupBy(rides: IndexRide[], key: (ride: IndexRide) => string) {
  const groups = new Map<string, IndexRide[]>();
  for (const ride of rides) {
    groups.set(key(ride), [...(groups.get(key(ride)) ?? []), ride]);
  }
  return [...groups.entries()];
}

function useFiltered() {
  const [ground, setGround] = useState<Ground>("all");
  const rides = useMemo(
    () =>
      RIDES.filter((ride) =>
        ground === "all" ? true : ground === "indoor" ? ride.indoor : !ride.indoor,
      ),
    [ground],
  );
  return { ground, setGround, rides };
}

function Header({ ground, onGround }: { ground: Ground; onGround: (next: Ground) => void }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <h1 className="font-semibold text-2xl tracking-tight">Activities</h1>
      <Segmented label="Rides" size="sm" items={GROUNDS} value={ground} onChange={onGround} />
    </div>
  );
}

function Weather({ ride }: { ride: IndexRide }) {
  if (!ride.weather) {
    return null;
  }
  const Glyph = weatherIcon(ride.weather.weatherCode);
  return (
    <span className="inline-flex items-center gap-1 tabular-nums">
      <Glyph size={13} stroke={1.8} aria-hidden="true" />
      <span style={{ color: temperatureColour(ride.weather.temperatureMaxCelsius) }}>
        {Math.round(ride.weather.temperatureMaxCelsius)}°
      </span>
      {ride.weather.precipitationMillimetres > 0 ? (
        <span className="inline-flex items-center gap-0.5 text-[var(--rain-2)]">
          <IconCloudRain size={12} aria-hidden="true" />
          {ride.weather.precipitationMillimetres} mm
        </span>
      ) : null}
    </span>
  );
}

function GroundDot({ ride }: { ride: IndexRide }) {
  return (
    <span
      className="inline-block size-2 shrink-0 rounded-full"
      style={{ background: groundColour(ride) }}
      title={ride.indoor ? "Indoor" : "Outdoor"}
    />
  );
}

function Thumb({ ride, size = "size-10" }: { ride: IndexRide; size?: string }) {
  return ride.indoor ? (
    <span
      className={`grid ${size} shrink-0 place-items-center rounded-[9px] text-[var(--panel)]`}
      style={{ background: "var(--ground-indoor)" }}
    >
      <IconBike size={18} stroke={1.8} aria-hidden="true" />
    </span>
  ) : (
    <span className={`${size} shrink-0 overflow-hidden rounded-[9px] bg-[var(--panel)] p-1`}>
      <RouteGlyph coordinates={ride.coordinates} title="ride" band={ride.band} />
    </span>
  );
}

/** B · Week ledger: one inset block per week, a row per ride, the week's totals in its heading. */
function Ledger() {
  const { ground, setGround, rides } = useFiltered();
  const weeks = groupBy(rides, (ride) => mondayOf(ride.startedAt));
  return (
    <Frame>
      <Header ground={ground} onGround={setGround} />
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_22rem]">
        <Panel icon={glyph(IconBike)} title="Rides" subtitle="newest first">
          <div className="flex flex-col gap-5">
            {weeks.map(([monday, week]) => {
              const sum = totals(week);
              return (
                <section key={monday} className="flex flex-col gap-2">
                  <h4 className="flex items-baseline justify-between px-1 text-sm">
                    <span className="font-semibold">
                      Week of {fmt(`${monday}T12:00:00Z`, { day: "numeric", month: "short" })}
                    </span>
                    <span className="text-[var(--ink-2)] text-xs tabular-nums">
                      {formatDistance(sum.distance)} · {formatDuration(sum.moving)} ·{" "}
                      {formatAscent(sum.ascent)}
                    </span>
                  </h4>
                  <ul className={`flex flex-col overflow-hidden rounded-xl ${WASH}`}>
                    {week.map((ride) => (
                      <li
                        key={ride.id}
                        className="flex items-center gap-3 border-[var(--panel)] border-b-2 px-3.5 py-2.5 last:border-b-0 hover:bg-[color-mix(in_oklab,var(--ink-2)_5%,transparent)]"
                      >
                        <Thumb ride={ride} />
                        <div className="flex min-w-0 flex-1 flex-col">
                          <span className="flex items-center gap-2 font-semibold text-sm">
                            {fmt(ride.startedAt, { weekday: "long" })}{" "}
                            {Number(fmt(ride.startedAt, { hour: "numeric", hour12: false })) < 12
                              ? "morning"
                              : "afternoon"}{" "}
                            ride
                          </span>
                          <span className="flex flex-wrap items-center gap-x-2 text-[var(--ink-2)] text-xs">
                            <span>
                              {fmt(ride.startedAt, {
                                day: "numeric",
                                month: "short",
                                hour: "2-digit",
                                minute: "2-digit",
                              })}
                            </span>
                            <span>{formatDuration(ride.movingSeconds)}</span>
                            <span>{formatAscent(ride.ascentMetres)}</span>
                            {ride.metrics?.heartRateTss ? (
                              <span className="inline-flex items-center gap-0.5">
                                <IconHeartbeat size={12} aria-hidden="true" />
                                {ride.metrics.heartRateTss} TSS
                              </span>
                            ) : null}
                            <Weather ride={ride} />
                          </span>
                        </div>
                        <span className="font-semibold text-lg tabular-nums tracking-tight">
                          {formatDistance(ride.distanceMetres)}
                        </span>
                      </li>
                    ))}
                  </ul>
                </section>
              );
            })}
          </div>
        </Panel>
        <div className="flex flex-col gap-4">
          <RideCalendar rides={rides} zone="Europe/Berlin" />
        </div>
      </div>
    </Frame>
  );
}

/** C · Cards: a month heading, then a card per ride with its shape large and its distance as the figure. */
function Cards() {
  const { ground, setGround, rides } = useFiltered();
  const months = groupBy(rides, (ride) => fmt(ride.startedAt, { month: "long", year: "numeric" }));
  return (
    <Frame>
      <Header ground={ground} onGround={setGround} />
      {months.map(([month, list]) => {
        const sum = totals(list);
        return (
          <section key={month} className="flex flex-col gap-3">
            <h2 className="flex items-baseline gap-3 px-1">
              <span className="font-semibold text-lg">{month}</span>
              <span className="text-[var(--ink-2)] text-sm tabular-nums">
                {sum.count} rides · {formatDistance(sum.distance)} · {formatAscent(sum.ascent)}
              </span>
            </h2>
            <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-5">
              {list.map((ride) => (
                <a
                  key={ride.id}
                  href="#ride"
                  className="flex flex-col gap-3 rounded-2xl bg-[var(--panel)] p-4 shadow-[var(--shadow)] hover:-translate-y-0.5 hover:shadow-[0_0_0_1px_var(--rule),var(--shadow)]"
                >
                  <div
                    className={`grid aspect-[4/3] place-items-center overflow-hidden rounded-xl ${WASH} p-3`}
                  >
                    {ride.indoor ? (
                      <IconBike size={36} stroke={1.4} style={{ color: "var(--ground-indoor)" }} />
                    ) : (
                      <RouteGlyph coordinates={ride.coordinates} title="ride" band={ride.band} />
                    )}
                  </div>
                  <div className="flex items-baseline justify-between gap-2">
                    <span className="font-semibold text-xl tabular-nums tracking-tight">
                      {formatDistance(ride.distanceMetres)}
                    </span>
                    <GroundDot ride={ride} />
                  </div>
                  <div className="flex flex-col gap-0.5 text-[var(--ink-2)] text-xs">
                    <span>
                      {fmt(ride.startedAt, { weekday: "short", day: "numeric", month: "short" })}
                    </span>
                    <span className="flex flex-wrap items-center gap-x-2">
                      {formatDuration(ride.movingSeconds)} · {formatAscent(ride.ascentMetres)}
                      <Weather ride={ride} />
                    </span>
                  </div>
                </a>
              ))}
            </div>
          </section>
        );
      })}
    </Frame>
  );
}

/** D · Calendar and rail: weeks as rows of day cells sized by distance; choosing a week lists its rides beside. */
function Calendar() {
  const { ground, setGround, rides } = useFiltered();
  const weeks = groupBy(rides, (ride) => mondayOf(ride.startedAt));
  const [selected, setSelected] = useState(weeks[0]?.[0] ?? "");
  const longest = Math.max(...rides.map((ride) => ride.distanceMetres), 1);
  const chosen = weeks.find(([monday]) => monday === selected)?.[1] ?? [];
  return (
    <Frame>
      <Header ground={ground} onGround={setGround} />
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_24rem]">
        <Panel icon={glyph(IconBike)} title="Calendar" subtitle="choose a week">
          <div className="grid grid-cols-[6rem_repeat(7,minmax(0,1fr))_6rem] gap-1.5 text-xs">
            <span />
            {["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"].map((day) => (
              <span key={day} className="text-center text-[var(--ink-2)]">
                {day}
              </span>
            ))}
            <span className="text-right text-[var(--ink-2)]">Total</span>
            {weeks.map(([monday, week]) => {
              const active = monday === selected;
              return (
                <Row key={monday} active={active} onClick={() => setSelected(monday)}>
                  <span className="self-center font-medium">
                    {fmt(`${monday}T12:00:00Z`, { day: "numeric", month: "short" })}
                  </span>
                  {Array.from({ length: 7 }, (_, day) => {
                    const onDay = week.filter(
                      (ride) => (new Date(ride.startedAt).getUTCDay() + 6) % 7 === day,
                    );
                    const distance = totals(onDay).distance;
                    const size = distance ? 30 + (distance / longest) * 70 : 0;
                    return (
                      <span
                        // biome-ignore lint/suspicious/noArrayIndexKey: the seven weekdays never reorder
                        key={day}
                        className={`grid h-12 place-items-center rounded-[9px] ${WASH}`}
                      >
                        {distance ? (
                          <span
                            className="rounded-[6px]"
                            style={{
                              width: `${size * 0.4}px`,
                              height: `${size * 0.4}px`,
                              background: onDay.some((ride) => ride.indoor)
                                ? "var(--ground-indoor)"
                                : "var(--ground-outdoor)",
                            }}
                          />
                        ) : null}
                      </span>
                    );
                  })}
                  <span className="self-center text-right font-semibold tabular-nums">
                    {formatDistance(totals(week).distance)}
                  </span>
                </Row>
              );
            })}
          </div>
        </Panel>
        <div className="flex flex-col gap-4">
          <RideCalendar rides={rides} zone="Europe/Berlin" />
          <Panel
            icon={glyph(IconRoute)}
            title={`Week of ${fmt(`${selected}T12:00:00Z`, { day: "numeric", month: "short" })}`}
          >
            <ul className={`flex flex-col overflow-hidden rounded-xl ${WASH}`}>
              {chosen.map((ride) => (
                <li
                  key={ride.id}
                  className="flex items-center gap-3 border-[var(--panel)] border-b-2 px-3 py-2.5 last:border-b-0"
                >
                  <Thumb ride={ride} size="size-9" />
                  <div className="flex min-w-0 flex-1 flex-col">
                    <span className="font-semibold text-sm">
                      {fmt(ride.startedAt, { weekday: "long" })}
                    </span>
                    <span className="text-[var(--ink-2)] text-xs">
                      {formatDuration(ride.movingSeconds)} · {formatAscent(ride.ascentMetres)}
                    </span>
                  </div>
                  <span className="font-semibold tabular-nums">
                    {formatDistance(ride.distanceMetres)}
                  </span>
                </li>
              ))}
            </ul>
          </Panel>
        </div>
      </div>
    </Frame>
  );
}

function Row({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={`col-span-9 grid grid-cols-subgrid gap-1.5 rounded-xl p-1 text-left ${active ? "bg-[var(--panel)] shadow-[0_0_0_1px_var(--rule),var(--shadow)]" : "hover:bg-[color-mix(in_oklab,var(--ink-2)_5%,transparent)]"}`}
    >
      {children}
    </button>
  );
}

function Frame({ children }: { children: ReactNode }) {
  return <div className="flex min-h-screen flex-col gap-4 bg-[var(--base)] p-6">{children}</div>;
}

const meta = {
  title: "Spikes/Activities Overview",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const B_WeekLedger: Story = { render: () => <Ledger /> };
export const C_Cards: Story = { render: () => <Cards /> };
export const D_CalendarAndRail: Story = { render: () => <Calendar /> };

type RowStyle = "today" | "table" | "stats" | "bar";

const LONGEST = Math.max(...RIDES.map((ride) => ride.distanceMetres));

function Stat({ label, value, wide = "w-20" }: { label: string; value: ReactNode; wide?: string }) {
  return (
    <span className={`flex ${wide} flex-col items-end`}>
      <span className="font-semibold text-sm tabular-nums">{value}</span>
      <span className="text-[10px] text-[var(--ink-2)] uppercase tracking-wide">{label}</span>
    </span>
  );
}

function Tss({ ride }: { ride: IndexRide }) {
  return ride.metrics?.heartRateTss ? <>{ride.metrics.heartRateTss}</> : <>–</>;
}

function RideRow({ ride, style }: { ride: IndexRide; style: RowStyle }) {
  const tile = (
    <span
      className="grid size-7 shrink-0 place-items-center rounded-[7px] text-[var(--panel)]"
      style={{ background: groundColour(ride) }}
    >
      <IconBike size={15} stroke={1.8} aria-hidden="true" />
    </span>
  );
  const day = fmt(ride.startedAt, { weekday: "short", day: "numeric", month: "short" });
  const time = fmt(ride.startedAt, { hour: "2-digit", minute: "2-digit" });

  if (style === "table") {
    return (
      <div className="grid grid-cols-[1.75rem_7rem_3.5rem_minmax(0,1fr)_5rem_5rem_4.5rem_3.5rem_4rem] items-center gap-x-3 px-3 py-1.5 text-sm tabular-nums">
        {tile}
        <span className="font-medium">{day}</span>
        <span className="text-[var(--ink-2)]">{time}</span>
        <span className="truncate text-[var(--ink-2)]">
          {ride.indoor ? "Zwift" : fmt(ride.startedAt, { weekday: "long" })} ride
        </span>
        <span className="text-right font-semibold">{formatDistance(ride.distanceMetres)}</span>
        <span className="text-right">{formatDuration(ride.movingSeconds)}</span>
        <span className="text-right">{formatAscent(ride.ascentMetres)}</span>
        <span className="text-right text-[var(--ink-2)]">
          <Tss ride={ride} />
        </span>
        <span className="flex justify-end text-xs">
          <Weather ride={ride} />
        </span>
      </div>
    );
  }
  if (style === "stats") {
    return (
      <div className="flex items-center gap-3 px-3 py-2">
        {tile}
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="font-semibold text-sm">
            {fmt(ride.startedAt, { weekday: "long" })} ride
          </span>
          <span className="flex items-center gap-2 text-[var(--ink-2)] text-xs">
            {fmt(ride.startedAt, { day: "numeric", month: "short" })} · {time}
            <Weather ride={ride} />
          </span>
        </span>
        <Stat label="Time" value={formatDuration(ride.movingSeconds)} />
        <Stat label="Ascent" value={formatAscent(ride.ascentMetres)} wide="w-16" />
        <Stat label="TSS" value={<Tss ride={ride} />} wide="w-12" />
        <Stat label="Distance" value={formatDistance(ride.distanceMetres)} wide="w-20" />
      </div>
    );
  }
  if (style === "bar") {
    return (
      <div className="relative flex items-center gap-3 px-3 py-1.5">
        <span
          aria-hidden="true"
          className="absolute inset-y-1 left-1 rounded-[7px] opacity-15"
          style={{
            width: `calc(${(ride.distanceMetres / LONGEST) * 100}% - 0.5rem)`,
            background: groundColour(ride),
          }}
        />
        <span className="relative flex min-w-0 flex-1 items-center gap-3 text-sm">
          {tile}
          <span className="w-24 font-medium">{day}</span>
          <span className="font-semibold tabular-nums">{formatDistance(ride.distanceMetres)}</span>
          <span className="flex items-center gap-x-3 text-[var(--ink-2)] text-xs tabular-nums">
            <span>{formatDuration(ride.movingSeconds)}</span>
            <span>{formatAscent(ride.ascentMetres)}</span>
            {ride.metrics?.heartRateTss ? <span>{ride.metrics.heartRateTss} TSS</span> : null}
            <Weather ride={ride} />
          </span>
        </span>
        <span className="relative text-[var(--ink-2)] text-xs tabular-nums">{time}</span>
      </div>
    );
  }
  return (
    <div className="flex items-center gap-3 px-3.5 py-2.5">
      <Thumb ride={ride} size="size-9" />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="font-semibold text-sm">
          {fmt(ride.startedAt, { weekday: "long" })} ride
        </span>
        <span className="flex flex-wrap items-center gap-x-2 text-[var(--ink-2)] text-xs">
          <span>{day}</span>
          <span>{formatDuration(ride.movingSeconds)}</span>
          <span>{formatAscent(ride.ascentMetres)}</span>
          {ride.metrics?.heartRateTss ? (
            <span className="inline-flex items-center gap-0.5">
              <IconHeartbeat size={12} aria-hidden="true" />
              {ride.metrics.heartRateTss} TSS
            </span>
          ) : null}
          <Weather ride={ride} />
        </span>
      </span>
      <span className="font-semibold text-lg tabular-nums tracking-tight">
        {formatDistance(ride.distanceMetres)}
      </span>
    </div>
  );
}

function RowSpike({ style }: { style: RowStyle }) {
  const weeks = groupBy(RIDES, (ride) => mondayOf(ride.startedAt)).slice(0, 4);
  return (
    <Frame>
      <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_22rem]">
        <Panel icon={glyph(IconBike)} title="Rides" subtitle="newest first">
          <div className="flex flex-col gap-4">
            {weeks.map(([monday, week]) => {
              const sum = totals(week);
              return (
                <section key={monday} className="flex flex-col gap-1.5">
                  <h4 className="flex items-baseline justify-between px-1 text-sm">
                    <span className="font-semibold">
                      {fmt(`${monday}T12:00:00Z`, { day: "numeric", month: "short" })}
                    </span>
                    <span className="text-[var(--ink-2)] text-xs tabular-nums">
                      {formatDistance(sum.distance)} · {formatDuration(sum.moving)} ·{" "}
                      {formatAscent(sum.ascent)}
                    </span>
                  </h4>
                  <div className={`flex flex-col overflow-hidden rounded-xl ${WASH}`}>
                    {week.map((ride) => (
                      <div
                        key={ride.id}
                        className="border-[var(--panel)] border-b-2 last:border-b-0 hover:bg-[color-mix(in_oklab,var(--ink-2)_5%,transparent)]"
                      >
                        <RideRow ride={ride} style={style} />
                      </div>
                    ))}
                  </div>
                </section>
              );
            })}
          </div>
        </Panel>
        <RideCalendar rides={RIDES} zone="Europe/Berlin" />
      </div>
    </Frame>
  );
}

/** Rows A · Today's two-line row, for comparison. */
export const Rows_A_Today: Story = { render: () => <RowSpike style="today" /> };
/** Rows B · One line per ride, figures in aligned table columns across the width. */
export const Rows_B_Table: Story = { render: () => <RowSpike style="table" /> };
/** Rows C · Name and date left, labelled figure columns right. */
export const Rows_C_Stats: Story = { render: () => <RowSpike style="stats" /> };
/** Rows D · One line with a faint bar behind it, as long as the ride. */
export const Rows_D_DistanceBar: Story = { render: () => <RowSpike style="bar" /> };

type PickerStyle = "solid" | "tint" | "soft" | "dot";

const OUT = "var(--ground-outdoor)";
const IN = "var(--ground-indoor)";

/** What a chosen segment is painted with: one ground's colour, or both split on the diagonal. */
function paint(key: Ground, share: number): string {
  const mix = (colour: string) =>
    share === 100 ? colour : `color-mix(in oklab, ${colour} ${share}%, var(--panel))`;
  return key === "all"
    ? `linear-gradient(135deg, ${mix(OUT)} 0 50%, ${mix(IN)} 50% 100%)`
    : mix(key === "indoor" ? IN : OUT);
}

function GroundPicker({ style }: { style: PickerStyle }) {
  const [value, setValue] = useState<Ground>("outdoor");
  const labels: Record<Ground, string> = { all: "Both", outdoor: "Outdoor", indoor: "Indoor" };

  return (
    <div
      className="inline-flex gap-[3px] self-start rounded-[12px] bg-[var(--muted)] p-[3px]"
      role="group"
      aria-label="Rides"
    >
      {(["all", "outdoor", "indoor"] as const).map((key) => {
        const chosen = key === value;
        const dot = (
          <span
            aria-hidden="true"
            className="size-2.5 rounded-full"
            style={{ background: paint(key, 100) }}
          />
        );
        return (
          <button
            key={key}
            type="button"
            aria-pressed={chosen}
            onClick={() => setValue(key)}
            className={`flex h-7 items-center gap-1.5 rounded-[9px] px-3 text-sm transition-colors ${chosen ? "font-semibold" : "text-[var(--ink-2)] hover:text-[var(--ink)]"}`}
            style={
              !chosen
                ? undefined
                : style === "solid"
                  ? {
                      background: paint(key, 100),
                      color: "white",
                      textShadow: "0 1px 1px rgb(0 0 0 / 0.25)",
                    }
                  : style === "tint"
                    ? {
                        background: paint(key, 22),
                        color: "var(--ink)",
                        boxShadow: "0 0 0 1px var(--rule)",
                      }
                    : style === "soft"
                      ? {
                          background: "var(--panel)",
                          boxShadow: `inset 0 -3px 0 0 ${key === "indoor" ? IN : OUT}, 0 0 0 1px var(--rule), var(--shadow)`,
                        }
                      : {
                          background: "var(--panel)",
                          boxShadow: "0 0 0 1px var(--rule), var(--shadow)",
                        }
            }
          >
            {style === "dot" ? dot : null}
            {labels[key]}
          </button>
        );
      })}
    </div>
  );
}

const PICKERS: { label: string; style: PickerStyle }[] = [
  {
    label: "B · Solid: the chosen segment filled with its ground, Both split on the diagonal",
    style: "solid",
  },
  { label: "C · Tint: a pale wash of the ground, Both split pale", style: "tint" },
  {
    label: "D · Underline: the white thumb kept, a ground-coloured edge beneath it",
    style: "soft",
  },
  { label: "E · Dots: the white thumb kept, every segment led by its ground's dot", style: "dot" },
];

/** The ground selector painted with the ground colours. Today's is the plain white thumb. */
export const GroundSelector: Story = {
  render: () => (
    <div className="flex min-h-screen flex-col gap-6 bg-[var(--base)] p-8">
      <div className="flex flex-col gap-2">
        <span className="text-[var(--ink-2)] text-xs">A · Today</span>
        <Segmented label="Rides" size="md" items={GROUNDS} value="outdoor" onChange={() => {}} />
      </div>
      {PICKERS.map(({ label, style }) => (
        <div key={style} className="flex flex-col gap-2">
          <span className="text-[var(--ink-2)] text-xs">{label}</span>
          <GroundPicker style={style} />
        </div>
      ))}
    </div>
  ),
};

type CalendarLook = "squares" | "light" | "heatmap" | "bars";

/** August 2026 of the synthetic rides, laid out Monday first, with a few days ridden on both grounds. */
function monthCells() {
  const ridden = new Map<number, CalendarGround>();
  for (const ride of RIDES) {
    const date = new Date(ride.startedAt);
    if (date.getUTCMonth() === 7 && date.getUTCFullYear() === 2026) {
      const day = date.getUTCDate();
      const ground = ride.indoor ? "indoor" : "outdoor";
      const before = ridden.get(day);
      ridden.set(day, before && before !== ground ? "both" : ground);
    }
  }
  // Synthetic data rarely doubles up, so three days get a second ride on the other ground.
  for (const day of [8, 15, 22]) {
    if (ridden.has(day)) {
      ridden.set(day, "both");
    }
  }
  const lead = (new Date(Date.UTC(2026, 7, 1)).getUTCDay() + 6) % 7;
  return { ridden, lead, length: 31 };
}

type CalendarGround = "outdoor" | "indoor" | "both";

/** One ground's colour, or both split on the diagonal as the ground selector does. */
function dayPaint(ground: CalendarGround): string {
  return ground === "both"
    ? `linear-gradient(135deg, ${OUT} 0 50%, ${IN} 50% 100%)`
    : ground === "outdoor"
      ? OUT
      : IN;
}

function CalendarSpike({ look }: { look: CalendarLook }) {
  const { ridden, lead, length } = monthCells();
  const dark = look !== "light";

  return (
    <section
      className={`flex w-[22rem] flex-col gap-4 rounded-2xl p-5 shadow-[var(--shadow)] ${dark ? "bg-[radial-gradient(circle_at_30%_0%,#4a4a4a,#333_70%)] text-white" : "bg-[var(--panel)]"}`}
    >
      <div className="flex items-center gap-3">
        <span
          className={`grid size-9 place-items-center rounded-[9px] ${dark ? "bg-white text-[#333]" : "bg-[radial-gradient(circle_at_50%_35%,#6e6e6e,#3d3d3d_85%)] text-[var(--panel)]"}`}
        >
          <IconBike size={18} stroke={1.8} aria-hidden="true" />
        </span>
        <h3 className="flex-1 font-semibold">August 2026</h3>
      </div>
      <div
        className={`grid grid-cols-7 text-center text-xs ${look === "heatmap" ? "gap-1" : "gap-1.5"}`}
      >
        {["M", "T", "W", "T", "F", "S", "S"].map((letter, index) => (
          // biome-ignore lint/suspicious/noArrayIndexKey: the weekday letters repeat and never reorder
          <span key={index} className={`pb-1 ${dark ? "text-white/60" : "text-[var(--ink-2)]"}`}>
            {letter}
          </span>
        ))}
        {Array.from({ length: lead }, (_, index) => (
          // biome-ignore lint/suspicious/noArrayIndexKey: blank leading cells
          <span key={`lead-${index}`} />
        ))}
        {Array.from({ length }, (_, index) => {
          const date = index + 1;
          const ground = ridden.get(date);
          const fill = ground ? { background: dayPaint(ground) } : undefined;
          if (look === "squares") {
            return (
              <span
                key={date}
                className={`grid aspect-square place-items-center rounded-[9px] tabular-nums ${ground ? "font-semibold text-white" : "bg-white/6 text-white/45"}`}
                style={fill}
              >
                {date}
              </span>
            );
          }
          if (look === "light") {
            return (
              <span
                key={date}
                className={`grid aspect-square place-items-center rounded-[9px] tabular-nums ${ground ? "font-semibold text-white" : "bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] text-[var(--ink-2)]"}`}
                style={fill}
              >
                {date}
              </span>
            );
          }
          if (look === "heatmap") {
            return (
              <span
                key={date}
                title={`${date} August`}
                className={`aspect-square rounded-[5px] ${ground ? "" : "bg-white/8"}`}
                style={fill}
              />
            );
          }
          return (
            <span
              key={date}
              className={`flex aspect-square flex-col items-center justify-center gap-1 rounded-[9px] tabular-nums ${ground ? "bg-white/10 font-semibold text-white" : "text-white/45"}`}
            >
              {date}
              {ground === "both" ? (
                <span className="flex gap-0.5">
                  <span className="h-1 w-2 rounded-full" style={{ background: OUT }} />
                  <span className="h-1 w-2 rounded-full" style={{ background: IN }} />
                </span>
              ) : (
                <span
                  className="h-1 w-4 rounded-full"
                  style={{ background: ground ? dayPaint(ground) : "transparent" }}
                />
              )}
            </span>
          );
        })}
      </div>
    </section>
  );
}

const LOOKS: { label: string; look: CalendarLook }[] = [
  {
    label: "B · Squares: the 9px corner every control has, empty days a faint tile",
    look: "squares",
  },
  {
    label: "C · Light card: a panel like every other card, filled squares on the wash",
    look: "light",
  },
  {
    label: "D · Heatmap: small squares without numbers, the month read as a pattern",
    look: "heatmap",
  },
  {
    label: "E · Bars: numbers kept plain, a ground-coloured bar beneath a ridden day",
    look: "bars",
  },
];

/** Alternatives to the round days. Today's rounds are the live component, shown first. */
export const CalendarLooks: Story = {
  render: () => (
    <div className="flex min-h-screen flex-wrap items-start gap-6 bg-[var(--base)] p-8">
      <div className="flex flex-col gap-2">
        <span className="text-[var(--ink-2)] text-xs">A · Today</span>
        <div className="w-[22rem]">
          <RideCalendar rides={RIDES} zone="UTC" />
        </div>
      </div>
      {LOOKS.map(({ label, look }) => (
        <div key={look} className="flex w-[22rem] flex-col gap-2">
          <span className="text-[var(--ink-2)] text-xs">{label}</span>
          <CalendarSpike look={look} />
        </div>
      ))}
    </div>
  ),
};
