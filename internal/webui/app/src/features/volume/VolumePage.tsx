/**
 * Volume: what the rider has actually ridden, their whole recorded history.
 *
 * Every other page is about routes the service holds for them; this is the one
 * about rides they have already done, read from the activity summaries their
 * target recorded. Weeks and months are the two periods training is counted in,
 * so they are the only two offered.
 */

import {
  IconBike,
  IconCalendarMonth,
  IconCalendarStats,
  IconCalendarWeek,
  IconClock,
  IconMountain,
  IconRoute,
  IconTrophy,
} from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { type ReactNode, useMemo, useState } from "react";
import { Link } from "react-router";
import { activitiesQuery, webUIConfigQuery } from "../../api/queries";
import type { Activity } from "../../api/types";
import {
  barOpacity,
  ChartLegend,
  TimeFrame,
  topRoundedBar,
} from "../../components/chart/TimeFrame";
import { FigureStrip, StripFigure } from "../../components/FigureStrip";
import { PageShell } from "../../components/Layout";
import { PanelHeading } from "../../components/PanelHeading";
import { Segmented } from "../../components/Segmented";
import { Skeleton } from "../../components/ui/skeleton";
import {
  formatAscent,
  formatCount,
  formatDistance,
  formatDuration,
  formatTimestamp,
} from "../../lib/format";
import {
  bucketActivities,
  startOfYear,
  type VolumeBucket,
  type VolumeRecords,
  type VolumeTotals,
  volumeRecords,
  volumeTotals,
  weekdayTotals,
  weekRangeLabel,
  type YearToDate,
  yearToDate,
} from "../../lib/volume";

type Ground = "both" | "outdoor" | "indoor";
const GROUNDS: ReadonlyArray<{ key: Ground; label: string }> = [
  { key: "both", label: "Both" },
  { key: "outdoor", label: "Outdoor" },
  { key: "indoor", label: "Indoor" },
];

/** The two grounds every chart and figure on the page splits by, outdoor first. */
const GROUND_COLOURS = { outdoor: "var(--ground-outdoor)", indoor: "var(--ground-indoor)" };
type Split<T> = Record<"outdoor" | "indoor", T>;
const splitRides = (rides: Activity[]): Split<Activity[]> => ({
  outdoor: rides.filter((ride) => !ride.indoor),
  indoor: rides.filter((ride) => ride.indoor),
});
/** Each ground's buckets keyed by their start instant, to read beside the combined ones. */
const bucketsByStart = (buckets: VolumeBucket[]) =>
  new Map(buckets.map((bucket) => [bucket.start.getTime(), bucket]));
const distanceAt = (buckets: Map<number, VolumeBucket>, start: Date) =>
  buckets.get(start.getTime())?.distanceMetres ?? 0;

/** How far back the page counts; This year keeps its own calendar window. */
const RANGES: ReadonlyArray<{ key: string; label: string; days?: number; name: string }> = [
  { key: "90", label: "3 months", days: 90, name: "last 3 months" },
  { key: "180", label: "6 months", days: 180, name: "last 6 months" },
  { key: "ytd", label: "YTD", name: "year to date" },
  { key: "365", label: "1 year", days: 365, name: "last year" },
  { key: "all", label: "All", name: "all rides" },
];
const WEEKDAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
const BOX = "flex min-w-0 flex-col gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)]";
const glyph = (Icon: typeof IconRoute) => <Icon size={18} stroke={1.8} />;

// Only while the config is unavailable: once it answers, its zone is the one
// used, whether or not it happens to match the browser's own.
const browserZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;

export function VolumePage() {
  const config = useQuery(webUIConfigQuery());
  const serviceZone = config.data?.timezone || null;
  const zone = serviceZone ?? browserZone();
  const [ground, setGround] = useState<Ground>("both");
  const { data, isPending, isError } = useQuery(activitiesQuery());
  const recorded = data ?? [];
  const [range, setRange] = useState("365");
  const selected = RANGES.find(({ key }) => key === range) ?? RANGES[3];
  const onGround = useMemo(
    () => (data ?? []).filter((ride) => ground === "both" || ride.indoor === (ground === "indoor")),
    [data, ground],
  );
  const from = useMemo(() => {
    if (range === "ytd") {
      return startOfYear(zone);
    }
    const days = selected?.days;
    return days === undefined ? undefined : new Date(Date.now() - days * 86_400_000);
  }, [range, selected, zone]);
  const activities = useMemo(
    () =>
      from ? onGround.filter((ride) => Date.parse(ride.startedAt) >= from.getTime()) : onGround,
    [onGround, from],
  );
  const weeks = useMemo(
    () => bucketActivities(activities, "week", zone, undefined, from),
    [activities, zone, from],
  );
  const months = useMemo(
    () => bucketActivities(activities, "month", zone, undefined, from),
    [activities, zone, from],
  );
  const totals = useMemo(() => volumeTotals(activities), [activities]);
  const split = useMemo(() => {
    const rides = splitRides(activities);
    const per = <T,>(read: (rides: Activity[]) => T): Split<T> => ({
      outdoor: read(rides.outdoor),
      indoor: read(rides.indoor),
    });
    return {
      totals: per(volumeTotals),
      weeks: per((some) => bucketsByStart(bucketActivities(some, "week", zone, undefined, from))),
      months: per((some) => bucketsByStart(bucketActivities(some, "month", zone, undefined, from))),
      weekdays: per((some) => weekdayTotals(some, zone)),
    };
  }, [activities, zone, from]);
  const both = split.totals.outdoor.count > 0 && split.totals.indoor.count > 0;
  const note = (read: (totals: VolumeTotals) => string) =>
    both ? (
      <GroundValues outdoor={read(split.totals.outdoor)} indoor={read(split.totals.indoor)} />
    ) : undefined;
  const year = useMemo(() => yearToDate(onGround, zone), [onGround, zone]);
  const records = useMemo(() => volumeRecords(activities, zone), [activities, zone]);
  const weekdays = useMemo(() => weekdayTotals(activities, zone), [activities, zone]);

  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-[1400px] flex-col gap-5">
        <header className="flex flex-wrap items-center justify-between gap-3">
          <h1 className="font-semibold text-2xl tracking-tight">Volume</h1>
          {recorded.length > 0 ? (
            <div className="flex flex-wrap items-center gap-2">
              <Segmented label="Range" size="sm" items={RANGES} value={range} onChange={setRange} />
              <Segmented
                label="Rides"
                size="sm"
                items={GROUNDS}
                value={ground}
                onChange={setGround}
              />
            </div>
          ) : null}
        </header>
        {!config.isPending && serviceZone === null && (
          <p className="text-[var(--ink-2)] text-xs">Periods follow this browser's time zone</p>
        )}
        {isPending ? (
          <Skeleton className="h-64 w-full" role="status" aria-label="Loading activities" />
        ) : isError ? (
          <p className="text-sm text-[var(--alert)]">
            The service did not say what has been ridden.
          </p>
        ) : recorded.length === 0 ? (
          <p className="text-[var(--ink-2)] text-sm">
            No rides have been recorded yet. Once a Wahoo account is connected on{" "}
            <Link className="underline" to="/account/accounts">
              your account
            </Link>
            , the rides it records appear here.
          </p>
        ) : totals.count === 0 ? (
          <p className="text-[var(--ink-2)] text-sm">
            No {ground === "both" ? "" : `${ground} `}rides in the {selected?.name}.
          </p>
        ) : (
          <>
            <FigureStrip>
              <StripFigure
                icon={glyph(IconRoute)}
                label="Distance"
                value={formatDistance(totals.distanceMetres)}
                note={note((some) => formatDistance(some.distanceMetres))}
              />
              <StripFigure
                icon={glyph(IconClock)}
                label="Moving time"
                value={formatDuration(totals.movingSeconds)}
                note={note((some) => formatDuration(some.movingSeconds))}
              />
              <StripFigure
                icon={glyph(IconMountain)}
                label="Ascent"
                value={formatAscent(totals.ascentMetres)}
                note={note((some) => formatAscent(some.ascentMetres))}
              />
              <StripFigure
                icon={glyph(IconBike)}
                label="Rides"
                value={totals.count.toLocaleString()}
                note={note((some) => some.count.toLocaleString())}
              />
            </FigureStrip>
            <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_22rem]">
              <div className="flex min-w-0 flex-col gap-5">
                <WeekChart weeks={weeks} grounds={split.weeks} zone={zone} legend={both} />
                <MonthList months={months} grounds={split.months} />
              </div>
              <div className="flex min-w-0 flex-col gap-5">
                <ThisYear year={year} />
                <Weekdays
                  weekdays={weekdays}
                  grounds={split.weekdays}
                  range={selected?.name ?? ""}
                />
                {records ? <Records records={records} range={selected?.name ?? ""} /> : null}
              </div>
            </div>
          </>
        )}
      </div>
    </PageShell>
  );
}

/** Two figures, each marked with its ground's colour. */
function GroundValues({ outdoor, indoor }: Split<string>) {
  return (
    <span className="flex items-center gap-2">
      <GroundValue colour={GROUND_COLOURS.outdoor} value={outdoor} />
      <GroundValue colour={GROUND_COLOURS.indoor} value={indoor} />
    </span>
  );
}

function GroundValue({ colour, value }: { colour: string; value: string }) {
  return (
    <span className="flex items-center gap-1">
      <span aria-hidden="true" className="size-2 rounded-full" style={{ background: colour }} />
      {value}
    </span>
  );
}

/** One horizontal bar, outdoor then indoor, as long as their sum against `widest`.
 * Only the bar's own ends are rounded; the grounds meet square across a small gap. */
function SplitBar({
  outdoor,
  indoor,
  widest,
  height = "h-2",
}: Split<number> & { widest: number; height?: string }) {
  const total = outdoor + indoor;

  return (
    <div
      aria-hidden="true"
      className={`flex ${height} min-w-px shrink-0 gap-0.5 overflow-hidden rounded-full`}
      style={{ width: `${(total / widest) * 100}%` }}
    >
      {(["outdoor", "indoor"] as const).map((ground) => {
        const metres = ground === "outdoor" ? outdoor : indoor;
        return metres > 0 ? (
          <div
            key={ground}
            className="min-w-px"
            style={{ flexGrow: metres, flexBasis: 0, background: GROUND_COLOURS[ground] }}
          />
        ) : null;
      })}
    </div>
  );
}

function WeekChart({
  weeks,
  grounds,
  zone,
  legend,
}: {
  weeks: VolumeBucket[];
  grounds: Split<Map<number, VolumeBucket>>;
  zone: string;
  legend: boolean;
}) {
  const shown = [...weeks].reverse();
  const day = new Intl.DateTimeFormat("en-CA", { timeZone: zone });
  const newest = shown.at(-1)?.start.getTime() ?? 0;
  // One day past the newest week, so its bar has an edge to end at; half a day spare clears DST.
  const dates = [...shown.map((week) => week.start), new Date(newest + 7.5 * 86_400_000)].map(
    (start) => day.format(start),
  );
  const high = Math.max(1, ...shown.map((week) => week.distanceMetres / 1000));

  return (
    <section className={BOX} aria-label="Weekly distance">
      <PanelHeading
        icon={glyph(IconCalendarStats)}
        title={`The last ${formatCount(shown.length, "week")}`}
        subtitle="distance a week"
        aside={
          legend ? (
            <ChartLegend
              items={GROUNDS.slice(1).map(({ key, label }) => ({
                label,
                colour: GROUND_COLOURS[key as keyof typeof GROUND_COLOURS],
                swatch: true,
              }))}
            />
          ) : undefined
        }
      />
      <TimeFrame
        label={`Distance in each of the last ${formatCount(shown.length, "week")}`}
        dates={dates}
        snap={shown.map((_, index) => index)}
        bars
        heading={(index) => (shown[index] ? weekRangeLabel(shown[index].start, zone) : null)}
        readout={(index) => {
          const week = shown[index];
          if (!week) {
            return null;
          }
          if (week.count === 0) {
            return <div className="opacity-80">No rides</div>;
          }
          return (
            <>
              {(["outdoor", "indoor"] as const).map((ground) => {
                const metres = distanceAt(grounds[ground], week.start);
                return metres > 0 ? (
                  <div
                    key={ground}
                    className="flex items-center justify-between gap-6 tabular-nums"
                  >
                    <span className="flex items-center gap-1.5 opacity-80">
                      <span
                        aria-hidden="true"
                        className="size-2 rounded-[2px]"
                        style={{ background: GROUND_COLOURS[ground] }}
                      />
                      {ground === "outdoor" ? "Outdoor" : "Indoor"}
                    </span>
                    {formatDistance(metres)}
                  </div>
                ) : null;
              })}
              <div className="mt-1 flex justify-between gap-6 border-[color-mix(in_oklab,currentColor_15%,transparent)] border-t pt-1 font-semibold tabular-nums">
                <span>{formatCount(week.count, "ride")}</span>
                {formatDistance(week.distanceMetres)}
              </div>
            </>
          );
        }}
        panels={[
          {
            height: 180,
            domain: [0, high],
            format: (value) => `${value} km`,
            draw: (x, y, active) =>
              shown.map((week, index) => {
                const left = x(index) + 1;
                const width = Math.max(x(index + 1) - left - 1, 2);
                const outdoor = distanceAt(grounds.outdoor, week.start) / 1000;
                const indoor = distanceAt(grounds.indoor, week.start) / 1000;
                // Only the upper ground is rounded; the lower keeps a square top under it.
                const outdoorHeight = Math.max(y(0) - y(outdoor), 0);
                // A surface-coloured gap keeps the two grounds apart.
                const indoorHeight = Math.max(y(outdoor) - y(outdoor + indoor) - 1, 0);
                return (
                  <g key={week.start.toISOString()} opacity={barOpacity(active, index)}>
                    {indoor > 0 ? (
                      <>
                        <rect
                          x={left}
                          y={y(outdoor)}
                          width={width}
                          height={outdoorHeight}
                          fill={GROUND_COLOURS.outdoor}
                        />
                        <path
                          d={topRoundedBar(left, y(outdoor + indoor), width, indoorHeight)}
                          fill={GROUND_COLOURS.indoor}
                        />
                      </>
                    ) : (
                      <path
                        d={topRoundedBar(left, y(outdoor), width, outdoorHeight)}
                        fill={GROUND_COLOURS.outdoor}
                      />
                    )}
                  </g>
                );
              }),
          },
        ]}
      />
    </section>
  );
}

function MonthList({
  months,
  grounds,
}: {
  months: VolumeBucket[];
  grounds: Split<Map<number, VolumeBucket>>;
}) {
  // The longest bar stops short of the row, leaving its distance room to sit at its end.
  const widest = Math.max(...months.map((month) => month.distanceMetres), 1) * 1.2;

  return (
    <section className={BOX} aria-label="By month">
      <PanelHeading icon={glyph(IconCalendarMonth)} title="By month" />
      <ul className="flex flex-col gap-2.5">
        {months.map((month) => (
          <li
            key={month.start.toISOString()}
            className="grid grid-cols-[5.5rem_1fr_4.5rem] items-center gap-3 text-sm"
          >
            <span className="text-[var(--ink-2)]">{month.label}</span>
            <div className="flex min-w-0 items-center gap-2">
              <SplitBar
                outdoor={distanceAt(grounds.outdoor, month.start)}
                indoor={distanceAt(grounds.indoor, month.start)}
                widest={widest}
                height="h-3"
              />
              <span className="whitespace-nowrap font-medium tabular-nums">
                {formatDistance(month.distanceMetres)}
              </span>
            </div>
            <span className="text-right text-[var(--ink-2)] text-xs tabular-nums">
              {formatCount(month.count, "ride")}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

/** A hairline ledger: a muted name on the left, whatever reads it on the right. */
function Ledger({ rows }: { rows: ReadonlyArray<{ name: ReactNode; value: ReactNode }> }) {
  return (
    <dl className="flex flex-col divide-y divide-[var(--rule)]">
      {rows.map(({ name, value }, index) => (
        <div
          // biome-ignore lint/suspicious/noArrayIndexKey: the rows are fixed and never reorder
          key={index}
          className="flex items-center justify-between gap-2 py-2 first:pt-0 last:pb-0"
        >
          <dt className="min-w-0 text-sm">{name}</dt>
          <dd className="flex shrink-0 items-center gap-2 font-medium text-sm tabular-nums">
            {value}
          </dd>
        </div>
      ))}
    </dl>
  );
}

function ThisYear({ year }: { year: YearToDate }) {
  const { current, previous } = year;
  const change = (now: number, before: number) => {
    if (before === 0) {
      return null;
    }
    const percent = Math.round(((now - before) / before) * 100);
    const tone = percent >= 0 ? "var(--good)" : "var(--hold)";
    return (
      <span
        className="rounded-md px-1.5 py-0.5 font-medium text-xs"
        style={{ background: `color-mix(in oklab, ${tone} 14%, transparent)`, color: tone }}
      >
        {percent >= 0 ? "+" : "−"}
        {Math.abs(percent)}%
      </span>
    );
  };
  const rows = [
    [
      "Distance",
      formatDistance(current.distanceMetres),
      current.distanceMetres,
      previous.distanceMetres,
    ],
    [
      "Moving time",
      formatDuration(current.movingSeconds),
      current.movingSeconds,
      previous.movingSeconds,
    ],
    ["Ascent", formatAscent(current.ascentMetres), current.ascentMetres, previous.ascentMetres],
    ["Rides", current.count.toLocaleString(), current.count, previous.count],
  ] as const;

  return (
    <section className={BOX} aria-label="This year">
      <PanelHeading
        icon={glyph(IconCalendarWeek)}
        title="This year"
        subtitle={`against ${year.year - 1} to date`}
      />
      <Ledger
        rows={rows.map(([name, value, now, before]) => ({
          name: <span className="text-[var(--ink-2)]">{name}</span>,
          value: (
            <>
              {value}
              {change(now, before)}
            </>
          ),
        }))}
      />
    </section>
  );
}

function Weekdays({
  weekdays,
  grounds,
  range,
}: {
  weekdays: VolumeTotals[];
  grounds: Split<VolumeTotals[]>;
  range: string;
}) {
  const widest = Math.max(...weekdays.map((day) => day.distanceMetres), 1);

  return (
    <section className={BOX} aria-label="By weekday">
      <PanelHeading icon={glyph(IconCalendarStats)} title="By weekday" subtitle={range} />
      <ul className="flex flex-col gap-1.5">
        {weekdays.map((day, index) => (
          <li
            key={WEEKDAYS[index]}
            className="grid grid-cols-[2.5rem_1fr_4.5rem] items-center gap-2 text-sm"
          >
            <span className="text-[var(--ink-2)]">{WEEKDAYS[index]}</span>
            <SplitBar
              outdoor={grounds.outdoor[index]?.distanceMetres ?? 0}
              indoor={grounds.indoor[index]?.distanceMetres ?? 0}
              widest={widest}
            />
            <span className="text-right text-[var(--ink-2)] text-xs tabular-nums">
              {formatCount(day.count, "ride")}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function Records({ records, range }: { records: VolumeRecords; range: string }) {
  const ride = (name: string, when: string, id: string) => (
    <Link className="flex flex-col hover:underline" to={`/activities/${id}`}>
      <span>{name}</span>
      <span className="text-[var(--ink-2)] text-xs">{formatTimestamp(when)}</span>
    </Link>
  );
  const period = (name: string, label: string) => (
    <span className="flex flex-col">
      <span>{name}</span>
      <span className="text-[var(--ink-2)] text-xs">{label}</span>
    </span>
  );

  return (
    <section className={BOX} aria-label="Records">
      <PanelHeading icon={glyph(IconTrophy)} title="Records" subtitle={range} />
      <Ledger
        rows={[
          {
            name: period("Biggest week", records.biggestWeek.label),
            value: formatDistance(records.biggestWeek.distanceMetres),
          },
          {
            name: period("Biggest month", records.biggestMonth.label),
            value: formatDistance(records.biggestMonth.distanceMetres),
          },
          {
            name: ride("Longest ride", records.longestRide.startedAt, records.longestRide.id),
            value: formatDistance(records.longestRide.distanceMetres),
          },
          {
            name: ride("Most climbing", records.mostClimbing.startedAt, records.mostClimbing.id),
            value: formatAscent(records.mostClimbing.ascentMetres),
          },
        ]}
      />
    </section>
  );
}
