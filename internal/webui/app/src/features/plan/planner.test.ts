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

  it("sets, undoes, and redoes turn cues, but not when unchanged", () => {
    const unset = plannerReducer(initialPlannerState, { type: "setCues", cues: false });
    expect(unset).toBe(initialPlannerState);

    const set = plannerReducer(initialPlannerState, { type: "setCues", cues: true });
    expect(set.cues).toBe(true);
    const undone = plannerReducer(set, { type: "undo" });
    expect(undone.cues).toBe(false);
    expect(plannerReducer(undone, { type: "redo" }).cues).toBe(true);
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
      plan: { name: "Stored route", profile: "fastbike", cues: true, waypoints: [first, second] },
    });

    expect(loaded).toMatchObject({
      name: "Stored route",
      profile: "fastbike",
      cues: true,
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

  it("sets, undoes, and redoes straight legs, but never on the first waypoint", () => {
    const state = reduce({ type: "append", waypoint: first }, { type: "append", waypoint: second });

    const ignored = plannerReducer(state, {
      type: "setStraight",
      id: state.waypoints[0]?.id ?? -1,
      straight: true,
    });
    expect(ignored).toBe(state);

    const set = plannerReducer(state, {
      type: "setStraight",
      id: state.waypoints[1]?.id ?? -1,
      straight: true,
    });
    expect(set.waypoints[1]).toMatchObject({ straight: true });
    expect(
      plannerReducer(set, { type: "setStraight", id: set.waypoints[1]?.id ?? -1, straight: true }),
    ).toBe(set);

    const undone = plannerReducer(set, { type: "undo" });
    expect(undone.waypoints[1]?.straight).toBeUndefined();
    expect(plannerReducer(undone, { type: "redo" }).waypoints[1]).toMatchObject({ straight: true });
  });

  it("preserves a straight flag across a move and a settling snap", () => {
    const state = reduce({ type: "append", waypoint: first }, { type: "append", waypoint: second });
    const straightID = state.waypoints[1]?.id ?? -1;
    const set = plannerReducer(state, { type: "setStraight", id: straightID, straight: true });

    const moved = plannerReducer(set, { type: "move", index: 1, waypoint: third });
    expect(moved.waypoints[1]).toMatchObject({ straight: true, ...third });

    const snapped = plannerReducer(moved, {
      type: "snap",
      id: straightID,
      from: third,
      waypoint: { longitude: 8.2001, latitude: 49.2001 },
    });
    expect(snapped.waypoints[1]).toMatchObject({ straight: true });
  });

  it("clears the straight flag whenever a waypoint becomes first", () => {
    const straightSecond = reduce(
      { type: "append", waypoint: first },
      { type: "append", waypoint: second },
      { type: "append", waypoint: third },
    );
    const withStraight = plannerReducer(straightSecond, {
      type: "setStraight",
      id: straightSecond.waypoints[2]?.id ?? -1,
      straight: true,
    });

    // reverse: the straight leg second→third stays that stretch, now arriving at second.
    const reversed = plannerReducer(withStraight, { type: "reverse" });
    expect(reversed.waypoints.map((waypoint) => Boolean(waypoint.straight))).toEqual([
      false,
      true,
      false,
    ]);
    expect(reversed.waypoints[0]?.straight).toBeUndefined();

    // delete: removing the first waypoint promotes a straight one.
    const straightFirst = plannerReducer(withStraight, {
      type: "setStraight",
      id: withStraight.waypoints[1]?.id ?? -1,
      straight: true,
    });
    const deleted = plannerReducer(straightFirst, { type: "delete", index: 0 });
    expect(deleted.waypoints[0]?.straight).toBeUndefined();

    // reorder: dragging a straight waypoint to the front.
    const reordered = plannerReducer(withStraight, {
      type: "reorder",
      order: [withStraight.waypoints[2]?.id ?? -1, 0, 1],
    });
    expect(reordered.waypoints[0]?.straight).toBeUndefined();

    // insert at 0: the previous first waypoint (never straight) simply shifts.
    const inserted = plannerReducer(straightSecond, {
      type: "insert",
      index: 0,
      waypoint: { longitude: 7.9, latitude: 48.9 },
    });
    expect(inserted.waypoints[0]?.straight).toBeUndefined();
  });

  it("reads a straight waypoint from a stored plan", () => {
    const loaded = plannerReducer(initialPlannerState, {
      type: "load",
      plan: {
        name: "Stored route",
        profile: "trekking",
        waypoints: [first, { ...second, straight: true }],
      },
    });

    expect(loaded.waypoints[1]).toMatchObject({ straight: true });
  });

  it("adds, moves, resizes, and deletes avoided areas, up to a limit of 20, with undo", () => {
    const withOne = plannerReducer(initialPlannerState, {
      type: "addAvoid",
      longitude: 8,
      latitude: 49,
    });
    expect(withOne.avoid).toMatchObject([{ id: 0, longitude: 8, latitude: 49, radiusMetres: 250 }]);

    const moved = plannerReducer(withOne, {
      type: "moveAvoid",
      id: 0,
      longitude: 8.01,
      latitude: 49.01,
    });
    expect(moved.avoid[0]).toMatchObject({ longitude: 8.01, latitude: 49.01 });

    const resized = plannerReducer(moved, { type: "setAvoidRadius", id: 0, radiusMetres: 1000 });
    expect(resized.avoid[0]).toMatchObject({ radiusMetres: 1000 });
    expect(plannerReducer(resized, { type: "setAvoidRadius", id: 0, radiusMetres: 1000 })).toBe(
      resized,
    );

    const undone = plannerReducer(resized, { type: "undo" });
    expect(undone.avoid[0]).toMatchObject({ radiusMetres: 250 });

    const deleted = plannerReducer(resized, { type: "deleteAvoid", id: 0 });
    expect(deleted.avoid).toEqual([]);
    expect(plannerReducer(deleted, { type: "deleteAvoid", id: 0 })).toBe(deleted);

    let capped = initialPlannerState;
    for (let index = 0; index < 20; index++) {
      capped = plannerReducer(capped, { type: "addAvoid", longitude: 8, latitude: 49 });
    }
    expect(capped.avoid).toHaveLength(20);
    expect(plannerReducer(capped, { type: "addAvoid", longitude: 8, latitude: 49 })).toBe(capped);
  });

  it("loads a stored plan's avoided areas and clears them on reset", () => {
    const loaded = plannerReducer(initialPlannerState, {
      type: "load",
      plan: {
        name: "Stored route",
        profile: "trekking",
        waypoints: [first, second],
        avoid: [{ longitude: 8.05, latitude: 49.05, radiusMetres: 500 }],
      },
    });

    expect(loaded.avoid).toMatchObject([
      { id: 0, longitude: 8.05, latitude: 49.05, radiusMetres: 500 },
    ]);
    expect(plannerReducer(loaded, { type: "reset" }).avoid).toEqual([]);
  });

  it("resets an opened plan to a new draft", () => {
    const edited = reduce(
      { type: "setName", name: "Stored route" },
      { type: "append", waypoint: first },
      { type: "setCues", cues: true },
    );

    expect(plannerReducer(edited, { type: "reset" })).toBe(initialPlannerState);
    expect(initialPlannerState.cues).toBe(false);
  });

  describe("insertMany", () => {
    // A route running due north, with places beside it at known distances along.
    const start = { longitude: 8, latitude: 49 };
    const finish = { longitude: 8, latitude: 49.3 };
    const early = { longitude: 8.01, latitude: 49.05 };
    const middle = { longitude: 7.99, latitude: 49.15 };
    const late = { longitude: 8.01, latitude: 49.25 };
    const beyond = { longitude: 8, latitude: 49.4 };
    const route = reduce({ type: "append", waypoint: start }, { type: "append", waypoint: finish });
    const positions = (state: typeof route) =>
      state.waypoints.map(({ longitude, latitude }) => ({ longitude, latitude }));

    it("orders places along the route whatever order they were chosen in", () => {
      const chosen = [late, beyond, early, middle];
      const reversed = [...chosen].reverse();

      const placed = plannerReducer(route, { type: "insertMany", waypoints: chosen });

      expect(positions(placed)).toEqual([start, early, middle, late, finish, beyond]);
      expect(positions(plannerReducer(route, { type: "insertMany", waypoints: reversed }))).toEqual(
        positions(placed),
      );
    });

    it("places each onto the leg it is nearest, as the route stood", () => {
      const bent = reduce(
        { type: "append", waypoint: start },
        { type: "append", waypoint: { longitude: 8.3, latitude: 49 } },
        { type: "append", waypoint: { longitude: 8.3, latitude: 49.3 } },
      );
      const northLeg = { longitude: 8.31, latitude: 49.2 };
      const eastLeg = { longitude: 8.1, latitude: 48.99 };

      const placed = plannerReducer(bent, { type: "insertMany", waypoints: [northLeg, eastLeg] });

      expect(positions(placed)).toEqual([
        start,
        eastLeg,
        { longitude: 8.3, latitude: 49 },
        northLeg,
        { longitude: 8.3, latitude: 49.3 },
      ]);
    });

    it("follows the chosen order where there is no route yet, and is one step to undo", () => {
      const placed = reduce({ type: "insertMany", waypoints: [late, early, middle] });

      expect(positions(placed)).toEqual([late, early, middle]);
      expect(placed.waypoints.map((waypoint) => waypoint.id)).toEqual([0, 1, 2]);
      expect(placed.nextWaypointID).toBe(3);
      expect(plannerReducer(placed, { type: "undo" }).waypoints).toEqual([]);
    });

    it("gives every new waypoint its own id and keeps the old ones", () => {
      const placed = plannerReducer(route, { type: "insertMany", waypoints: [late, early] });

      expect(placed.waypoints.map((waypoint) => waypoint.id)).toEqual([0, 3, 2, 1]);
    });

    it("adds no more than the waypoint cap leaves room for", () => {
      const many = Array.from({ length: 60 }, (_, index) => ({
        longitude: 8,
        latitude: 49 + index / 1000,
      }));

      const placed = reduce({ type: "insertMany", waypoints: many });

      expect(placed.waypoints).toHaveLength(50);
      expect(plannerReducer(placed, { type: "insertMany", waypoints: [early] })).toBe(placed);
      expect(plannerReducer(route, { type: "insertMany", waypoints: [] })).toBe(route);
    });

    it("orders places beside a leg that starts and ends in one place", () => {
      const loop = reduce({ type: "append", waypoint: start }, { type: "append", waypoint: start });

      const placed = plannerReducer(loop, { type: "insertMany", waypoints: [early, late] });

      expect(positions(placed)).toEqual([start, early, late, start]);
    });

    it("unwraps a longitude from a copy of the world", () => {
      const placed = reduce({ type: "insertMany", waypoints: [{ longitude: 368, latitude: 49 }] });

      expect(placed.waypoints[0]?.longitude).toBe(8);
    });
  });
});
