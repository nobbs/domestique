/**
 * The rides themselves, newest first: Volume answers how much has been ridden,
 * this answers which rides those were. Each row leads to the ride's own page.
 */

import { useMemo } from "react";
import { Link } from "react-router";
import type { ActivityWeatherSummary } from "../../api/types";
import { PageShell } from "../../components/Layout";
import { Skeleton } from "../../components/ui/skeleton";
import {
  formatAscent,
  formatDistance,
  formatMovingTime,
  formatPrecipitation,
  formatTimestamp,
  formatWindSpeed,
} from "../../lib/format";

/**
 * What the ride was ridden through, in one line: the range the temperature
 * moved over, the wind, and what fell if anything did. A dry ride says nothing
 * about rain rather than saying none fell — the absence is the reading.
 */
function conditionsLine(weather: ActivityWeatherSummary): string {
  const low = Math.round(weather.temperatureMinCelsius);
  const high = Math.round(weather.temperatureMaxCelsius);
  const temperature = low === high ? `${low}°` : `${low}–${high}°`;
  const parts = [temperature, formatWindSpeed(weather.windSpeedKmh)];
  if (weather.precipitationMillimetres > 0) {
    parts.push(formatPrecipitation(weather.precipitationMillimetres));
  }

  return parts.join(" · ");
}

import { useActivities } from "./useActivities";

export function ActivitiesPage() {
  const { activities, isPending, isError } = useActivities();
  const rides = useMemo(
    () => [...activities].sort((first, second) => second.startedAt.localeCompare(first.startedAt)),
    [activities],
  );

  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-5">
        <h1 className="font-semibold text-2xl tracking-tight">Activities</h1>
        {isPending ? (
          <Skeleton className="h-64 w-full" role="status" aria-label="Loading activities" />
        ) : isError ? (
          <p className="text-sm text-[var(--alert)]">
            The service did not say what has been ridden.
          </p>
        ) : rides.length === 0 ? (
          <p className="text-[var(--ink-2)] text-sm">
            No rides have been recorded yet. Once a Wahoo account is connected on{" "}
            <Link className="underline" to="/settings">
              settings
            </Link>
            , the rides it records appear here.
          </p>
        ) : (
          <ul className="flex flex-col gap-2">
            {rides.map((ride) => (
              <li key={ride.id}>
                <Link
                  to={`/activities/${ride.id}`}
                  className="flex flex-col gap-1 rounded-lg bg-[var(--panel)] px-3 py-2 ring-1 ring-black/5 hover:bg-[var(--base)]"
                >
                  <span className="font-medium text-sm">{formatTimestamp(ride.startedAt)}</span>
                  <span className="text-[var(--ink-2)] text-xs">
                    {[
                      formatDistance(ride.distanceMetres),
                      formatMovingTime(ride.movingSeconds),
                      formatAscent(ride.ascentMetres),
                    ].join(" · ")}
                  </span>
                  {/*
                   * A second line rather than more of the first: what the ride
                   * was is one sentence and what it was ridden through is
                   * another, and a card that runs them together reads as neither.
                   */}
                  {ride.weather ? (
                    <span className="text-[var(--ink-2)] text-xs">
                      {conditionsLine(ride.weather)}
                    </span>
                  ) : null}
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
    </PageShell>
  );
}
