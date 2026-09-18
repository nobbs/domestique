/**
 * A month of days, each marked where a ride started on it: olive outdoor, slate
 * indoor, split on the diagonal where the day had both. Days are read in the
 * service's time zone, as every other period on the page is.
 */

import { IconCalendarCheck, IconChevronLeft, IconChevronRight } from "@tabler/icons-react";
import { useState } from "react";
import type { Activity } from "../api/types";

type Ground = "outdoor" | "indoor" | "both";

/** Each ridden day as `YYYY-MM-DD` in `zone`, with the ground it counts as. */
export function riddenDays(rides: readonly Activity[], zone: string): Map<string, Ground> {
  const day = new Intl.DateTimeFormat("en-CA", { timeZone: zone });
  const days = new Map<string, Ground>();
  for (const ride of rides) {
    const started = new Date(ride.startedAt);
    if (Number.isNaN(started.getTime())) {
      continue;
    }
    const key = day.format(started);
    const ground = ride.indoor ? "indoor" : "outdoor";
    const before = days.get(key);
    days.set(key, before && before !== ground ? "both" : ground);
  }
  return days;
}

const PAINT: Record<Ground, string> = {
  outdoor: "var(--ground-outdoor)",
  indoor: "var(--ground-indoor)",
  both: "linear-gradient(135deg, var(--ground-outdoor) 0 50%, var(--ground-indoor) 50% 100%)",
};
const WEEKDAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
const NAV =
  "grid size-7 place-items-center rounded-[9px] text-[var(--ink-2)] hover:bg-[color-mix(in_oklab,var(--ink-2)_10%,transparent)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-[var(--accent)] focus-visible:outline-offset-2";
const pad = (value: number) => String(value).padStart(2, "0");

export function RideCalendar({
  rides,
  zone,
  listed,
  onDay,
}: {
  rides: readonly Activity[];
  zone: string;
  /** Days the page can show a ride for; only these become buttons. */
  listed?: ReadonlySet<string>;
  onDay?: (day: string) => void;
}) {
  const days = riddenDays(rides, zone);
  // Opens on the newest ridden month, or this one when nothing has been ridden.
  const newest =
    [...days.keys()].sort().at(-1) ?? new Intl.DateTimeFormat("en-CA", { timeZone: zone }).format();
  const [cursor, setCursor] = useState(() => ({
    year: Number(newest.slice(0, 4)),
    month: Number(newest.slice(5, 7)) - 1,
  }));
  const first = new Date(Date.UTC(cursor.year, cursor.month, 1));
  const length = new Date(Date.UTC(cursor.year, cursor.month + 1, 0)).getUTCDate();
  const lead = (first.getUTCDay() + 6) % 7;
  const title = first.toLocaleString("en-GB", { month: "long", year: "numeric", timeZone: "UTC" });
  const toToday = () => {
    const today = new Intl.DateTimeFormat("en-CA", { timeZone: zone }).format();
    setCursor({ year: Number(today.slice(0, 4)), month: Number(today.slice(5, 7)) - 1 });
  };
  const step = (by: number) =>
    setCursor(({ year, month }) => {
      const next = new Date(Date.UTC(year, month + by, 1));
      return { year: next.getUTCFullYear(), month: next.getUTCMonth() };
    });

  return (
    <section
      aria-label={`Days ridden in ${title}`}
      className="flex flex-col gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)]"
    >
      <div className="flex items-center gap-3">
        <button
          type="button"
          aria-label="This month"
          onClick={toToday}
          className="grid size-9 place-items-center rounded-[9px] [background:var(--mark)] text-[var(--mark-ink)] hover:brightness-125 focus-visible:outline-2 focus-visible:outline-[var(--accent)] focus-visible:outline-offset-2"
        >
          <IconCalendarCheck size={18} stroke={1.8} aria-hidden="true" />
        </button>
        <h2 className="flex-1 font-semibold">{title}</h2>
        <button type="button" aria-label="Previous month" className={NAV} onClick={() => step(-1)}>
          <IconChevronLeft size={16} aria-hidden="true" />
        </button>
        <button type="button" aria-label="Next month" className={NAV} onClick={() => step(1)}>
          <IconChevronRight size={16} aria-hidden="true" />
        </button>
      </div>
      <div className="grid grid-cols-7 gap-1.5 text-center text-xs">
        {WEEKDAYS.map((weekday) => (
          <span key={weekday} aria-hidden="true" className="pb-1 text-[var(--ink-2)]">
            {weekday[0]}
          </span>
        ))}
        {Array.from({ length: lead }, (_, index) => (
          // biome-ignore lint/suspicious/noArrayIndexKey: blank cells before the first, never reordered
          <span key={`lead-${index}`} />
        ))}
        {Array.from({ length: length }, (_, index) => {
          const date = index + 1;
          const key = `${cursor.year}-${pad(cursor.month + 1)}-${pad(date)}`;
          const ground = days.get(key);
          const cell = {
            "aria-label": `${date} ${title}: ${ground ? `ridden, ${ground}` : "no ride"}`,
            className: `grid aspect-square place-items-center rounded-[9px] tabular-nums ${ground ? "font-semibold text-white" : "bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] text-[var(--ink-2)]"}`,
            style: ground ? { background: PAINT[ground] } : undefined,
          };
          return ground && onDay && listed?.has(key) ? (
            <button
              key={date}
              type="button"
              {...cell}
              className={`${cell.className} hover:brightness-110 focus-visible:outline-2 focus-visible:outline-[var(--accent)] focus-visible:outline-offset-2`}
              onClick={() => onDay(key)}
            >
              {date}
            </button>
          ) : (
            <span key={date} role="img" {...cell}>
              {date}
            </span>
          );
        })}
      </div>
    </section>
  );
}
