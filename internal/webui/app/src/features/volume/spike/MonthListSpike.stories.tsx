/**
 * Six takes on the volume page's By month card.
 *
 * Storybook only, over nine synthetic months that move from indoor to outdoor riding.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { IconCalendarMonth } from "@tabler/icons-react";
import type { ReactNode } from "react";
import { PanelHeading } from "../../../components/PanelHeading";

interface Month {
  short: string;
  label: string;
  outdoor: number;
  indoor: number;
  hours: string;
  ascent: string;
  rides: number;
  /** The same month a year earlier, in km. */
  lastYear: number;
}

const MONTHS: Month[] = [
  {
    short: "Sep",
    label: "Sep 2026",
    outdoor: 453,
    indoor: 0,
    hours: "19 h 25 min",
    ascent: "2,068 m",
    rides: 6,
    lastYear: 520,
  },
  {
    short: "Aug",
    label: "Aug 2026",
    outdoor: 581,
    indoor: 0,
    hours: "24 h",
    ascent: "2,196 m",
    rides: 11,
    lastYear: 610,
  },
  {
    short: "Jul",
    label: "Jul 2026",
    outdoor: 546,
    indoor: 0,
    hours: "22 h 37 min",
    ascent: "2,092 m",
    rides: 12,
    lastYear: 470,
  },
  {
    short: "Jun",
    label: "Jun 2026",
    outdoor: 344,
    indoor: 0,
    hours: "14 h 35 min",
    ascent: "1,385 m",
    rides: 8,
    lastYear: 505,
  },
  {
    short: "May",
    label: "May 2026",
    outdoor: 550,
    indoor: 0,
    hours: "24 h 25 min",
    ascent: "2,869 m",
    rides: 11,
    lastYear: 430,
  },
  {
    short: "Apr",
    label: "Apr 2026",
    outdoor: 673,
    indoor: 0,
    hours: "29 h 22 min",
    ascent: "3,757 m",
    rides: 14,
    lastYear: 390,
  },
  {
    short: "Mar",
    label: "Mar 2026",
    outdoor: 437,
    indoor: 78,
    hours: "22 h 34 min",
    ascent: "2,867 m",
    rides: 13,
    lastYear: 310,
  },
  {
    short: "Feb",
    label: "Feb 2026",
    outdoor: 45,
    indoor: 130,
    hours: "7 h 20 min",
    ascent: "2,001 m",
    rides: 6,
    lastYear: 260,
  },
  {
    short: "Jan",
    label: "Jan 2026",
    outdoor: 0,
    indoor: 324,
    hours: "11 h 45 min",
    ascent: "3,246 m",
    rides: 13,
    lastYear: 240,
  },
];

const OUT = "var(--ground-outdoor)";
const IN = "var(--ground-indoor)";
const total = (month: Month) => month.outdoor + month.indoor;
const widest = Math.max(...MONTHS.map(total), ...MONTHS.map((month) => month.lastYear));
const BOX =
  "flex w-full max-w-[56rem] flex-col gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)]";

function Card({
  children,
  subtitle,
  aside,
}: {
  children: ReactNode;
  subtitle?: string;
  aside?: ReactNode;
}) {
  return (
    <section className={BOX}>
      <PanelHeading
        icon={<IconCalendarMonth size={18} stroke={1.8} />}
        title="By month"
        subtitle={subtitle}
        aside={aside}
      />
      {children}
    </section>
  );
}

/** A two-ground bar, rounded at its own ends only, `height` tall, as long as its share of `of`. */
function Split({
  month,
  of = widest,
  height = "h-2",
}: {
  month: Month;
  of?: number;
  height?: string;
}) {
  return (
    <div
      className={`flex ${height} min-w-px gap-0.5 overflow-hidden rounded-full`}
      style={{ width: `${(total(month) / of) * 100}%` }}
    >
      {month.outdoor > 0 ? <div style={{ flexGrow: month.outdoor, background: OUT }} /> : null}
      {month.indoor > 0 ? <div style={{ flexGrow: month.indoor, background: IN }} /> : null}
    </div>
  );
}

function Dot({ colour }: { colour: string }) {
  return (
    <span
      aria-hidden="true"
      className="size-2 shrink-0 rounded-[2px]"
      style={{ background: colour }}
    />
  );
}

/** A · Today's: label, bar, the whole line of figures under it. */
function Current() {
  return (
    <Card>
      <ul className="flex flex-col gap-2">
        {MONTHS.map((month) => (
          <li key={month.label} className="grid grid-cols-[5.5rem_1fr] items-center gap-3">
            <span className="text-[var(--ink-2)] text-sm">{month.label}</span>
            <div className="flex flex-col gap-1">
              <Split month={month} />
              <span className="text-[var(--ink-2)] text-xs">
                {total(month)} km · {month.hours} · {month.ascent} · {month.rides} rides
              </span>
            </div>
          </li>
        ))}
      </ul>
    </Card>
  );
}

/** B · Ledger: a hairline table, one row a month, the bar a column among the figures. */
function Ledger() {
  const head = "pb-2 font-normal text-[var(--ink-2)] text-xs";
  return (
    <Card>
      <table className="w-full text-sm tabular-nums">
        <thead>
          <tr className="border-[var(--rule)] border-b text-left">
            <th className={head}>Month</th>
            <th className={`${head} w-[40%]`} />
            <th className={`${head} text-right`}>Distance</th>
            <th className={`${head} text-right`}>Time</th>
            <th className={`${head} text-right`}>Ascent</th>
            <th className={`${head} text-right`}>Rides</th>
          </tr>
        </thead>
        <tbody>
          {MONTHS.map((month) => (
            <tr key={month.label} className="border-[var(--rule)] border-b last:border-0">
              <td className="py-2.5">{month.label}</td>
              <td className="pr-4">
                <Split month={month} height="h-1.5" />
              </td>
              <td className="text-right font-medium">{total(month)} km</td>
              <td className="text-right text-[var(--ink-2)]">{month.hours}</td>
              <td className="text-right text-[var(--ink-2)]">{month.ascent}</td>
              <td className="text-right text-[var(--ink-2)]">{month.rides}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </Card>
  );
}

/** C · Columns: the months as a stacked column chart, oldest left, the distance over each. */
function Columns() {
  const shown = [...MONTHS].reverse();
  return (
    <Card>
      <div className="flex h-56 items-end gap-3">
        {shown.map((month) => (
          <div
            key={month.label}
            className="flex h-full flex-1 flex-col items-center justify-end gap-1"
          >
            <span className="text-[var(--ink-2)] text-xs tabular-nums">{total(month)}</span>
            <div
              className="flex w-full flex-col-reverse gap-0.5 overflow-hidden rounded-t-md"
              style={{ height: `${(total(month) / widest) * 85}%` }}
            >
              {month.outdoor > 0 ? (
                <div style={{ flexGrow: month.outdoor, background: OUT }} />
              ) : null}
              {month.indoor > 0 ? <div style={{ flexGrow: month.indoor, background: IN }} /> : null}
            </div>
            <span className="text-[var(--ink-2)] text-xs">{month.short}</span>
          </div>
        ))}
      </div>
    </Card>
  );
}

/** D · Labelled bars: one line a month, the distance at the bar's end, rides at the row's. */
function Labelled() {
  return (
    <Card>
      <ul className="flex flex-col gap-2.5">
        {MONTHS.map((month) => (
          <li
            key={month.label}
            className="grid grid-cols-[4.5rem_1fr_4.5rem] items-center gap-3 text-sm"
          >
            <span className="text-[var(--ink-2)]">{month.label}</span>
            <div className="flex items-center gap-2">
              <Split month={month} of={widest * 1.15} height="h-3" />
              <span className="whitespace-nowrap font-medium tabular-nums">{total(month)} km</span>
            </div>
            <span className="text-right text-[var(--ink-2)] text-xs tabular-nums">
              {month.rides} rides
            </span>
          </li>
        ))}
      </ul>
    </Card>
  );
}

/** E · Tiles: a tile a month, the distance large, its split and rides beneath. */
function Tiles() {
  return (
    <Card>
      <div className="grid grid-cols-3 gap-2 sm:grid-cols-5">
        {MONTHS.map((month) => (
          <div
            key={month.label}
            className="flex flex-col gap-1.5 rounded-xl bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] p-3"
          >
            <span className="text-[var(--ink-2)] text-xs">{month.label}</span>
            <span className="font-semibold text-lg tabular-nums tracking-tight">
              {total(month)} km
            </span>
            <Split month={month} of={total(month)} height="h-1.5" />
            <span className="flex items-center justify-between text-[var(--ink-2)] text-xs tabular-nums">
              {month.rides} rides
              <span>{month.hours.split(" min")[0]}</span>
            </span>
          </div>
        ))}
      </div>
    </Card>
  );
}

/** F · Against last year: each month's bar over a hairline marking the same month a year before. */
function AgainstLastYear() {
  return (
    <Card
      subtitle="against 2025"
      aside={
        <span className="flex items-center gap-3 text-[var(--ink-2)] text-xs">
          <span className="flex items-center gap-1">
            <Dot colour={OUT} />
            Outdoor
          </span>
          <span className="flex items-center gap-1">
            <Dot colour={IN} />
            Indoor
          </span>
          <span className="flex items-center gap-1">
            <span className="h-3 w-0.5 rounded-full bg-[var(--ink)]" />
            2025
          </span>
        </span>
      }
    >
      <ul className="flex flex-col gap-2.5">
        {MONTHS.map((month) => {
          const change = Math.round(((total(month) - month.lastYear) / month.lastYear) * 100);
          const tone = change >= 0 ? "var(--good)" : "var(--hold)";
          return (
            <li
              key={month.label}
              className="grid grid-cols-[4.5rem_1fr_4rem_3.5rem] items-center gap-3 text-sm"
            >
              <span className="text-[var(--ink-2)]">{month.label}</span>
              <div className="relative h-3">
                <div className="absolute inset-y-0 left-0 flex w-full items-center">
                  <Split month={month} height="h-3" />
                </div>
                <span
                  className="absolute -inset-y-0.5 w-0.5 rounded-full bg-[var(--ink)]"
                  style={{ left: `${(month.lastYear / widest) * 100}%` }}
                />
              </div>
              <span className="text-right font-medium tabular-nums">{total(month)} km</span>
              <span
                className="justify-self-end rounded-md px-1.5 py-0.5 font-medium text-xs tabular-nums"
                style={{ background: `color-mix(in oklab, ${tone} 14%, transparent)`, color: tone }}
              >
                {change >= 0 ? "+" : "−"}
                {Math.abs(change)}%
              </span>
            </li>
          );
        })}
      </ul>
    </Card>
  );
}

const meta = {
  title: "Spikes/Volume Month List",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const page = (Variant: () => ReactNode): Story => ({
  render: () => (
    <div className="min-h-dvh bg-[var(--base)] p-8 text-[var(--ink)]">
      <Variant />
    </div>
  ),
});

/** A · What the page draws today. */
export const Current_ = page(Current);
/** B · A hairline table with the bar as one column. */
export const LedgerTable = page(Ledger);
/** C · Stacked columns, oldest month on the left. */
export const ColumnChart = page(Columns);
/** D · One line a month, distance labelled at the bar's end. */
export const LabelledBars = page(Labelled);
/** E · A tile a month. */
export const MonthTiles = page(Tiles);
/** F · Each month against the same month last year. */
export const AgainstLastYearBars = page(AgainstLastYear);
