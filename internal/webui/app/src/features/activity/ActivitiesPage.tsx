/**
 * The rides themselves, arranged by week: newest week first, each ride a bar
 * sized by distance so a week's rhythm reads at a glance. Which week and
 * which weekday a ride belongs to is decided in the service's own time zone,
 * exactly as Volume decides it.
 */

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { Link } from "react-router";
import { webUIConfigQuery } from "../../api/queries";
import type { Activity } from "../../api/types";
import { PageShell } from "../../components/Layout";
import { Skeleton } from "../../components/ui/skeleton";
import {
  formatAscent,
  formatDistance,
  formatDuration,
  formatPrecipitation,
} from "../../lib/format";
import { type RideWeek, weekdayIndex, weekRangeLabel, weeksWithRides } from "../../lib/volume";
import { temperatureColour, weatherIcon } from "../../lib/weather";

import { useActivities } from "./useActivities";

const WEEKDAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
const WEEK_COLUMNS = "11rem repeat(7, minmax(0, 1fr))";
/** The tallest bar, which the longest ride on the page gets; every other bar scales to it. */
const BAR_REM = 6;

// Only while the config is unavailable: once it answers, its zone is the one
// used, whether or not it happens to match the browser's own.
const browserZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;

function DayLabel({ children }: { children: string }) {
  return (
    <span className="font-semibold text-[10px] text-[var(--ink-2)] uppercase tracking-[0.08em]">
      {children}
    </span>
  );
}

function Weather({ ride }: { ride: Activity }) {
  if (!ride.weather) {
    return null;
  }
  const Glyph = weatherIcon(ride.weather.weatherCode);

  return (
    <span className="flex items-center gap-1 text-[10px] tabular-nums">
      <Glyph size={14} stroke={1.6} aria-hidden="true" className="text-[var(--ink-2)]" />
      <span style={{ color: temperatureColour(ride.weather.temperatureMaxCelsius) }}>
        {Math.round(ride.weather.temperatureMaxCelsius)}°
      </span>
      {ride.weather.precipitationMillimetres > 0 ? (
        <span className="text-[var(--rain-2)]">
          {formatPrecipitation(ride.weather.precipitationMillimetres)}
        </span>
      ) : null}
    </span>
  );
}

function RideBar({ ride, longest }: { ride: Activity; longest: number }) {
  return (
    <Link
      to={`/activities/${ride.id}`}
      className="flex items-end gap-1.5 rounded text-left hover:bg-[var(--base)]"
    >
      <span
        aria-hidden="true"
        className="w-3 shrink-0 rounded-t bg-[var(--accent)]"
        style={{ height: `${(ride.distanceMetres / longest) * BAR_REM}rem` }}
      />
      <span className="flex flex-col gap-0.5 pb-0.5">
        <span className="font-semibold text-sm tabular-nums">
          {formatDistance(ride.distanceMetres)}
        </span>
        <span className="text-[10px] text-[var(--ink-2)] tabular-nums">
          {formatAscent(ride.ascentMetres)}
        </span>
        <Weather ride={ride} />
      </span>
    </Link>
  );
}

function WeekPanel({ week, zone, longest }: { week: RideWeek; zone: string; longest: number }) {
  const range = weekRangeLabel(week.start, zone);

  if (week.count === 0) {
    return (
      <section
        aria-label={`Week ${range}`}
        className="rounded-xl bg-[var(--panel)] px-3 py-2 ring-1 ring-black/5"
      >
        <span className="font-medium text-sm">{range}</span>
        <span className="ml-2 text-[var(--ink-2)] text-xs">No rides</span>
      </section>
    );
  }

  return (
    <section
      aria-label={`Week ${range}`}
      className="grid gap-x-2 rounded-xl bg-[var(--panel)] p-2 ring-1 ring-black/5"
      style={{ gridTemplateColumns: WEEK_COLUMNS }}
    >
      <div className="flex flex-col gap-2 pr-2">
        <span className="font-medium text-sm">{range}</span>
        <div className="flex flex-col gap-1 text-xs tabular-nums">
          <span className="font-semibold text-base">{formatDistance(week.distanceMetres)}</span>
          <span className="text-[var(--ink-2)]">
            {formatDuration(week.movingSeconds)} · {formatAscent(week.ascentMetres)}
          </span>
        </div>
      </div>
      {WEEKDAYS.map((day, index) => (
        // The floor every bar stands on, so a day with one short ride is read
        // against the same height as a day with a long one.
        <div
          key={day}
          role="group"
          aria-label={day}
          className="flex items-end gap-2 border-[var(--rule)] border-b pb-1"
          style={{ height: `${BAR_REM + 1.5}rem` }}
        >
          {week.rides
            .filter((ride) => weekdayIndex(ride, zone) === index)
            .map((ride) => (
              <RideBar key={ride.id} ride={ride} longest={longest} />
            ))}
        </div>
      ))}
    </section>
  );
}

export function ActivitiesPage() {
  const config = useQuery(webUIConfigQuery());
  const serviceZone = config.data?.timezone || null;
  const zone = serviceZone ?? browserZone();
  const { activities, isPending, isError } = useActivities();
  const weeks = useMemo(() => weeksWithRides(activities, zone), [activities, zone]);
  // From the rides on the page: one with an unreadable start counts nowhere here.
  const longest = Math.max(
    ...weeks.flatMap((week) => week.rides.map((ride) => ride.distanceMetres)),
    1,
  );

  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-4">
        <h1 className="font-semibold text-2xl tracking-tight">Activities</h1>
        {!config.isPending && serviceZone === null && (
          <p className="text-[var(--ink-2)] text-xs">Periods follow this browser's time zone</p>
        )}
        {isPending ? (
          <Skeleton className="h-64 w-full" role="status" aria-label="Loading activities" />
        ) : isError ? (
          <p className="text-sm text-[var(--alert)]">
            The service did not say what has been ridden.
          </p>
        ) : weeks.length === 0 ? (
          <p className="text-[var(--ink-2)] text-sm">
            No rides have been recorded yet. Once a Wahoo account is connected on{" "}
            <Link className="underline" to="/settings">
              settings
            </Link>
            , the rides it records appear here.
          </p>
        ) : (
          <>
            <div className="grid gap-x-2 px-2" style={{ gridTemplateColumns: WEEK_COLUMNS }}>
              <span />
              {WEEKDAYS.map((day) => (
                <DayLabel key={day}>{day}</DayLabel>
              ))}
            </div>
            {weeks.map((week) => (
              <WeekPanel key={week.start.toISOString()} week={week} zone={zone} longest={longest} />
            ))}
          </>
        )}
      </div>
    </PageShell>
  );
}
