import type { PlanAvoid } from "../../api/generated";
import {
  PLAN_PROFILES,
  type Plan,
  type PlanProfile,
  type PlanWaypoint,
  type Position,
} from "../../api/types";

export interface PlannerState {
  name: string;
  profile: PlanProfile;
  cues: boolean;
  waypoints: PlannerWaypoint[];
  nextWaypointID: number;
  avoid: PlannerAvoid[];
  nextAvoidID: number;
  past: PlannerSnapshot[];
  future: PlannerSnapshot[];
}

interface PlannerSnapshot {
  name: string;
  profile: PlanProfile;
  cues: boolean;
  waypoints: PlannerWaypoint[];
  nextWaypointID: number;
  avoid: PlannerAvoid[];
  nextAvoidID: number;
}

export interface PlannerWaypoint extends PlanWaypoint {
  id: number;
}

export interface PlannerAvoid extends PlanAvoid {
  id: number;
}

/** A new local plan prefilled from the route currently open in the library. */
export interface PlannerSeed {
  name: string;
  profile: PlanProfile;
  waypoints: PlanWaypoint[];
}

/** Keeps a source line within the planner API's waypoint cap without losing its ends. */
export function samplePlanWaypoints(coordinates: Position[]): PlanWaypoint[] {
  const valid = coordinates.flatMap(([longitude, latitude]) =>
    Number.isFinite(longitude) &&
    Number.isFinite(latitude) &&
    longitude >= -180 &&
    longitude <= 180 &&
    latitude >= -90 &&
    latitude <= 90
      ? [{ longitude, latitude }]
      : [],
  );
  if (valid.length < 2) {
    return [];
  }
  const first = valid[0];
  if (!first) {
    return [];
  }
  const count = Math.min(valid.length, 50);

  return Array.from(
    { length: count },
    (_, index) => valid[Math.floor((index * (valid.length - 1)) / (count - 1))] ?? first,
  );
}

/** The service refuses longer plan names, so a copied title is cut to fit. */
export const MAX_PLAN_NAME_LENGTH = 120;

/** An unsaved draft copied from another provider's route; null when it has no name or too short a line. */
export function plannerSeedFrom(title: string, coordinates: Position[]): PlannerSeed | null {
  const waypoints = samplePlanWaypoints(coordinates);
  const name = Array.from(title.trim()).slice(0, MAX_PLAN_NAME_LENGTH).join("");

  return waypoints.length < 2 || name === ""
    ? null
    : {
        name,
        profile: "trekking",
        waypoints,
      };
}

export function isPlannerSeed(value: unknown): value is PlannerSeed {
  if (!value || typeof value !== "object") {
    return false;
  }
  const candidate = value as Partial<PlannerSeed>;

  return (
    typeof candidate.name === "string" &&
    candidate.name.trim().length > 0 &&
    Array.from(candidate.name).length <= MAX_PLAN_NAME_LENGTH &&
    typeof candidate.profile === "string" &&
    PLAN_PROFILES.includes(candidate.profile as PlanProfile) &&
    Array.isArray(candidate.waypoints) &&
    candidate.waypoints.length >= 2 &&
    candidate.waypoints.length <= 50 &&
    candidate.waypoints.every(
      (waypoint) =>
        waypoint !== null &&
        typeof waypoint === "object" &&
        Number.isFinite(waypoint.longitude) &&
        Number.isFinite(waypoint.latitude) &&
        waypoint.longitude >= -180 &&
        waypoint.longitude <= 180 &&
        waypoint.latitude >= -90 &&
        waypoint.latitude <= 90,
    )
  );
}

export type PlannerAction =
  | { type: "setName"; name: string }
  | { type: "setProfile"; profile: PlanProfile }
  | { type: "setCues"; cues: boolean }
  | { type: "append"; waypoint: PlanWaypoint }
  | { type: "insert"; index: number; waypoint: PlanWaypoint }
  | { type: "move"; index: number; waypoint: PlanWaypoint }
  | { type: "snap"; id: number; from: PlanWaypoint; waypoint: PlanWaypoint }
  | { type: "delete"; index: number }
  | { type: "reverse" }
  | { type: "reorder"; index: number; direction: "up" | "down" }
  | { type: "reorder"; order: number[] }
  | { type: "setStraight"; id: number; straight: boolean }
  | { type: "addAvoid"; longitude: number; latitude: number }
  | { type: "moveAvoid"; id: number; longitude: number; latitude: number }
  | { type: "setAvoidRadius"; id: number; radiusMetres: number }
  | { type: "deleteAvoid"; id: number }
  | { type: "undo" }
  | { type: "redo" }
  | { type: "reset" }
  | {
      type: "load";
      plan: Pick<Plan, "name" | "profile" | "waypoints"> & { cues?: boolean; avoid?: PlanAvoid[] };
    };

/**
 * The same place, read within one turn of the globe. A click or a marker drag
 * on a wrapped copy of the world answers a longitude beyond ±180, which the
 * service refuses as out of range.
 */
export function unwrapped(waypoint: PlanWaypoint): PlanWaypoint {
  if (waypoint.longitude >= -180 && waypoint.longitude <= 180) {
    return waypoint;
  }
  const longitude = ((((waypoint.longitude + 180) % 360) + 360) % 360) - 180;

  return { ...waypoint, longitude };
}

/** The first waypoint can never be straight; every waypoint mutation runs through here. */
function normaliseWaypoints(waypoints: PlannerWaypoint[]): PlannerWaypoint[] {
  const first = waypoints[0];
  if (!first?.straight) {
    return waypoints;
  }
  const { straight: _straight, ...rest } = first;
  return [rest as PlannerWaypoint, ...waypoints.slice(1)];
}

function snapshot({
  name,
  profile,
  cues,
  waypoints,
  nextWaypointID,
  avoid,
  nextAvoidID,
}: PlannerState): PlannerSnapshot {
  return { name, profile, cues, waypoints, nextWaypointID, avoid, nextAvoidID };
}

function apply(state: PlannerState, next: PlannerSnapshot): PlannerState {
  return {
    ...next,
    waypoints: normaliseWaypoints(next.waypoints),
    past: [...state.past, snapshot(state)],
    future: [],
  };
}

export const initialPlannerState: PlannerState = {
  name: "",
  profile: "trekking",
  cues: false,
  waypoints: [],
  nextWaypointID: 0,
  avoid: [],
  nextAvoidID: 0,
  past: [],
  future: [],
};

export function plannerReducer(state: PlannerState, action: PlannerAction): PlannerState {
  switch (action.type) {
    case "setName":
      return action.name === state.name
        ? state
        : apply(state, { ...snapshot(state), name: action.name });
    case "setProfile":
      return action.profile === state.profile
        ? state
        : apply(state, { ...snapshot(state), profile: action.profile });
    case "setCues":
      return action.cues === state.cues
        ? state
        : apply(state, { ...snapshot(state), cues: action.cues });
    case "append":
      return state.waypoints.length === 50
        ? state
        : apply(state, {
            ...snapshot(state),
            waypoints: [
              ...state.waypoints,
              { ...unwrapped(action.waypoint), id: state.nextWaypointID },
            ],
            nextWaypointID: state.nextWaypointID + 1,
          });
    case "insert": {
      if (state.waypoints.length === 50) {
        return state;
      }
      const index = Math.max(0, Math.min(action.index, state.waypoints.length));

      return apply(state, {
        ...snapshot(state),
        waypoints: [
          ...state.waypoints.slice(0, index),
          { ...unwrapped(action.waypoint), id: state.nextWaypointID },
          ...state.waypoints.slice(index),
        ],
        nextWaypointID: state.nextWaypointID + 1,
      });
    }
    case "move": {
      if (!state.waypoints[action.index]) {
        return state;
      }
      const waypoints = [...state.waypoints];
      const current = waypoints[action.index];
      if (!current) {
        return state;
      }
      waypoints[action.index] = { ...current, ...unwrapped(action.waypoint), id: current.id };

      return apply(state, { ...snapshot(state), waypoints });
    }
    case "snap": {
      // Settles a waypoint just placed onto the road beside it. It belongs to the
      // placing, so it rewrites the present rather than adding a step to undo.
      // A reply for a waypoint moved, or an id reused, since it was asked is stale.
      const at = state.waypoints.findIndex(
        (waypoint) =>
          waypoint.id === action.id &&
          waypoint.longitude === action.from.longitude &&
          waypoint.latitude === action.from.latitude,
      );
      if (at < 0) {
        return state;
      }
      const waypoints = [...state.waypoints];
      waypoints[at] = { ...waypoints[at], ...unwrapped(action.waypoint), id: action.id };

      return { ...state, waypoints };
    }
    case "delete":
      return state.waypoints[action.index]
        ? apply(state, {
            ...snapshot(state),
            waypoints: state.waypoints.filter((_, index) => index !== action.index),
          })
        : state;
    case "reverse":
      return state.waypoints.length < 2
        ? state
        : apply(state, { ...snapshot(state), waypoints: [...state.waypoints].reverse() });
    case "reorder": {
      if ("order" in action) {
        if (
          action.order.length !== state.waypoints.length ||
          new Set(action.order).size !== state.waypoints.length
        ) {
          return state;
        }
        const byID = new Map(state.waypoints.map((waypoint) => [waypoint.id, waypoint]));
        const waypoints = action.order.map((id) => byID.get(id));
        if (waypoints.some((waypoint) => !waypoint)) {
          return state;
        }
        const ordered = waypoints as PlannerWaypoint[];
        return ordered.every((waypoint, index) => waypoint === state.waypoints[index])
          ? state
          : apply(state, { ...snapshot(state), waypoints: ordered });
      }
      const nextIndex = action.index + (action.direction === "up" ? -1 : 1);
      if (!state.waypoints[action.index] || !state.waypoints[nextIndex]) {
        return state;
      }
      const waypoints = [...state.waypoints];
      const [waypoint] = waypoints.splice(action.index, 1);
      if (!waypoint) {
        return state;
      }
      waypoints.splice(nextIndex, 0, waypoint);

      return apply(state, { ...snapshot(state), waypoints });
    }
    case "setStraight": {
      const index = state.waypoints.findIndex((waypoint) => waypoint.id === action.id);
      const current = state.waypoints[index];
      if (index <= 0 || !current || (current.straight ?? false) === action.straight) {
        return state;
      }
      const waypoints = [...state.waypoints];
      waypoints[index] = { ...current, straight: action.straight };

      return apply(state, { ...snapshot(state), waypoints });
    }
    case "addAvoid":
      return state.avoid.length === 20
        ? state
        : apply(state, {
            ...snapshot(state),
            avoid: [
              ...state.avoid,
              {
                id: state.nextAvoidID,
                longitude: action.longitude,
                latitude: action.latitude,
                radiusMetres: 250,
              },
            ],
            nextAvoidID: state.nextAvoidID + 1,
          });
    case "moveAvoid": {
      const index = state.avoid.findIndex((area) => area.id === action.id);
      const current = state.avoid[index];
      if (!current) {
        return state;
      }
      const avoid = [...state.avoid];
      avoid[index] = { ...current, longitude: action.longitude, latitude: action.latitude };

      return apply(state, { ...snapshot(state), avoid });
    }
    case "setAvoidRadius": {
      const index = state.avoid.findIndex((area) => area.id === action.id);
      const current = state.avoid[index];
      if (!current || current.radiusMetres === action.radiusMetres) {
        return state;
      }
      const avoid = [...state.avoid];
      avoid[index] = { ...current, radiusMetres: action.radiusMetres };

      return apply(state, { ...snapshot(state), avoid });
    }
    case "deleteAvoid":
      return state.avoid.some((area) => area.id === action.id)
        ? apply(state, {
            ...snapshot(state),
            avoid: state.avoid.filter((area) => area.id !== action.id),
          })
        : state;
    case "undo": {
      const previous = state.past.at(-1);
      if (!previous) {
        return state;
      }

      return {
        ...previous,
        past: state.past.slice(0, -1),
        future: [snapshot(state), ...state.future],
      };
    }
    case "redo": {
      const next = state.future[0];
      if (!next) {
        return state;
      }

      return { ...next, past: [...state.past, snapshot(state)], future: state.future.slice(1) };
    }
    case "reset":
      return initialPlannerState;
    case "load": {
      const avoid = (action.plan.avoid ?? []).map((area, id) => ({ ...area, id }));

      return {
        ...action.plan,
        // A copy seeded from a library route carries no switch of its own.
        cues: action.plan.cues ?? false,
        waypoints: normaliseWaypoints(
          action.plan.waypoints.map((waypoint, id) => ({ ...waypoint, id })),
        ),
        nextWaypointID: action.plan.waypoints.length,
        avoid,
        nextAvoidID: avoid.length,
        past: [],
        future: [],
      };
    }
  }
}
