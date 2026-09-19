import { describe, expect, it } from "vitest";
import type { Position } from "../../api/types";
import {
  deviationStretches,
  initialPlannerState,
  isPlannerSeed,
  MAX_PLAN_WAYPOINTS,
  nextTraceStep,
  plannerReducer,
  plannerSeedFrom,
  rideTraceCoordinates,
  startTrace,
} from "./planner";

const first = { longitude: 8, latitude: 49 };
const second = { longitude: 8.1, latitude: 49.1 };
const third = { longitude: 8.2, latitude: 49.2 };

function seedOf(waypoints: number) {
  return {
    name: "Alpine loop",
    profile: "trekking",
    waypoints: Array.from({ length: waypoints }, (_, index) => ({
      longitude: 8 + index / 1000,
      latitude: 49,
    })),
  };
}

// An L: about 2.9 km east, then 4.4 km north, turning at vertex 40.
const corner: Position[] = [
  ...Array.from({ length: 40 }, (_, index): Position => [8 + index / 1000, 49]),
  ...Array.from({ length: 41 }, (_, index): Position => [8.04, 49 + index / 1000]),
];
const last = corner.length - 1;

function reduce(...actions: Parameters<typeof plannerReducer>[1][]) {
  return actions.reduce(plannerReducer, initialPlannerState);
}

describe("plannerReducer", () => {
  it("seeds a copy with the route's ends and turns, remembering where each sits on it", () => {
    const wiggled: Position[] = corner.map(([longitude, latitude], index) =>
      index === 20 ? [longitude, latitude + 0.0001] : [longitude, latitude],
    );
    wiggled.splice(1, 0, [Number.NaN, 49]);

    const seed = plannerSeedFrom("Corner", wiggled);

    expect(seed?.waypoints).toEqual([
      { longitude: 8, latitude: 49 },
      { longitude: 8.04, latitude: 49 },
      { longitude: 8.04, latitude: 49.04 },
    ]);
    expect(seed?.trace?.indices).toEqual([0, 40, last]);
    expect(seed?.trace?.route).toHaveLength(corner.length);
    expect(seed && isPlannerSeed(seed)).toBe(true);
    expect(seed && isPlannerSeed({ ...seed, trace: { ...seed.trace, indices: [0, last] } })).toBe(
      false,
    );
    expect(
      seed && isPlannerSeed({ ...seed, trace: { ...seed.trace, indices: [40, 0, last] } }),
    ).toBe(false);
  });

  it("adds a waypoint where the routed leg strays, then proves it necessary", () => {
    const chord = [corner[0] ?? [8, 49], corner[last] ?? [8.04, 49.04]];
    const alongRoute = [corner.slice(0, 41), corner.slice(40)];

    const added = nextTraceStep(startTrace({ route: corner, indices: [0, last] }), [chord]);
    expect(added).toMatchObject({ phase: "add", indices: [0, 40, last], rounds: 1 });

    const pruning = added && nextTraceStep(added, alongRoute);
    expect(pruning).toMatchObject({ phase: "prune", indices: [0, last], removed: [40] });

    const restored = pruning && nextTraceStep(pruning, [chord]);
    expect(restored).toMatchObject({ indices: [0, 40, last], settled: [40], removed: [] });

    expect(restored && nextTraceStep(restored, alongRoute)).toMatchObject({
      phase: "done",
      indices: [0, 40, last],
      incomplete: false,
    });
  });

  it("adds a waypoint where a routed leg wanders off the route and back", () => {
    const straight = corner.slice(0, 41);
    const spur: Position[] = [...straight.slice(0, 21), [8.02, 49.005], ...straight.slice(20)];

    const added = nextTraceStep(startTrace({ route: straight, indices: [0, 40] }), [spur]);

    expect(added).toMatchObject({ phase: "add", indices: [0, 20, 40] });
  });

  it("judges a leg by its course, not by a stray point at either end", () => {
    const straight = corner.slice(0, 41);
    // Cut a vertex past each waypoint, about 73 m out: the leg still follows the route.
    const leg: Position[] = [[7.999, 49], ...straight, [8.041, 49]];

    expect(nextTraceStep(startTrace({ route: straight, indices: [0, 40] }), [leg])).toMatchObject({
      phase: "done",
      incomplete: false,
    });
  });

  it("ends a trace the round limit stopped as incomplete", () => {
    const chord = [corner[0] ?? [8, 49], corner[last] ?? [8.04, 49.04]];
    const exhausted = { ...startTrace({ route: corner, indices: [0, last] }), rounds: 12 };

    expect(nextTraceStep(exhausted, [chord])).toMatchObject({
      phase: "done",
      indices: [0, last],
      incomplete: true,
    });
  });

  it("finds a leg's straying the same whichever way round it was routed", () => {
    const straight = corner.slice(0, 41);
    const spur: Position[] = [...straight.slice(0, 21), [8.02, 49.005], ...straight.slice(20)];

    const forward = nextTraceStep(startTrace({ route: straight, indices: [0, 40] }), [spur]);
    const backward = nextTraceStep(startTrace({ route: straight, indices: [0, 40] }), [
      [...spur].reverse(),
    ]);

    expect(backward.indices).toEqual(forward.indices);
  });

  it("prunes every other waypoint a leg does without, and keeps the removal", () => {
    const straight = corner.slice(0, 41);
    const progress = {
      ...startTrace({ route: straight, indices: [0, 10, 20, 30, 40] }),
      rounds: 1,
    };

    const pruning = nextTraceStep(
      progress,
      [0, 10, 20, 30].map((at) => straight.slice(at, at + 11)),
    );
    expect(pruning).toMatchObject({ phase: "prune", indices: [0, 20, 40], removed: [10, 30] });

    const next = pruning && nextTraceStep(pruning, [straight.slice(0, 21), straight.slice(20)]);
    expect(next).toMatchObject({ indices: [0, 40], removed: [20], settled: [] });
  });

  it("traces waypoints in place, keeping their ids and leaving nothing to undo", () => {
    const state = reduce({
      type: "load",
      plan: { name: "Trace", profile: "trekking", waypoints: [first, third] },
    });

    const traced = plannerReducer(state, {
      type: "trace",
      waypoints: [{ ...first, id: 0 }, second, { ...third, id: 1 }],
    });

    expect(traced.waypoints.map((waypoint) => waypoint.id)).toEqual([0, 2, 1]);
    expect(traced.nextWaypointID).toBe(3);
    expect(traced.past).toEqual([]);
    expect(plannerReducer(state, { type: "trace", waypoints: [first] })).toBe(state);
  });

  it("stops accepting waypoints at the cap", () => {
    const filled = Array.from({ length: MAX_PLAN_WAYPOINTS }, () => ({
      type: "append" as const,
      waypoint: first,
    })).reduce(plannerReducer, initialPlannerState);

    expect(filled.waypoints).toHaveLength(MAX_PLAN_WAYPOINTS);
    expect(plannerReducer(filled, { type: "append", waypoint: second })).toBe(filled);
    expect(plannerReducer(filled, { type: "insert", index: 0, waypoint: second })).toBe(filled);
    expect(isPlannerSeed(seedOf(MAX_PLAN_WAYPOINTS))).toBe(true);
    expect(isPlannerSeed(seedOf(MAX_PLAN_WAYPOINTS + 1))).toBe(false);
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
      const many = Array.from({ length: MAX_PLAN_WAYPOINTS + 10 }, (_, index) => ({
        longitude: 8,
        latitude: 49 + index / 1000,
      }));

      const placed = reduce({ type: "insertMany", waypoints: many });

      expect(placed.waypoints).toHaveLength(MAX_PLAN_WAYPOINTS);
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

describe("deviationStretches", () => {
  // Nine vertices along a parallel; the detoured plan swings three of them about 1.1 km north.
  const route: Position[] = Array.from({ length: 9 }, (_, index) => [8 + index * 0.01, 49]);
  const detoured: Position[] = route.map(([longitude, latitude], index) =>
    index >= 3 && index <= 5 ? [longitude, latitude + 0.01] : [longitude, latitude],
  );

  it("finds the stretch the plan strays from, padded by one vertex either side", () => {
    expect(deviationStretches(route, detoured)).toEqual([route.slice(2, 7)]);
  });

  it("finds nothing on a plan that rides the route backwards or starts partway round", () => {
    const loop: Position[] = [...route, [8.08, 49.01], [8, 49.01], [8, 49]];

    expect(deviationStretches(route, [...route].reverse())).toEqual([]);
    expect(deviationStretches(loop, [...loop.slice(4), ...loop.slice(1, 5)])).toEqual([]);
  });

  it("finds the same stretch on a plan long enough to skip runs of it", () => {
    const long: Position[] = Array.from({ length: 300 }, (_, index) => [8 + index * 0.001, 49]);
    const detour: Position[] = long.map(([longitude, latitude], index) =>
      index >= 150 && index <= 160 ? [longitude, latitude + 0.01] : [longitude, latitude],
    );

    expect(deviationStretches(long, detour)).toEqual([long.slice(149, 162)]);
  });

  it("finds nothing where the plan follows the route, or against too short a plan", () => {
    expect(deviationStretches(route, route)).toEqual([]);
    expect(deviationStretches(route, [])).toEqual([]);
  });
});

describe("rideTraceCoordinates", () => {
  const metresPerDegree = (Math.PI / 180) * 6_371_000;
  // A straight south-north line at the equator, where a degree of longitude
  // and of latitude are both worth one metresPerDegree: no cos(latitude)
  // scaling to account for.
  const at = (northMetres: number): Position => [8, northMetres / metresPerDegree];

  it("drops an isolated out-and-back spike", () => {
    const track: Position[] = [
      at(0),
      at(50),
      at(100),
      [8.02, at(100)[1]], // ~2.2 km east, sandwiched between two samples back at 100 m
      at(100),
      at(150),
      at(200),
    ];

    const cleaned = rideTraceCoordinates(track);

    expect(cleaned.some(([longitude]) => longitude === 8.02)).toBe(false);
    expect(cleaned[0]).toEqual(track[0]);
    expect(cleaned.at(-1)).toEqual(track.at(-1));
  });

  it("thins a dense line to at least 25 m spacing", () => {
    const dense: Position[] = Array.from({ length: 20 }, (_, index) => at(index * 5));

    const cleaned = rideTraceCoordinates(dense);

    expect(cleaned.length).toBeLessThan(dense.length);
    expect(cleaned[0]).toEqual(dense[0]);
    expect(cleaned.at(-1)).toEqual(dense.at(-1));
    for (let index = 1; index < cleaned.length - 1; index++) {
      const gap = ((cleaned[index]?.[1] ?? 0) - (cleaned[index - 1]?.[1] ?? 0)) * metresPerDegree;
      expect(gap).toBeGreaterThanOrEqual(25);
    }
  });

  it("collapses a stationary cluster to one point", () => {
    const cluster: Position[] = [at(0), at(100), at(102), at(99), at(101), at(200)];

    const cleaned = rideTraceCoordinates(cluster);

    expect(cleaned).toEqual([at(0), at(100), at(200)]);
  });

  it("always keeps the first and last valid points, however close the last is to what came before", () => {
    const track: Position[] = [at(0), at(50), at(100), at(150), at(153)];

    const cleaned = rideTraceCoordinates(track);

    expect(cleaned[0]).toEqual(at(0));
    expect(cleaned.at(-1)).toEqual(at(153));
  });

  it("drops invalid positions before cleaning", () => {
    const track: Position[] = [at(0), [Number.NaN, 0], at(100), [200, 49], at(200)];

    const cleaned = rideTraceCoordinates(track);

    expect(cleaned).toEqual([at(0), at(100), at(200)]);
  });

  it("leaves a clean 30 m-spaced line untouched", () => {
    const evenly: Position[] = Array.from({ length: 6 }, (_, index) => at(index * 30));

    expect(rideTraceCoordinates(evenly)).toEqual(evenly);
  });
});
