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
  waypoints: PlannerWaypoint[];
  nextWaypointID: number;
  past: PlannerSnapshot[];
  future: PlannerSnapshot[];
}

interface PlannerSnapshot {
  name: string;
  profile: PlanProfile;
  waypoints: PlannerWaypoint[];
  nextWaypointID: number;
}

export interface PlannerWaypoint extends PlanWaypoint {
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
  | { type: "append"; waypoint: PlanWaypoint }
  | { type: "insert"; index: number; waypoint: PlanWaypoint }
  | { type: "move"; index: number; waypoint: PlanWaypoint }
  | { type: "snap"; id: number; waypoint: PlanWaypoint }
  | { type: "delete"; index: number }
  | { type: "reverse" }
  | { type: "reorder"; index: number; direction: "up" | "down" }
  | { type: "reorder"; order: number[] }
  | { type: "undo" }
  | { type: "redo" }
  | { type: "reset" }
  | { type: "load"; plan: Pick<Plan, "name" | "profile" | "waypoints"> };

/**
 * The same place, read within one turn of the globe. A click or a marker drag
 * on a wrapped copy of the world answers a longitude beyond ±180, which the
 * service refuses as out of range.
 */
function unwrapped(waypoint: PlanWaypoint): PlanWaypoint {
  if (waypoint.longitude >= -180 && waypoint.longitude <= 180) {
    return waypoint;
  }
  const longitude = ((((waypoint.longitude + 180) % 360) + 360) % 360) - 180;

  return { ...waypoint, longitude };
}

function snapshot({ name, profile, waypoints, nextWaypointID }: PlannerState): PlannerSnapshot {
  return { name, profile, waypoints, nextWaypointID };
}

function apply(state: PlannerState, next: PlannerSnapshot): PlannerState {
  return { ...next, past: [...state.past, snapshot(state)], future: [] };
}

export const initialPlannerState: PlannerState = {
  name: "",
  profile: "trekking",
  waypoints: [],
  nextWaypointID: 0,
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
      waypoints[action.index] = { ...unwrapped(action.waypoint), id: current.id };

      return apply(state, { ...snapshot(state), waypoints });
    }
    case "snap": {
      // Settles a waypoint just placed onto the road beside it. It belongs to the
      // placing, so it rewrites the present rather than adding a step to undo.
      const at = state.waypoints.findIndex((waypoint) => waypoint.id === action.id);
      if (at < 0) {
        return state;
      }
      const waypoints = [...state.waypoints];
      waypoints[at] = { ...unwrapped(action.waypoint), id: action.id };

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
    case "load":
      return {
        ...action.plan,
        waypoints: action.plan.waypoints.map((waypoint, id) => ({ ...waypoint, id })),
        nextWaypointID: action.plan.waypoints.length,
        past: [],
        future: [],
      };
  }
}
