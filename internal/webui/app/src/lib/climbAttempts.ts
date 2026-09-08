/**
 * The rider's own attempts at a route's climbs, paired to the climbs this
 * browser found.
 *
 * The service finds a route's climbs by the same rule this browser does, over
 * the same stored geometry, so the two agree — but they are two
 * implementations, and an attempt shown against the wrong climb is worse than
 * an attempt not shown at all. Pairing is therefore by where a climb starts
 * rather than by its position in either list: a climb the two disagree about
 * simply carries no times.
 */

import type { RouteClimb, RouteClimbAttempt } from "../api/types";
import type { Climb } from "./climbs";

/**
 * How far apart two starts may be and still be the same climb. One gradient
 * window: nearer than that and no two climbs of one route can be confused.
 */
const SAME_CLIMB_METRES = 100;

/** What the sidebar says about one climb: the quickest attempt, and the last. */
export interface ClimbTimes {
  bestSeconds: number;
  lastSeconds: number;
  attempts: RouteClimbAttempt[];
}

/**
 * The times for each climb this browser found, by its index in that list. A
 * climb with no attempt, or one the service put somewhere else, is absent.
 */
export function climbTimes(found: Climb[], served: RouteClimb[]): Map<number, ClimbTimes> {
  const times = new Map<number, ClimbTimes>();
  found.forEach((climb, index) => {
    const match = served.find(
      (one) => Math.abs(one.startMetres - climb.startMetres) <= SAME_CLIMB_METRES,
    );
    if (match === undefined || match.attempts.length === 0) {
      return;
    }
    // Served quickest first, so the best is the head; the last is whichever was
    // ridden most recently, which that order says nothing about.
    const best = match.attempts[0];
    const last = match.attempts.reduce((latest, one) =>
      Date.parse(one.riddenAt) > Date.parse(latest.riddenAt) ? one : latest,
    );
    if (best === undefined) {
      return;
    }
    times.set(index, {
      bestSeconds: best.seconds,
      lastSeconds: last.seconds,
      attempts: match.attempts,
    });
  });

  return times;
}
