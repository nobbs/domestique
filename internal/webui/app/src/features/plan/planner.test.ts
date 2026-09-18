import { describe, expect, it } from "vitest";
import type { Position } from "../../api/types";
import {
  initialPlannerState,
  isPlannerSeed,
  plannerReducer,
  plannerSeedFrom,
  samplePlanWaypoints,
} from "./planner";

const first = { longitude: 8, latitude: 49 };
const second = { longitude: 8.1, latitude: 49.1 };
const third = { longitude: 8.2, latitude: 49.2 };

function reduce(...actions: Parameters<typeof plannerReducer>[1][]) {
  return actions.reduce(plannerReducer, initialPlannerState);
}

describe("plannerReducer", () => {
  it("samples a valid source line deterministically within the waypoint cap", () => {
    const coordinates: Position[] = Array.from({ length: 52 }, (_, index) => [8 + index / 100, 49]);
    coordinates.splice(1, 0, [Number.NaN, 49]);

    const sampled = samplePlanWaypoints(coordinates);

    expect(sampled).toHaveLength(50);
    expect(sampled[0]).toEqual({ longitude: 8, latitude: 49 });
    expect(sampled.at(-1)).toEqual({ longitude: 8.51, latitude: 49 });
  });

  it("cuts a copied title to a name the planner accepts", () => {
    const seed = plannerSeedFrom("x".repeat(130), [
      [8, 49],
      [8.1, 49.1],
    ]);

    expect(seed?.name).toHaveLength(120);
    expect(seed && isPlannerSeed(seed)).toBe(true);
    expect(plannerSeedFrom("Short", [[8, 49]])).toBeNull();
    expect(
      plannerSeedFrom("  ", [
        [8, 49],
        [8.1, 49.1],
      ]),
    ).toBeNull();

    // Characters, as the service counts them: an emoji is one, not two.
    const astral = plannerSeedFrom("\u{1F6B4}".repeat(121), [
      [8, 49],
      [8.1, 49.1],
    ]);
    expect(Array.from(astral?.name ?? "")).toHaveLength(120);
    expect(astral?.name.endsWith("\u{1F6B4}")).toBe(true);
    expect(astral && isPlannerSeed(astral)).toBe(true);
  });

  it("rejects malformed copy seeds", () => {
    const seed = {
      name: "Copied loop",
      profile: "trekking",
      waypoints: [first, second],
    };

    expect(isPlannerSeed({ ...seed, name: "" })).toBe(false);
    expect(isPlannerSeed({ ...seed, name: " " })).toBe(false);
    expect(isPlannerSeed({ ...seed, name: "x".repeat(121) })).toBe(false);
    expect(isPlannerSeed({ ...seed, waypoints: [null, second] })).toBe(false);
  });

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

  it("commits a dragged waypoint order as one history entry", () => {
    const state = reduce(
      { type: "append", waypoint: first },
      { type: "append", waypoint: second },
      { type: "append", waypoint: third },
    );
    const reordered = plannerReducer(state, { type: "reorder", order: [1, 2, 0] });

    expect(reordered.waypoints).toMatchObject([second, third, first]);
    expect(reordered.past).toHaveLength(state.past.length + 1);
  });

  it("deletes one waypoint", () => {
    const state = reduce(
      { type: "append", waypoint: first },
      { type: "append", waypoint: second },
      { type: "delete", index: 0 },
    );

    expect(state.waypoints).toMatchObject([second]);
  });

  it("inserts a waypoint at the requested position", () => {
    const middle = { longitude: 8.05, latitude: 49.05 };
    const state = reduce(
      { type: "append", waypoint: first },
      { type: "append", waypoint: second },
      { type: "insert", index: 1, waypoint: middle },
    );

    expect(state.waypoints).toMatchObject([first, middle, second]);
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

  it("reads a waypoint placed on a wrapped copy of the world within one globe", () => {
    // A click east of the antimeridian on the next copy of the map: 211.2 is
    // the same meridian as -148.8, and the service refuses anything past 180.
    const wrapped = reduce(
      { type: "append", waypoint: { longitude: 211.2, latitude: 11.4 } },
      { type: "append", waypoint: { longitude: -400, latitude: 20 } },
    );

    expect(wrapped.waypoints[0]).toMatchObject({ longitude: -148.8, latitude: 11.4 });
    expect(wrapped.waypoints[1]).toMatchObject({ longitude: -40, latitude: 20 });
  });

  it("unwraps a waypoint dragged onto another copy of the world", () => {
    const dragged = reduce(
      { type: "append", waypoint: first },
      { type: "move", index: 0, waypoint: { longitude: 368, latitude: 49 } },
    );

    expect(dragged.waypoints[0]).toMatchObject({ longitude: 8, latitude: 49 });
  });

  it("leaves a waypoint already within the globe alone", () => {
    const placed = reduce({ type: "insert", index: 0, waypoint: { longitude: 180, latitude: 0 } });

    expect(placed.waypoints[0]).toMatchObject({ longitude: 180, latitude: 0 });
  });

  it("settles a placed waypoint onto its road without a step of its own to undo", () => {
    const placed = reduce(
      { type: "append", waypoint: first },
      { type: "append", waypoint: second },
    );
    const snapped = plannerReducer(placed, {
      type: "snap",
      id: 1,
      from: second,
      waypoint: { longitude: 8.1003, latitude: 49.1002 },
    });

    expect(snapped.waypoints[1]).toMatchObject({ id: 1, longitude: 8.1003, latitude: 49.1002 });
    expect(snapped.past).toBe(placed.past);
    // One undo takes back the placing and the snap together.
    expect(plannerReducer(snapped, { type: "undo" }).waypoints).toHaveLength(1);
  });

  it("ignores a snap for a waypoint that is already gone", () => {
    const placed = reduce({ type: "append", waypoint: first });
    const deleted = plannerReducer(placed, { type: "delete", index: 0 });

    expect(plannerReducer(deleted, { type: "snap", id: 0, from: first, waypoint: second })).toBe(
      deleted,
    );
  });

  it("ignores a snap for a waypoint moved since it was asked", () => {
    const placed = reduce({ type: "append", waypoint: first });
    const moved = plannerReducer(placed, { type: "move", index: 0, waypoint: second });

    expect(
      plannerReducer(moved, {
        type: "snap",
        id: 0,
        from: first,
        waypoint: { longitude: 8.0001, latitude: 49.0001 },
      }),
    ).toBe(moved);
  });

  it("ignores a snap whose waypoint id now names another place", () => {
    const placed = reduce({ type: "append", waypoint: first });
    const undone = plannerReducer(placed, { type: "undo" });
    const replaced = plannerReducer(undone, { type: "append", waypoint: second });

    expect(replaced.waypoints[0]?.id).toBe(0);
    expect(
      plannerReducer(replaced, {
        type: "snap",
        id: 0,
        from: first,
        waypoint: { longitude: 8.0001, latitude: 49.0001 },
      }),
    ).toBe(replaced);
  });

  it("resets an opened plan to a new draft", () => {
    const edited = reduce(
      { type: "setName", name: "Stored route" },
      { type: "append", waypoint: first },
    );

    expect(plannerReducer(edited, { type: "reset" })).toBe(initialPlannerState);
  });
});
