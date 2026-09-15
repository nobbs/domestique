import type { Plan, PlanProfile, PlanWaypoint } from "../../api/types";

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

export type PlannerAction =
  | { type: "setName"; name: string }
  | { type: "setProfile"; profile: PlanProfile }
  | { type: "append"; waypoint: PlanWaypoint }
  | { type: "insert"; index: number; waypoint: PlanWaypoint }
  | { type: "move"; index: number; waypoint: PlanWaypoint }
  | { type: "delete"; index: number }
  | { type: "reverse" }
  | { type: "reorder"; index: number; direction: "up" | "down" }
  | { type: "reorder"; order: number[] }
  | { type: "undo" }
  | { type: "redo" }
  | { type: "reset" }
  | { type: "load"; plan: Pick<Plan, "name" | "profile" | "waypoints"> };

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
            waypoints: [...state.waypoints, { ...action.waypoint, id: state.nextWaypointID }],
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
          { ...action.waypoint, id: state.nextWaypointID },
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
      waypoints[action.index] = { ...action.waypoint, id: current.id };

      return apply(state, { ...snapshot(state), waypoints });
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
