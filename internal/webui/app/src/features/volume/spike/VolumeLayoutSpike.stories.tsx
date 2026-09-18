/**
 * Three wide layouts for the volume page after the fitness page's strip and rail.
 * Every rail card is derived from the ride list alone, so none needs a new endpoint.
 *
 * Storybook only, over two synthetic years of riding.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  IconBike,
  IconCalendarStats,
  IconCalendarWeek,
  IconClock,
  IconMountain,
  IconRoute,
  IconTrophy,
} from "@tabler/icons-react";
import type { ReactNode } from "react";
import type { Activity } from "../../../api/types";
import { PanelHeading, PanelMark } from "../../../components/PanelHeading";
import { formatAscent, formatCount, formatDistance, formatDuration } from "../../../lib/format";
import {
  bucketActivities,
  type VolumeBucket,
  volumeTotals,
  weekdayIndex,
} from "../../../lib/volume";

const ZONE = "Europe/Berlin";
const NOW = new Date("2026-09-17T12:00:00Z");
const DAY = 86_400_000;

const RIDES: Activity[] = Array.from({ length: 720 }, (_, index) => index).flatMap((daysAgo) => {
  const date = new Date(NOW.getTime() - daysAgo * DAY);
  const weekday = (date.getUTCDay() + 6) % 7;
  const season = 0.6 + 0.4 * Math.sin(((date.getUTCMonth() - 2) / 12) * 2 * Math.PI);
  const rides = weekday === 5 || weekday === 6 || (weekday === 2 && daysAgo % 3 !== 0);
  if (!rides || (daysAgo * 7) % 11 === 0) {
    return [];
  }
  const long = weekday === 6 ? 2.2 : weekday === 5 ? 1.4 : 0.8;
  const distanceMetres = Math.round(42_000 * long * season * (1 + 0.2 * Math.sin(daysAgo)));
  return [
    {
      id: `ride-${daysAgo}`,
      startedAt: new Date(date.setUTCHours(8, 0, 0, 0)).toISOString(),
      distanceMetres,
      movingSeconds: Math.round(distanceMetres / 7.8),
      elapsedSeconds: Math.round(distanceMetres / 7.2),
      ascentMetres: Math.round((distanceMetres / 1000) * (8 + 6 * Math.abs(Math.sin(daysAgo / 5)))),
      typeId: 0,
      locationId: 0,
      provider: daysAgo % 9 === 0 ? "zwift" : "wahoo",
    } as Activity,
  ];
});

const totals = volumeTotals(RIDES);
const weeks = bucketActivities(RIDES, "week", ZONE, NOW);
const months = bucketActivities(RIDES, "month", ZONE, NOW);

const yearOf = (ride: Activity) => new Date(ride.startedAt).getUTCFullYear();
const dayOfYear = (date: Date) =>
  Math.floor((date.getTime() - Date.UTC(date.getUTCFullYear(), 0, 1)) / DAY);
const today = dayOfYear(NOW);
const thisYear = volumeTotals(RIDES.filter((ride) => yearOf(ride) === 2026));
const lastYearToDate = volumeTotals(
  RIDES.filter((ride) => yearOf(ride) === 2025 && dayOfYear(new Date(ride.startedAt)) <= today),
);

const byDistance = (one: { distanceMetres: number }, other: { distanceMetres: number }) =>
  other.distanceMetres - one.distanceMetres;
const bestWeek = [...weeks].sort(byDistance)[0];
const bestMonth = [...months].sort(byDistance)[0];
const longest = [...RIDES].sort(byDistance)[0];
const steepest = [...RIDES].sort((one, other) => other.ascentMetres - one.ascentMetres)[0];

const weekdays = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"].map((name, index) => {
  const rides = RIDES.filter((ride) => weekdayIndex(ride, ZONE) === index);
  return { name, ...volumeTotals(rides) };
});

const icon = (Glyph: typeof IconRoute) => <Glyph size={18} stroke={1.8} />;
const BOX = "flex min-w-0 flex-col gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)]";
const tint = (tone: string) => `color-mix(in oklab, ${tone} 14%, transparent)`;

function Chip({ tone, children }: { tone: string; children: ReactNode }) {
  return (
    <span
      className="whitespace-nowrap rounded-md px-1.5 py-0.5 font-medium text-xs"
      style={{ background: tint(tone), color: tone }}
    >
      {children}
    </span>
  );
}

const signedPercent = (now: number, before: number) => {
  const percent = before > 0 ? Math.round(((now - before) / before) * 100) : 0;
  return `${percent >= 0 ? "+" : "−"}${Math.abs(percent)}%`;
};

function Strip() {
  const figures = [
    { glyph: IconRoute, label: "Distance", value: formatDistance(totals.distanceMetres) },
    { glyph: IconClock, label: "Moving time", value: formatDuration(totals.movingSeconds) },
    { glyph: IconMountain, label: "Ascent", value: formatAscent(totals.ascentMetres) },
    { glyph: IconBike, label: "Rides", value: totals.count.toLocaleString() },
  ];
  return (
    <div className="grid grid-cols-2 gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)] lg:grid-cols-4 lg:gap-0 lg:divide-x lg:divide-[var(--rule)]">
      {figures.map(({ glyph, label, value }) => (
        <div key={label} className="flex items-center gap-3 lg:px-4 lg:first:pl-0 lg:last:pr-0">
          <PanelMark>{icon(glyph)}</PanelMark>
          <div className="flex flex-col">
            <span className="text-[var(--ink-2)] text-xs">{label}</span>
            <span className="font-semibold text-xl tabular-nums tracking-tight">{value}</span>
          </div>
        </div>
      ))}
    </div>
  );
}

function BucketList({
  buckets,
  heading,
  glyph,
}: {
  buckets: VolumeBucket[];
  heading: string;
  glyph: typeof IconRoute;
}) {
  const widest = Math.max(...buckets.map((bucket) => bucket.distanceMetres), 1);
  return (
    <section className={BOX}>
      <PanelHeading icon={icon(glyph)} title={heading} />
      <ul className="flex flex-col gap-2">
        {buckets.slice(0, 20).map((bucket) => (
          <li
            key={bucket.start.toISOString()}
            className="grid grid-cols-[5.5rem_1fr] items-center gap-3"
          >
            <span className="text-[var(--ink-2)] text-sm">{bucket.label}</span>
            <div className="flex flex-col gap-1">
              <div
                className="h-2 min-w-px rounded-full bg-[var(--accent)]"
                style={{ width: `${(bucket.distanceMetres / widest) * 100}%` }}
              />
              <span className="text-[var(--ink-2)] text-xs">
                {bucket.count === 0
                  ? "No rides"
                  : [
                      formatDistance(bucket.distanceMetres),
                      formatDuration(bucket.movingSeconds),
                      formatAscent(bucket.ascentMetres),
                      formatCount(bucket.count, "ride"),
                    ].join(" · ")}
              </span>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}

function WeekChart() {
  const shown = weeks.slice(0, 52).reverse();
  const widest = Math.max(...shown.map((bucket) => bucket.distanceMetres), 1);
  return (
    <section className={BOX}>
      <PanelHeading
        icon={icon(IconCalendarStats)}
        title="The last 52 weeks"
        subtitle="distance a week"
      />
      <div className="flex h-56 items-end gap-[3px]">
        {shown.map((bucket) => (
          <div
            key={bucket.start.toISOString()}
            title={`${bucket.label}: ${formatDistance(bucket.distanceMetres)}`}
            className="min-h-px flex-1 rounded-t-sm bg-[var(--accent)]"
            style={{ height: `${(bucket.distanceMetres / widest) * 100}%` }}
          />
        ))}
      </div>
      <div className="flex justify-between text-[var(--ink-2)] text-xs">
        <span>{shown[0]?.label}</span>
        <span>{shown[shown.length - 1]?.label}</span>
      </div>
    </section>
  );
}

function YearToDate() {
  const rows = [
    [
      "Distance",
      formatDistance(thisYear.distanceMetres),
      thisYear.distanceMetres,
      lastYearToDate.distanceMetres,
    ],
    [
      "Moving time",
      formatDuration(thisYear.movingSeconds),
      thisYear.movingSeconds,
      lastYearToDate.movingSeconds,
    ],
    [
      "Ascent",
      formatAscent(thisYear.ascentMetres),
      thisYear.ascentMetres,
      lastYearToDate.ascentMetres,
    ],
    ["Rides", String(thisYear.count), thisYear.count, lastYearToDate.count],
  ] as const;
  return (
    <section className={BOX}>
      <PanelHeading
        icon={icon(IconCalendarWeek)}
        title="This year"
        subtitle="against 2025 to date"
      />
      <dl className="flex flex-col divide-y divide-[var(--rule)]">
        {rows.map(([label, value, now, before]) => (
          <div
            key={label}
            className="flex items-center justify-between gap-2 py-2 first:pt-0 last:pb-0"
          >
            <dt className="text-[var(--ink-2)] text-sm">{label}</dt>
            <dd className="flex items-center gap-2 font-medium text-sm tabular-nums">
              {value}
              <Chip tone={now >= before ? "var(--good)" : "var(--hold)"}>
                {signedPercent(now, before)}
              </Chip>
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

function Records() {
  const rows = [
    ["Biggest week", bestWeek ? formatDistance(bestWeek.distanceMetres) : "—", bestWeek?.label],
    ["Biggest month", bestMonth ? formatDistance(bestMonth.distanceMetres) : "—", bestMonth?.label],
    [
      "Longest ride",
      longest ? formatDistance(longest.distanceMetres) : "—",
      longest?.startedAt.slice(0, 10),
    ],
    [
      "Most climbing",
      steepest ? formatAscent(steepest.ascentMetres) : "—",
      steepest?.startedAt.slice(0, 10),
    ],
  ] as const;
  return (
    <section className={BOX}>
      <PanelHeading icon={icon(IconTrophy)} title="Records" />
      <dl className="flex flex-col divide-y divide-[var(--rule)]">
        {rows.map(([label, value, when]) => (
          <div
            key={label}
            className="flex items-baseline justify-between gap-2 py-2 first:pt-0 last:pb-0"
          >
            <dt className="flex flex-col">
              <span className="text-sm">{label}</span>
              <span className="text-[var(--ink-2)] text-xs">{when}</span>
            </dt>
            <dd className="font-medium text-sm tabular-nums">{value}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

function Weekdays() {
  const widest = Math.max(...weekdays.map((day) => day.distanceMetres), 1);
  return (
    <section className={BOX}>
      <PanelHeading icon={icon(IconCalendarStats)} title="By weekday" subtitle="all rides" />
      <ul className="flex flex-col gap-1.5">
        {weekdays.map((day) => (
          <li
            key={day.name}
            className="grid grid-cols-[2.5rem_1fr_4.5rem] items-center gap-2 text-sm"
          >
            <span className="text-[var(--ink-2)]">{day.name}</span>
            <div className="h-2 rounded-full bg-[var(--rule)]">
              <div
                className="h-2 rounded-full bg-[var(--accent)]"
                style={{ width: `${(day.distanceMetres / widest) * 100}%` }}
              />
            </div>
            <span className="text-right text-[var(--ink-2)] text-xs tabular-nums">
              {formatCount(day.count, "ride")}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function Page({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-dvh bg-[var(--base)] px-6 py-8 text-[var(--ink)]">
      <div className="mx-auto flex w-full max-w-[1400px] flex-col gap-5">
        <h1 className="font-semibold text-2xl tracking-tight">Volume</h1>
        {children}
      </div>
    </div>
  );
}

const Rail = ({ main, rail }: { main: ReactNode; rail: ReactNode }) => (
  <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_22rem]">
    <div className="flex min-w-0 flex-col gap-5">{main}</div>
    <div className="flex min-w-0 flex-col gap-5">{rail}</div>
  </div>
);

const meta = {
  title: "Spikes/Volume Layout",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A · Strip and rail: today's weekly list in the main column; this year against last, and records, in the rail. */
export const StripAndRail: Story = {
  render: () => (
    <Page>
      <Strip />
      <Rail
        main={<BucketList buckets={weeks} heading="By week" glyph={IconCalendarStats} />}
        rail={
          <>
            <YearToDate />
            <Records />
          </>
        }
      />
    </Page>
  ),
};

/** B · Chart first: a year of weeks as columns over the monthly list; weekday mix and records in the rail. */
export const ChartAndRail: Story = {
  render: () => (
    <Page>
      <Strip />
      <Rail
        main={
          <>
            <WeekChart />
            <BucketList buckets={months} heading="By month" glyph={IconCalendarWeek} />
          </>
        }
        rail={
          <>
            <YearToDate />
            <Weekdays />
            <Records />
          </>
        }
      />
    </Page>
  ),
};

/** C · Both periods at once: weeks and months side by side, no switch; this year in the rail. */
export const PeriodsSideBySide: Story = {
  render: () => (
    <Page>
      <Strip />
      <Rail
        main={
          <div className="grid items-start gap-5 xl:grid-cols-2">
            <BucketList buckets={weeks} heading="By week" glyph={IconCalendarStats} />
            <BucketList buckets={months} heading="By month" glyph={IconCalendarWeek} />
          </div>
        }
        rail={
          <>
            <YearToDate />
            <Records />
          </>
        }
      />
    </Page>
  ),
};
