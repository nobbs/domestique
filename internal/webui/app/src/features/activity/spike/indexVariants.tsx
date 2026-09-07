/**
 * Three ways the activities index could read.
 *
 * Today it is a column of identical cards, each a date and two lines of
 * figures. Each variant below decides what the list is *for* — looking a ride
 * up, browsing what was ridden, or seeing the rhythm of the weeks — and lets
 * the shape follow.
 */

import { type ReactNode, useMemo } from "react";
import { RouteGlyph } from "../../../components/RouteGlyph";
import { formatAscent, formatDistance, formatDuration } from "../../../lib/format";
import { temperatureColour, weatherIcon } from "../../../lib/weather";
import { type IndexRide, RIDES } from "./indexData";

const WEEKDAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
const WEEK_COLUMNS = "11rem repeat(7, minmax(0, 1fr))";

function dayOf(iso: string): number {
  return (new Date(iso).getUTCDay() + 6) % 7;
}

function label(iso: string, options: Intl.DateTimeFormatOptions): string {
  return new Intl.DateTimeFormat("en-GB", { ...options, timeZone: "UTC" }).format(new Date(iso));
}

function Label({ children }: { children: ReactNode }) {
  return (
    <span className="font-semibold text-[10px] text-[var(--ink-2)] uppercase tracking-[0.08em]">
      {children}
    </span>
  );
}

function Weather({ ride, temperature = true }: { ride: IndexRide; temperature?: boolean }) {
  if (!ride.weather) {
    return null;
  }
  const Glyph = weatherIcon(ride.weather.weatherCode);

  return (
    <span className="flex items-center gap-1 text-xs tabular-nums">
      <Glyph size={16} stroke={1.6} aria-hidden="true" className="text-[var(--ink-2)]" />
      {temperature ? (
        <span style={{ color: temperatureColour(ride.weather.temperatureMaxCelsius) }}>
          {Math.round(ride.weather.temperatureMaxCelsius)}°
        </span>
      ) : null}
      {ride.weather.precipitationMillimetres > 0 ? (
        <span className="text-[var(--rain-2)]">{ride.weather.precipitationMillimetres} mm</span>
      ) : null}
    </span>
  );
}

interface Group {
  key: string;
  rides: IndexRide[];
  distance: number;
  moving: number;
  ascent: number;
  tss: number;
}

function groupBy(rides: IndexRide[], keyOf: (ride: IndexRide) => string): Group[] {
  const groups: Group[] = [];
  for (const ride of rides) {
    const key = keyOf(ride);
    let group = groups.find((one) => one.key === key);
    if (!group) {
      group = { key, rides: [], distance: 0, moving: 0, ascent: 0, tss: 0 };
      groups.push(group);
    }
    group.rides.push(ride);
    group.distance += ride.distanceMetres;
    group.moving += ride.movingSeconds;
    group.ascent += ride.ascentMetres;
    group.tss += ride.metrics?.heartRateTss ?? 0;
  }
  return groups;
}

/** The Monday a ride's week starts on, as a key that sorts. */
function weekOf(ride: IndexRide): string {
  const at = new Date(ride.startedAt);
  at.setUTCDate(at.getUTCDate() - dayOf(ride.startedAt));
  return at.toISOString().slice(0, 10);
}

function Totals({ group, size = "sm" }: { group: Group; size?: "sm" | "md" }) {
  const value = size === "md" ? "text-lg" : "text-sm";
  return (
    <dl className="flex gap-4 tabular-nums">
      {[
        [formatDistance(group.distance), "ridden"],
        [formatDuration(group.moving), "moving"],
        [formatAscent(group.ascent), "climbed"],
        [group.tss ? String(group.tss) : "", "hrTSS"],
      ]
        .filter(([figure]) => figure)
        .map(([figure, name]) => (
          <div key={name} className="flex flex-col">
            <span className={`font-semibold ${value}`}>{figure}</span>
            <span className="text-[10px] text-[var(--ink-2)]">{name}</span>
          </div>
        ))}
    </dl>
  );
}

/* --------------------------------------------------------------- A: Ledger */

/**
 * The bet: the index is a training log and a log is a table. One ride per
 * row with every figure in its own column, months as the only rules, and each
 * month's totals in its header so the log adds up as it is read.
 */
export function LedgerIndex() {
  const months = useMemo(
    () => groupBy(RIDES, (ride) => label(ride.startedAt, { month: "long", year: "numeric" })),
    [],
  );

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-6">
      <h1 className="font-semibold text-2xl tracking-tight">Activities</h1>
      {months.map((month) => (
        <section key={month.key} className="flex flex-col gap-2">
          <div className="flex items-end justify-between gap-4 px-2">
            <h2 className="font-medium">{month.key}</h2>
            <Totals group={month} />
          </div>
          <div className="overflow-x-auto rounded-xl bg-[var(--panel)] ring-1 ring-black/5">
            <table className="w-full border-collapse text-sm">
              <thead>
                <tr className="text-[var(--ink-2)] text-xs">
                  <th className="px-3 py-2 text-left font-normal" colSpan={2}>
                    Ride
                  </th>
                  <th className="px-3 py-2 text-right font-normal">Distance</th>
                  <th className="px-3 py-2 text-right font-normal">Moving</th>
                  <th className="px-3 py-2 text-right font-normal">Climbed</th>
                  <th className="px-3 py-2 text-right font-normal">Avg HR</th>
                  <th className="px-3 py-2 text-right font-normal">hrTSS</th>
                  <th className="px-3 py-2 text-right font-normal">Weather</th>
                </tr>
              </thead>
              <tbody>
                {month.rides.map((ride) => (
                  <tr
                    key={ride.id}
                    className="cursor-pointer border-[var(--rule)] border-t tabular-nums hover:bg-[var(--base)]"
                  >
                    <td className="w-10 py-1.5 pl-3">
                      <div className="size-7">
                        <RouteGlyph coordinates={ride.coordinates} title="ride" band={ride.band} />
                      </div>
                    </td>
                    <td className="px-3 py-1.5">
                      <span className="font-medium">
                        {label(ride.startedAt, { weekday: "short", day: "numeric" })}
                      </span>
                      <span className="ml-2 text-[var(--ink-2)] text-xs">
                        {label(ride.startedAt, { hour: "2-digit", minute: "2-digit" })}
                      </span>
                    </td>
                    <td className="px-3 py-1.5 text-right">
                      {formatDistance(ride.distanceMetres)}
                    </td>
                    <td className="px-3 py-1.5 text-right">{formatDuration(ride.movingSeconds)}</td>
                    <td className="px-3 py-1.5 text-right">{formatAscent(ride.ascentMetres)}</td>
                    <td className="px-3 py-1.5 text-right text-[var(--ink-2)]">
                      {ride.metrics?.averageHeartRateBpm ?? "—"}
                    </td>
                    <td className="px-3 py-1.5 text-right text-[var(--ink-2)]">
                      {ride.metrics?.heartRateTss ?? "—"}
                    </td>
                    <td className="px-3 py-1.5">
                      <span className="flex justify-end">
                        <Weather ride={ride} />
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      ))}
    </div>
  );
}

/* ---------------------------------------------------------------- B: Cards */

/**
 * The bet: the index is for browsing, and a ride is recognised by its shape
 * before its date. Each card leads with the glyph and one large distance; the
 * rest is a line of small figures and a weather mark. Three across, so a
 * season fits on a screen.
 */
export function CardsIndex() {
  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-4">
      <h1 className="font-semibold text-2xl tracking-tight">Activities</h1>
      <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {RIDES.map((ride) => (
          <li
            key={ride.id}
            className="flex cursor-pointer gap-3 rounded-xl bg-[var(--panel)] p-3 ring-1 ring-black/5 hover:bg-[var(--base)]"
          >
            <div className="size-16 shrink-0 rounded-lg bg-[var(--base)] p-1.5">
              <RouteGlyph coordinates={ride.coordinates} title="ride" band={ride.band} />
            </div>
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <div className="flex items-baseline justify-between gap-2">
                <span className="text-[var(--ink-2)] text-xs">
                  {label(ride.startedAt, { weekday: "short", day: "numeric", month: "short" })}
                </span>
                <Weather ride={ride} />
              </div>
              <span className="font-semibold text-2xl tabular-nums tracking-tight">
                {formatDistance(ride.distanceMetres)}
              </span>
              <span className="text-[var(--ink-2)] text-xs tabular-nums">
                {formatDuration(ride.movingSeconds)} · {formatAscent(ride.ascentMetres)}
                {ride.metrics ? ` · ${ride.metrics.averageHeartRateBpm} bpm` : ""}
              </span>
              {ride.metrics?.heartRateTss ? (
                <span
                  className="mt-1 flex h-1 overflow-hidden rounded-full bg-black/5"
                  aria-hidden="true"
                >
                  <span
                    className="h-full rounded-full bg-[var(--accent)]"
                    style={{ width: `${Math.min(100, ride.metrics.heartRateTss / 2.5)}%` }}
                  />
                </span>
              ) : null}
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}

/* ---------------------------------------------------------------- C: Weeks */

/** The tallest bar, which the longest ride in the range gets; every other bar is scaled to it. */
const BAR_REM = 6;

/**
 * The bet: what a rider wants from the index is the rhythm — which weeks were
 * big, which were rest, whether the weekend ride happened. One row per week,
 * seven day columns, each ride a bar whose height is its distance on one
 * scale shared by every week, with its figures beside it and the week's
 * totals along the left. Volume's numbers, but with the rides in them.
 */
export function WeeksIndex() {
  const weeks = useMemo(() => groupBy(RIDES, weekOf), []);
  const longest = Math.max(...RIDES.map((ride) => ride.distanceMetres));

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-4">
      <h1 className="font-semibold text-2xl tracking-tight">Activities</h1>
      <div className="grid gap-x-2 px-2" style={{ gridTemplateColumns: WEEK_COLUMNS }}>
        <span />
        {WEEKDAYS.map((day) => (
          <Label key={day}>{day}</Label>
        ))}
      </div>
      {weeks.map((week) => (
        <section
          key={week.key}
          className="grid gap-x-2 rounded-xl bg-[var(--panel)] p-2 ring-1 ring-black/5"
          style={{ gridTemplateColumns: WEEK_COLUMNS }}
        >
          <div className="flex flex-col gap-2 pr-2">
            <span className="font-medium text-sm">
              {label(week.key, { day: "numeric", month: "short" })} –{" "}
              {label(new Date(new Date(week.key).getTime() + 6 * 86_400_000).toISOString(), {
                day: "numeric",
                month: "short",
              })}
            </span>
            <div className="flex flex-col gap-1 text-xs tabular-nums">
              <span className="font-semibold text-base">{formatDistance(week.distance)}</span>
              <span className="text-[var(--ink-2)]">
                {formatDuration(week.moving)} · {formatAscent(week.ascent)}
                {week.tss ? ` · ${week.tss} hrTSS` : ""}
              </span>
            </div>
          </div>
          {WEEKDAYS.map((day, index) => (
            // The floor every bar stands on, so a row with one short ride is
            // read against the same height as a row with a long one.
            <div
              key={day}
              className="flex items-end gap-2 border-[var(--rule)] border-b pb-1"
              style={{ height: `${BAR_REM + 1.5}rem` }}
            >
              {week.rides
                .filter((ride) => dayOf(ride.startedAt) === index)
                .map((ride) => (
                  <button
                    type="button"
                    key={ride.id}
                    className="flex items-end gap-1.5 rounded text-left hover:bg-[var(--base)]"
                  >
                    <span
                      aria-hidden="true"
                      className="w-3 shrink-0 rounded-t"
                      style={{
                        height: `${(ride.distanceMetres / longest) * BAR_REM}rem`,
                        backgroundColor: `var(--grade-${ride.band})`,
                      }}
                    />
                    <span className="flex flex-col gap-0.5 pb-0.5">
                      <span className="size-5">
                        <RouteGlyph coordinates={ride.coordinates} title="ride" band={ride.band} />
                      </span>
                      <span className="font-semibold text-sm tabular-nums">
                        {formatDistance(ride.distanceMetres)}
                      </span>
                      <span className="text-[10px] text-[var(--ink-2)] tabular-nums">
                        {formatAscent(ride.ascentMetres)}
                      </span>
                      <Weather ride={ride} temperature={false} />
                    </span>
                  </button>
                ))}
            </div>
          ))}
        </section>
      ))}
    </div>
  );
}
