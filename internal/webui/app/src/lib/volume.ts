/**
 * Recorded activities gathered into the periods a rider thinks in.
 *
 * Every bucket edge — the Monday a week starts, the first of a month, the
 * window's start — is formed in the service's own zone, passed in by the
 * caller, so two riders in different zones agree on which week a Sunday-night
 * ride belongs to.
 */

import type { Activity } from "../api/types";

export type Granularity = "week" | "month";

export interface VolumeTotals {
  distanceMetres: number;
  movingSeconds: number;
  ascentMetres: number;
  count: number;
}

export interface VolumeBucket extends VolumeTotals {
  start: Date;
  label: string;
}

const WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

interface ZonedParts {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
  weekday: number;
}

// One formatter per zone: bucketing reads the parts of many instants in the
// same zone, and constructing a formatter is the expensive half of that.
const PARTS_FORMATTERS = new Map<string, Intl.DateTimeFormat>();

function partsFormatter(zone: string): Intl.DateTimeFormat {
  let formatter = PARTS_FORMATTERS.get(zone);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat("en-US", {
      timeZone: zone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      weekday: "short",
      hourCycle: "h23",
    });
    PARTS_FORMATTERS.set(zone, formatter);
  }

  return formatter;
}

/** `date`'s calendar fields as read in `zone`, weekday 0 (Sunday) to 6. */
function zonedParts(date: Date, zone: string): ZonedParts {
  const parts = Object.fromEntries(
    partsFormatter(zone)
      .formatToParts(date)
      .map((part) => [part.type, part.value]),
  );

  return {
    year: Number(parts.year),
    month: Number(parts.month),
    day: Number(parts.day),
    hour: Number(parts.hour) % 24,
    minute: Number(parts.minute),
    second: Number(parts.second),
    weekday: WEEKDAYS.indexOf(parts.weekday ?? ""),
  };
}

/** Minutes `zone` is ahead of UTC at `date`, positive east of Greenwich. */
function offsetMinutes(date: Date, zone: string): number {
  const parts = zonedParts(date, zone);
  const asUTC = Date.UTC(
    parts.year,
    parts.month - 1,
    parts.day,
    parts.hour,
    parts.minute,
    parts.second,
  );

  return (asUTC - date.getTime()) / 60_000;
}

/**
 * The instant that is midnight on `year`-`month`-`day` in `zone`.
 *
 * Guesses the offset from that date read as UTC, then re-reads it at the
 * guessed instant: the one DST-edge case where that shifts the offset again
 * cannot land on a date this call is asked to compute a whole-day edge for.
 */
function zonedMidnight(year: number, month: number, day: number, zone: string): Date {
  const guess = Date.UTC(year, month - 1, day);
  const instant = new Date(guess - offsetMinutes(new Date(guess), zone) * 60_000);

  return new Date(guess - offsetMinutes(instant, zone) * 60_000);
}

const LABEL_FORMATTERS = new Map<string, Intl.DateTimeFormat>();

function labelFormatter(zone: string, granularity: Granularity): Intl.DateTimeFormat {
  const key = `${granularity}:${zone}`;
  let formatter = LABEL_FORMATTERS.get(key);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat(
      undefined,
      granularity === "week"
        ? { day: "numeric", month: "short", timeZone: zone }
        : { month: "short", year: "numeric", timeZone: zone },
    );
    LABEL_FORMATTERS.set(key, formatter);
  }

  return formatter;
}

/** The Monday of `date`'s ISO week, at midnight, in `zone`. */
function startOfWeek(date: Date, zone: string): Date {
  const { year, month, day, weekday } = zonedParts(date, zone);

  return zonedMidnight(year, month, day - ((weekday + 6) % 7), zone);
}

function startOfBucket(date: Date, granularity: Granularity, zone: string): Date {
  if (granularity === "week") {
    return startOfWeek(date, zone);
  }
  const { year, month } = zonedParts(date, zone);

  return zonedMidnight(year, month, 1, zone);
}

function previousBucket(start: Date, granularity: Granularity, zone: string): Date {
  const { year, month, day } = zonedParts(start, zone);

  return granularity === "week"
    ? zonedMidnight(year, month, day - 7, zone)
    : zonedMidnight(year, month - 1, 1, zone);
}

function empty(start: Date, granularity: Granularity, zone: string): VolumeBucket {
  return {
    start,
    label: labelFormatter(zone, granularity).format(start),
    distanceMetres: 0,
    movingSeconds: 0,
    ascentMetres: 0,
    count: 0,
  };
}

function add(bucket: VolumeTotals, activity: Activity): void {
  bucket.distanceMetres += activity.distanceMetres;
  bucket.movingSeconds += activity.movingSeconds;
  bucket.ascentMetres += activity.ascentMetres;
  bucket.count += 1;
}

/** Distance, time, climbing and rides across every activity given. */
export function volumeTotals(activities: Activity[]): VolumeTotals {
  const totals: VolumeTotals = {
    distanceMetres: 0,
    movingSeconds: 0,
    ascentMetres: 0,
    count: 0,
  };
  dateActivities(activities).forEach(({ activity }) => {
    add(totals, activity);
  });

  return totals;
}

/** Activities with a readable start; the rest count nowhere on this page. */
function dateActivities(activities: Activity[]) {
  return activities
    .map((activity) => ({ activity, startedAt: new Date(activity.startedAt) }))
    .filter(({ startedAt }) => !Number.isNaN(startedAt.getTime()));
}

/**
 * One bucket per period from the earliest activity to `now`, newest first.
 *
 * Periods nobody rode in are present and zero: a gap is a fact about the
 * riding, and a list that closed over it would read as an unbroken run.
 */
export function bucketActivities(
  activities: Activity[],
  granularity: Granularity,
  zone: string,
  now = new Date(),
): VolumeBucket[] {
  const dated = dateActivities(activities);
  if (dated.length === 0) {
    return [];
  }

  const earliest = startOfBucket(
    new Date(dated.reduce((low, { startedAt }) => Math.min(low, startedAt.getTime()), Infinity)),
    granularity,
    zone,
  );
  const buckets: VolumeBucket[] = [];
  const byStart = new Map<number, VolumeBucket>();
  for (
    let start = startOfBucket(now, granularity, zone);
    start.getTime() >= earliest.getTime();
    start = previousBucket(start, granularity, zone)
  ) {
    const bucket = empty(start, granularity, zone);
    buckets.push(bucket);
    byStart.set(start.getTime(), bucket);
  }

  dated.forEach(({ activity, startedAt }) => {
    const bucket = byStart.get(startOfBucket(startedAt, granularity, zone).getTime());
    if (bucket) {
      add(bucket, activity);
    }
  });

  return buckets;
}

/** How far back the page asks for; a whole day, so the query key holds still. */
export const WINDOW_DAYS = 365;

/** The inclusive start of the asked-for window, midnight in `zone`. */
export function windowStart(zone: string, now = new Date()): string {
  const { year, month, day } = zonedParts(now, zone);

  return zonedMidnight(year, month, day - WINDOW_DAYS, zone).toISOString();
}
