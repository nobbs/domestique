/**
 * A route's own ride history, read off the activities the page already holds.
 *
 * Every recorded ride carries the route it was matched to and the share of each
 * the two had in common, so a route's history is a fold over that one list
 * rather than a request of its own.
 */

import type { Activity, ActivityRouteMatch, Route } from "../api/types";
import { routeKey } from "../api/types";

/** How much of a route a ride must cover to count as a lap of it. The match
 * itself admits 0.92, which is a shorter ride than the whole laps rode. */
export const WHOLE_LAP_COVERAGE = 0.99;

export interface RiddenRide {
  ride: Activity;
  match: ActivityRouteMatch;
  /** Whether the whole route was covered. A partial lap competes for no best. */
  whole: boolean;
  /** Average speed over the ride's moving time, null where it moved for none. */
  speedKmh: number | null;
  /** The fastest whole lap, which is the only row a best is claimed for. */
  personalBest: boolean;
}

function averageSpeed(ride: Activity): number | null {
  if (!Number.isFinite(ride.movingSeconds) || ride.movingSeconds <= 0) {
    return null;
  }

  return (ride.distanceMetres / ride.movingSeconds) * 3.6;
}

/** The rides matched to one route, newest first. Direction is carried on each
 * rather than folded into the best, which the reader can see for themselves. */
export function riddenOn(
  activities: Activity[],
  route: Pick<Route, "provider" | "sourceRouteId" | "stageOrder">,
): RiddenRide[] {
  const key = routeKey(route);
  const ridden: Omit<RiddenRide, "personalBest">[] = [];
  for (const ride of activities) {
    const match = ride.routeMatch;
    if (!match || routeKey(match) !== key) {
      continue;
    }
    ridden.push({
      ride,
      match,
      whole: match.routeCoverage >= WHOLE_LAP_COVERAGE,
      speedKmh: averageSpeed(ride),
    });
  }
  ridden.sort((left, right) => right.ride.startedAt.localeCompare(left.ride.startedAt));
  // Ties keep the earliest of the sorted rows, which is the most recent ride.
  const best = ridden
    .filter((entry) => entry.whole && entry.ride.movingSeconds > 0)
    .reduce<(typeof ridden)[number] | null>(
      (fastest, entry) =>
        fastest === null || entry.ride.movingSeconds < fastest.ride.movingSeconds ? entry : fastest,
      null,
    );

  return ridden.map((entry) => ({ ...entry, personalBest: entry === best }));
}
