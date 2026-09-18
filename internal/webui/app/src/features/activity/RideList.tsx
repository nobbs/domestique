/**
 * The rides themselves, newest first: one inset block per week or month with its
 * totals in the heading, a row per ride leading to that ride's page. Weeks and
 * weekdays are read in the service's time zone, exactly as the charts read them.
 */

import { IconBike, IconHeartbeat } from "@tabler/icons-react";
import { useMemo, useState } from "react";
import { Link } from "react-router";
import type { Activity } from "../../api/types";
import { Panel } from "../../components/PanelHeading";
import { Segmented } from "../../components/Segmented";
import {
  formatAscent,
  formatDistance,
  formatDuration,
  formatPrecipitation,
} from "../../lib/format";
import { type Granularity, ridesByPeriod, weekRangeLabel } from "../../lib/volume";
import { temperatureColour, weatherIcon } from "../../lib/weather";

function Weather({ ride }: { ride: Activity }) {
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
        <span className="text-[var(--rain-2)]">
          {formatPrecipitation(ride.weather.precipitationMillimetres)}
        </span>
      ) : null}
    </span>
  );
}

const PERIODS = [
  { key: "week", label: "Week" },
  { key: "month", label: "Month" },
] as const;

export function RideList({
  rides,
  zone,
  highlight,
}: {
  rides: Activity[];
  zone: string;
  /** A `YYYY-MM-DD` day in `zone` whose rides are tinted, as the calendar asks. */
  highlight?: string | null;
}) {
  const [period, setPeriod] = useState<Granularity>("week");
  const groups = useMemo(
    () => ridesByPeriod(rides, period, zone).filter((group) => group.count > 0),
    [rides, period, zone],
  );
  // Every bar is read against the longest ride in the list, so rows compare down the page.
  const longest = Math.max(1, ...rides.map((ride) => ride.distanceMetres));
  const day = new Intl.DateTimeFormat("en-GB", {
    weekday: "short",
    day: "numeric",
    month: "short",
    timeZone: zone,
  });
  const dayKey = new Intl.DateTimeFormat("en-CA", { timeZone: zone });
  const time = new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    timeZone: zone,
  });

  return (
    <Panel
      icon={<IconBike size={18} stroke={1.8} />}
      title="Rides"
      subtitle="newest first"
      aside={
        <Segmented label="Group by" size="sm" items={PERIODS} value={period} onChange={setPeriod} />
      }
    >
      <div className="flex flex-col gap-4">
        {groups.map((group) => {
          const range = period === "week" ? weekRangeLabel(group.start, zone) : group.label;
          return (
            <section
              key={group.start.toISOString()}
              aria-label={`${period === "week" ? "Week" : "Month"} ${range}`}
              className="flex flex-col gap-1.5"
            >
              <h3 className="flex flex-wrap items-baseline justify-between gap-x-3 px-1 text-sm">
                <span className="font-semibold">{range}</span>
                <span className="text-[var(--ink-2)] text-xs tabular-nums">
                  {formatDistance(group.distanceMetres)} · {formatDuration(group.movingSeconds)} ·{" "}
                  {formatAscent(group.ascentMetres)}
                </span>
              </h3>
              {/* 11px is the bar's 7px plus its 4px inset, so the two corners run concentric. */}
              <ul className="flex flex-col overflow-hidden rounded-[11px] bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]">
                {group.rides.map((ride) => {
                  const colour = ride.indoor ? "var(--ground-indoor)" : "var(--ground-outdoor)";
                  const started = new Date(ride.startedAt);
                  return (
                    <li key={ride.id} className="border-[var(--panel)] border-b-2 last:border-b-0">
                      <Link
                        to={`/activities/${ride.id}`}
                        data-day={dayKey.format(started)}
                        data-highlighted={dayKey.format(started) === highlight || undefined}
                        className="relative flex items-center gap-3 px-3 py-1.5 before:absolute before:inset-1 before:rounded-[7px] before:transition-colors before:duration-700 data-highlighted:before:bg-[color-mix(in_oklab,var(--accent)_22%,transparent)] hover:before:bg-[color-mix(in_oklab,var(--ink-2)_8%,transparent)] focus-visible:outline-2 focus-visible:outline-[var(--accent)] focus-visible:outline-offset-[-2px]"
                      >
                        <span
                          aria-hidden="true"
                          className="absolute inset-y-1 left-1 rounded-[7px] opacity-15"
                          style={{
                            width: `calc(${(ride.distanceMetres / longest) * 100}% - 0.5rem)`,
                            background: colour,
                          }}
                        />
                        <span className="relative flex min-w-0 flex-1 flex-wrap items-center gap-x-3 text-sm">
                          <span
                            role="img"
                            aria-label={ride.indoor ? "Indoor" : "Outdoor"}
                            className="grid size-7 shrink-0 place-items-center rounded-[7px] text-[var(--panel)]"
                            style={{ background: colour }}
                          >
                            <IconBike size={15} stroke={1.8} aria-hidden="true" />
                          </span>
                          <span className="w-24 font-medium">{day.format(started)}</span>
                          <span className="font-semibold tabular-nums">
                            {formatDistance(ride.distanceMetres)}
                          </span>
                          <span className="flex flex-wrap items-center gap-x-3 text-[var(--ink-2)] text-xs tabular-nums">
                            <span>{formatDuration(ride.movingSeconds)}</span>
                            <span>{formatAscent(ride.ascentMetres)}</span>
                            {ride.metrics?.heartRateTss ? (
                              <span className="inline-flex items-center gap-0.5">
                                <IconHeartbeat size={12} aria-hidden="true" />
                                {Math.round(ride.metrics.heartRateTss)} TSS
                              </span>
                            ) : null}
                            <Weather ride={ride} />
                            {ride.provider === "zwift" ? <span>Zwift</span> : null}
                          </span>
                        </span>
                        {/* Arrival is start plus elapsed time, stops included. */}
                        <span className="relative text-[var(--ink-2)] text-xs tabular-nums">
                          {time.format(started)}–
                          {time.format(new Date(started.getTime() + ride.elapsedSeconds * 1000))}
                        </span>
                      </Link>
                    </li>
                  );
                })}
              </ul>
            </section>
          );
        })}
      </div>
    </Panel>
  );
}
