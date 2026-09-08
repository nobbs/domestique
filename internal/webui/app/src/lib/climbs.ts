/**
 * One sustained climb of a route.
 *
 * Found by the service, not here. The rule — a run whose gradient, measured
 * back over a hundred metres, holds at three percent or more, reported once it
 * is at least a window long — is stated once in
 * `docs/specs/measurement.md` §Sustained climbs and applied once, in
 * `internal/measure/climb.go`. This browser drew the same rule for itself until
 * the service began timing rides over these climbs; two implementations of one
 * rule can only ever come to disagree, and a climb the two put in different
 * places is a rider's time shown against the wrong hill.
 */

import type { RouteClimb } from "../api/types";

export type Climb = RouteClimb;
