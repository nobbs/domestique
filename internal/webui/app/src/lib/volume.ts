/**
 * Recorded activities gathered into the periods a rider thinks in.
 *
 * Every date here is read in the browser's own zone: the web UI's config
 * carries no service timezone, and the one place the service's zone is stated
 * is the shared settings, which a rider is answered 403 for. A week boundary
 * an hour out of step with the service is a smaller wrong than a page that
 * cannot load for anyone but an admin.
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

const WEEK_LABEL = new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short" });
const MONTH_LABEL = new Intl.DateTimeFormat(undefined, { month: "short", year: "numeric" });

/** The Monday of `date`'s ISO week, at midnight. */
function startOfWeek(date: Date): Date {
  const start = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  start.setDate(start.getDate() - ((start.getDay() + 6) % 7));

  return start;
}

function startOfBucket(date: Date, granularity: Granularity): Date {
  return granularity === "week"
    ? startOfWeek(date)
    : new Date(date.getFullYear(), date.getMonth(), 1);
}

function previousBucket(start: Date, granularity: Granularity): Date {
  return granularity === "week"
    ? new Date(start.getFullYear(), start.getMonth(), start.getDate() - 7)
    : new Date(start.getFullYear(), start.getMonth() - 1, 1);
}

function empty(start: Date, granularity: Granularity): VolumeBucket {
  return {
    start,
    label: (granularity === "week" ? WEEK_LABEL : MONTH_LABEL).format(start),
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
  now = new Date(),
): VolumeBucket[] {
  const dated = dateActivities(activities);
  if (dated.length === 0) {
    return [];
  }

  const earliest = startOfBucket(
    new Date(dated.reduce((low, { startedAt }) => Math.min(low, startedAt.getTime()), Infinity)),
    granularity,
  );
  const buckets: VolumeBucket[] = [];
  const byStart = new Map<number, VolumeBucket>();
  for (
    let start = startOfBucket(now, granularity);
    start.getTime() >= earliest.getTime();
    start = previousBucket(start, granularity)
  ) {
    const bucket = empty(start, granularity);
    buckets.push(bucket);
    byStart.set(start.getTime(), bucket);
  }

  dated.forEach(({ activity, startedAt }) => {
    const bucket = byStart.get(startOfBucket(startedAt, granularity).getTime());
    if (bucket) {
      add(bucket, activity);
    }
  });

  return buckets;
}

/** How far back the page asks for; a whole day, so the query key holds still. */
export const WINDOW_DAYS = 365;

/** The inclusive start of the asked-for window, as the service reads it. */
export function windowStart(now = new Date()): string {
  const start = new Date(now.getFullYear(), now.getMonth(), now.getDate() - WINDOW_DAYS);

  return start.toISOString();
}
