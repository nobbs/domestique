import { describe, expect, it } from "vitest";
import { initialPlannerState, plannerReducer } from "./planner";

const first = { longitude: 8, latitude: 49 };
const second = { longitude: 8.1, latitude: 49.1 };

function reduce(...actions: Parameters<typeof plannerReducer>[1][]) {
  return actions.reduce(plannerReducer, initialPlannerState);
}

describe("plannerReducer", () => {
  it("records name, profile, and waypoint edits in one history", () => {
    const state = reduce(
      { type: "setName", name: "Morning loop" },
      { type: "setProfile", profile: "gravel" },
      { type: "append", waypoint: first },
      { type: "move", index: 0, waypoint: second },
    );

    expect(state).toMatchObject({ name: "Morning loop", profile: "gravel", waypoints: [second] });
    expect(state.past).toHaveLength(4);
    expect(state.future).toEqual([]);
  });

  it("reverses and reorders waypoints", () => {
    const state = reduce(
      { type: "append", waypoint: first },
      { type: "append", waypoint: second },
      { type: "reverse" },
      { type: "reorder", index: 1, direction: "up" },
    );

    expect(state.waypoints).toMatchObject([first, second]);
  });

  it("deletes one waypoint", () => {
    const state = reduce(
      { type: "append", waypoint: first },
      { type: "append", waypoint: second },
      { type: "delete", index: 0 },
    );

    expect(state.waypoints).toMatchObject([second]);
  });

  it("does not record no-op waypoint actions", () => {
    const state = reduce(
      { type: "move", index: 0, waypoint: first },
      { type: "reverse" },
      { type: "reorder", index: 0, direction: "up" },
    );

    expect(state).toBe(initialPlannerState);
  });

  it("undoes and redoes only within history bounds", () => {
    const changed = reduce({ type: "append", waypoint: first }, { type: "undo" });
    const redone = plannerReducer(changed, { type: "redo" });

    expect(changed.waypoints).toEqual([]);
    expect(plannerReducer(changed, { type: "undo" })).toBe(changed);
    expect(redone.waypoints).toMatchObject([first]);
    expect(plannerReducer(redone, { type: "redo" })).toBe(redone);
  });

  it("loads a stored plan without retaining a prior edit history", () => {
    const edited = reduce({ type: "append", waypoint: first });
    const loaded = plannerReducer(edited, {
      type: "load",
      plan: { name: "Stored route", profile: "fastbike", waypoints: [first, second] },
    });

    expect(loaded).toMatchObject({
      name: "Stored route",
      profile: "fastbike",
      waypoints: [first, second],
    });
    expect(loaded.past).toEqual([]);
    expect(loaded.future).toEqual([]);
  });

  it("resets an opened plan to a new draft", () => {
    const edited = reduce(
      { type: "setName", name: "Stored route" },
      { type: "append", waypoint: first },
    );

    expect(plannerReducer(edited, { type: "reset" })).toBe(initialPlannerState);
  });
});
