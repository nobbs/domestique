/**
 * What this route has actually been ridden, as the dock's third stop.
 *
 * The list is only ever shown for a route with rides behind it — the stop
 * itself is absent otherwise, so there is no empty state to write.
 */

import { IconTrophy } from "@tabler/icons-react";
import { Link } from "react-router";
import { formatDuration, formatSpeed, formatTimestamp } from "../../lib/format";
import type { RiddenRide } from "../../lib/rideHistory";
import { conditionsSentence } from "../../lib/weather";

function HistoryRow({ entry }: { entry: RiddenRide }) {
  const { ride, match, whole, speedKmh, personalBest } = entry;

  return (
    <li>
      <Link
        to={`/activities/${ride.id}`}
        className="flex flex-wrap items-baseline gap-x-3 rounded-md px-2 py-1 text-xs hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)]"
      >
        <span className="w-40 shrink-0 text-[var(--ink-2)]">{formatTimestamp(ride.startedAt)}</span>
        <span className="w-20 shrink-0 font-semibold text-sm tabular-nums">
          {formatDuration(ride.movingSeconds)}
        </span>
        <span className="w-20 shrink-0 tabular-nums">{formatSpeed(speedKmh)}</span>
        {personalBest ? (
          <span className="inline-flex items-center gap-1 text-[var(--accent)]">
            <IconTrophy size={12} stroke={2} aria-hidden="true" />
            Best
          </span>
        ) : null}
        {whole ? null : (
          <span className="text-[var(--ink-2)]">
            {Math.round(match.routeCoverage * 100)}% of the route
          </span>
        )}
        {match.direction === "reverse" ? (
          <span className="text-[var(--ink-2)]">ridden in reverse</span>
        ) : null}
        {ride.weather ? (
          <span className="ml-auto text-[var(--ink-2)]">{conditionsSentence(ride.weather)}</span>
        ) : null}
      </Link>
    </li>
  );
}

export function RouteHistory({ rides }: { rides: RiddenRide[] }) {
  return (
    <ul aria-label="Ride history" className="min-h-0 flex-1 overflow-y-auto">
      {rides.map((entry) => (
        <HistoryRow key={entry.ride.id} entry={entry} />
      ))}
    </ul>
  );
}
