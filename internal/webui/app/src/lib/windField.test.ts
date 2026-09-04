/**
 * The maths behind the drifting field, asked without a map.
 *
 * The question underneath most of these is the meteorological convention: a
 * forecast's direction is where the wind comes *from*, and a field that reads it
 * as where the air is going draws every streak backwards — a mistake nothing on
 * a moving map would make obvious. So the headline case is spelled out on the
 * ground rather than in the corridor's own coordinates: a wind from the north
 * has to leave the particle further south than it found it.
 */

import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import type { WindSample } from "./conditionsField";
import { corridorRadii } from "./conditionsField";
import { cumulativeMetres } from "./profile";
import type { FieldGeometry, FieldParticle } from "./windField";
import {
  advanceField,
  DRIFT_TIME_SCALE,
  driftRates,
  edgeMetresOf,
  FLOATS_PER_VERTEX,
  fieldSize,
  flowBearingDegrees,
  isSpent,
  lifeAlpha,
  MAX_PARTICLES,
  MAX_STATIC_ARROWS,
  mercatorXY,
  PARTICLES_PER_KILOMETRE,
  positionAt,
  respawn,
  routeBearingAt,
  seedField,
  segmentAt,
  staticFlow,
  streakAlpha,
  VERTICES_PER_STREAK,
  writeStreaks,
} from "./windField";

/** A due-east road of about 29 km at 49° N, a point every kilometre or so. */
const EAST: Position[] = Array.from({ length: 41 }, (_, index): Position => [8 + index * 0.01, 49]);
const EAST_DISTANCES = cumulativeMetres(EAST);
const EAST_METRES = EAST_DISTANCES[EAST_DISTANCES.length - 1] ?? 0;

/** The same length of road heading due north instead. */
const NORTH: Position[] = Array.from(
  { length: 41 },
  (_, index): Position => [8, 49 + index * 0.0065],
);
const NORTH_DISTANCES = cumulativeMetres(NORTH);
const NORTH_METRES = NORTH_DISTANCES[NORTH_DISTANCES.length - 1] ?? 0;

/** ICON-D2's cell, so the corridor is 1500 m of core and 4000 m of fade. */
const METRES_PER_CELL = 2000;
const EDGE_METRES = corridorRadii(METRES_PER_CELL).edgeMetres;

/** A wind of one strength and one direction the whole way along a road. */
function steady(directionDegrees: number, speedKmh: number, totalMetres: number): WindSample[] {
  return [
    { distanceMetres: 0, speedKmh, directionDegrees },
    { distanceMetres: totalMetres, speedKmh, directionDegrees },
  ];
}

function eastRoad(directionDegrees: number, speedKmh = 20): FieldGeometry {
  return {
    coordinates: EAST,
    distances: EAST_DISTANCES,
    samples: steady(directionDegrees, speedKmh, EAST_METRES),
    metresPerCell: METRES_PER_CELL,
    totalMetres: EAST_METRES,
  };
}

function northRoad(directionDegrees: number, speedKmh = 20): FieldGeometry {
  return {
    coordinates: NORTH,
    distances: NORTH_DISTANCES,
    samples: steady(directionDegrees, speedKmh, NORTH_METRES),
    metresPerCell: METRES_PER_CELL,
    totalMetres: NORTH_METRES,
  };
}

function particle(overrides: Partial<FieldParticle> = {}): FieldParticle {
  return {
    alongMetres: EAST_METRES / 2,
    offsetMetres: 0,
    ageSeconds: 0,
    lifeSeconds: 1000,
    alongMetresPerSecond: 0,
    acrossMetresPerSecond: 0,
    ...overrides,
  };
}

/**
 * A repeatable stand-in for `Math.random`, never returning exactly nought or
 * one: a fixed seed is what makes a field of a few hundred drifting particles
 * something a test can assert about at all.
 */
function sequence(seed: number): () => number {
  let state = seed;

  return () => {
    state = (state * 1_103_515_245 + 12_345) % 2_147_483_648;

    return (state + 0.5) / 2_147_483_648;
  };
}

describe("the direction the air is going", () => {
  it("turns the direction a wind comes from into the way it travels", () => {
    expect(flowBearingDegrees(0)).toBe(180);
    expect(flowBearingDegrees(270)).toBe(90);
    expect(flowBearingDegrees(350)).toBe(170);
  });

  it("stays a compass bearing for a direction given below nought", () => {
    expect(flowBearingDegrees(-10)).toBe(170);
  });
});

describe("a wind resolved into the corridor's own directions", () => {
  it("carries a particle across a road the wind blows at right angles to", () => {
    const rates = driftRates({ speedKmh: 36, directionDegrees: 0 }, 90);

    // 36 km/h is 10 m/s, and the field is played at DRIFT_TIME_SCALE.
    expect(rates.acrossMetresPerSecond).toBeCloseTo(10 * DRIFT_TIME_SCALE, 6);
    expect(rates.alongMetresPerSecond).toBeCloseTo(0, 6);
  });

  it("carries it back toward the start on a road the wind blows along", () => {
    const rates = driftRates({ speedKmh: 36, directionDegrees: 0 }, 0);

    expect(rates.alongMetresPerSecond).toBeCloseTo(-10 * DRIFT_TIME_SCALE, 6);
    expect(rates.acrossMetresPerSecond).toBeCloseTo(0, 6);
  });

  it("carries nothing at all when the wind has dropped", () => {
    const rates = driftRates({ speedKmh: 0, directionDegrees: 210 }, 45);

    expect(rates.alongMetresPerSecond).toBeCloseTo(0, 12);
    expect(rates.acrossMetresPerSecond).toBeCloseTo(0, 12);
  });
});

describe("a wind from the north", () => {
  it("drives a particle southward over the ground", () => {
    const geometry = eastRoad(0);
    const drifting = particle();
    const before = positionAt(EAST, EAST_DISTANCES, drifting.alongMetres, drifting.offsetMetres);

    advanceField([drifting], geometry, 1);
    const after = positionAt(EAST, EAST_DISTANCES, drifting.alongMetres, drifting.offsetMetres);

    expect(after?.[1] ?? 0).toBeLessThan(before?.[1] ?? 0);
    expect(after?.[0] ?? 0).toBeCloseTo(before?.[0] ?? 0, 6);
  });

  it("drives it southward down a north-heading road as well, back toward the start", () => {
    const geometry = northRoad(0);
    const drifting = particle({ alongMetres: NORTH_METRES / 2 });
    const before = positionAt(NORTH, NORTH_DISTANCES, drifting.alongMetres, drifting.offsetMetres);

    advanceField([drifting], geometry, 1);
    const after = positionAt(NORTH, NORTH_DISTANCES, drifting.alongMetres, drifting.offsetMetres);

    expect(drifting.alongMetres).toBeLessThan(NORTH_METRES / 2);
    expect(after?.[1] ?? 0).toBeLessThan(before?.[1] ?? 0);
  });

  it("drives it northward when the wind is from the south instead", () => {
    const geometry = eastRoad(180);
    const drifting = particle();

    advanceField([drifting], geometry, 1);

    expect(drifting.offsetMetres).toBeLessThan(0);
  });
});

describe("finding the way back to the ground", () => {
  it("bisects to the segment a distance falls on", () => {
    const distances = [0, 100, 200, 300];

    expect(segmentAt(distances, 0)).toBe(0);
    expect(segmentAt(distances, 100)).toBe(1);
    expect(segmentAt(distances, 150)).toBe(1);
    expect(segmentAt(distances, 299)).toBe(2);
  });

  it("clamps to the last segment past the end of the route", () => {
    expect(segmentAt([0, 100, 200, 300], 5000)).toBe(2);
  });

  it("reads the way the road points at a distance along it", () => {
    expect(routeBearingAt(EAST, EAST_DISTANCES, EAST_METRES / 2)).toBeCloseTo(90, 1);
    expect(routeBearingAt(NORTH, NORTH_DISTANCES, NORTH_METRES / 2)).toBeCloseTo(0, 1);
  });

  it("steps a positive offset out to the right of the way the road is ridden", () => {
    const on = positionAt(EAST, EAST_DISTANCES, EAST_METRES / 2, 0);
    const beside = positionAt(EAST, EAST_DISTANCES, EAST_METRES / 2, 1000);

    // Not to the last decimal: the segment's great-circle bearing is a hair off
    // due east at this latitude, which leans the normal a few centimetres east.
    expect(beside?.[0] ?? 0).toBeCloseTo(on?.[0] ?? 0, 5);
    // A thousand metres south of a road running east, in degrees of latitude.
    expect((on?.[1] ?? 0) - (beside?.[1] ?? 0)).toBeCloseTo(1000 / 111_195, 4);
  });

  it("has nowhere to put a particle without a route to put it on", () => {
    expect(positionAt([], [], 0, 0)).toBeNull();
    expect(positionAt(EAST, [0, 1], 0, 0)).toBeNull();
  });

  it("puts the world square where MapLibre's matrix expects it", () => {
    expect(mercatorXY([0, 0])).toEqual([0.5, 0.5]);
    const [x, y] = mercatorXY([90, 45]);

    expect(x).toBeCloseTo(0.75, 6);
    expect(y).toBeCloseTo(0.359725, 5);
  });
});

describe("how many streaks a route is seeded with", () => {
  it("scales with the length of the ride", () => {
    expect(fieldSize(10_000)).toBe(10 * PARTICLES_PER_KILOMETRE);
  });

  it("stops at the cap rather than growing with a very long stage", () => {
    expect(fieldSize(1_000_000)).toBe(MAX_PARTICLES);
  });

  it("seeds nothing for a route with no length to seed along", () => {
    expect(fieldSize(0)).toBe(0);
    expect(fieldSize(Number.NaN)).toBe(0);
  });

  it("seeds every particle inside the corridor and part way through a life", () => {
    const geometry = eastRoad(0);
    const particles = seedField(geometry, 50, sequence(7));

    expect(particles).toHaveLength(50);
    for (const seeded of particles) {
      expect(Math.abs(seeded.offsetMetres)).toBeLessThan(EDGE_METRES);
      expect(seeded.alongMetres).toBeGreaterThanOrEqual(0);
      expect(seeded.alongMetres).toBeLessThanOrEqual(EAST_METRES);
      expect(isSpent(seeded, geometry)).toBe(false);
    }
    // Not all at the same age, or the whole field would fade in and out together.
    expect(new Set(particles.map((seeded) => seeded.ageSeconds)).size).toBeGreaterThan(1);
  });
});

describe("a particle that runs out of corridor", () => {
  it("is spent at the edge, past the finish and before the start", () => {
    const geometry = eastRoad(0);

    expect(isSpent(particle({ offsetMetres: EDGE_METRES }), geometry)).toBe(true);
    expect(isSpent(particle({ offsetMetres: -EDGE_METRES }), geometry)).toBe(true);
    expect(isSpent(particle({ alongMetres: -1 }), geometry)).toBe(true);
    expect(isSpent(particle({ alongMetres: EAST_METRES + 1 }), geometry)).toBe(true);
    expect(isSpent(particle({ ageSeconds: 5, lifeSeconds: 4 }), geometry)).toBe(true);
  });

  it("is put back somewhere the corridor still speaks for, never on its edge", () => {
    const geometry = eastRoad(0);
    const random = sequence(3);
    for (let attempt = 0; attempt < 200; attempt++) {
      const spent = particle({ offsetMetres: EDGE_METRES * 2 });
      respawn(spent, geometry, random);

      expect(isSpent(spent, geometry)).toBe(false);
    }
  });

  it("respawns rather than escaping, however long the wind blows across it", () => {
    const geometry = eastRoad(0, 60);
    const particles = seedField(geometry, 40, sequence(11));
    const started = particles.map((each) => `${each.alongMetres}:${each.offsetMetres}`);
    for (let frame = 0; frame < 400; frame++) {
      advanceField(particles, geometry, 0.05, sequence(frame + 1));
    }

    for (const drifting of particles) {
      expect(Math.abs(drifting.offsetMetres)).toBeLessThan(EDGE_METRES);
      expect(drifting.alongMetres).toBeGreaterThanOrEqual(0);
      expect(drifting.alongMetres).toBeLessThanOrEqual(EAST_METRES);
    }
    // A steady 60 km/h crosswind takes every one of them out of a 4 km corridor
    // long before the four hundredth frame, so all of them are somewhere else.
    expect(particles.map((each) => `${each.alongMetres}:${each.offsetMetres}`)).not.toEqual(
      started,
    );
  });

  it("drifts nowhere at all where the forecast has nothing to say", () => {
    const geometry: FieldGeometry = { ...eastRoad(0), samples: [] };
    const still = particle();

    advanceField([still], geometry, 1);

    expect(still.offsetMetres).toBe(0);
    expect(still.alongMetres).toBe(EAST_METRES / 2);
    expect(still.ageSeconds).toBe(1);
  });
});

describe("how strongly a streak is drawn", () => {
  it("fades in, holds, and fades out again across a life", () => {
    expect(lifeAlpha(particle({ ageSeconds: 0, lifeSeconds: 4 }))).toBe(0);
    expect(lifeAlpha(particle({ ageSeconds: 2, lifeSeconds: 4 }))).toBe(1);
    expect(lifeAlpha(particle({ ageSeconds: 4, lifeSeconds: 4 }))).toBe(0);
    expect(lifeAlpha(particle({ ageSeconds: 0.5, lifeSeconds: 4 }))).toBeCloseTo(0.5, 6);
  });

  it("fades again with the corridor, so the field ends where the wash does", () => {
    const held = particle({ ageSeconds: 2, lifeSeconds: 4 });

    expect(streakAlpha({ ...held, offsetMetres: 0 }, METRES_PER_CELL)).toBe(1);
    expect(streakAlpha({ ...held, offsetMetres: 2500 }, METRES_PER_CELL)).toBeLessThan(1);
    expect(streakAlpha({ ...held, offsetMetres: EDGE_METRES }, METRES_PER_CELL)).toBe(0);
  });

  it("takes the corridor's edge from the forecast's own grid", () => {
    expect(edgeMetresOf(eastRoad(0))).toBe(EDGE_METRES);
  });
});

describe("the field written into a vertex buffer", () => {
  it("writes a tail and a head per streak, the tail at nothing", () => {
    const geometry = eastRoad(0);
    const drifting = particle({ ageSeconds: 1, lifeSeconds: 2 });
    advanceField([drifting], geometry, 0.5);
    const buffer = new Float32Array(VERTICES_PER_STREAK * FLOATS_PER_VERTEX);

    const written = writeStreaks([drifting], geometry, buffer);

    expect(written).toBe(VERTICES_PER_STREAK);
    expect(buffer[2]).toBe(0);
    expect(buffer[5]).toBeCloseTo(streakAlpha(drifting, METRES_PER_CELL), 6);
  });

  it("trails the streak behind the way the air is going", () => {
    const geometry = eastRoad(0);
    const drifting = particle({ ageSeconds: 1, lifeSeconds: 2 });
    advanceField([drifting], geometry, 0.5);
    const buffer = new Float32Array(VERTICES_PER_STREAK * FLOATS_PER_VERTEX);

    writeStreaks([drifting], geometry, buffer);

    // Mercator y grows southward, so a streak blown south has its tail behind
    // it to the north — a smaller y than the head's.
    expect(buffer[1] ?? 0).toBeLessThan(buffer[4] ?? 0);
    expect(buffer[0] ?? 0).toBeCloseTo(buffer[3] ?? 0, 6);
  });

  it("stops at the end of the buffer rather than writing past it", () => {
    const geometry = eastRoad(0);
    const particles = seedField(geometry, 10, sequence(5));
    const buffer = new Float32Array(2 * VERTICES_PER_STREAK * FLOATS_PER_VERTEX);

    expect(writeStreaks(particles, geometry, buffer)).toBe(2 * VERTICES_PER_STREAK);
  });
});

describe("the arrows a reader who asked for no movement gets instead", () => {
  it("points them the way the air is going, not the way it comes from", () => {
    const arrows = staticFlow(eastRoad(0));

    expect(arrows).toHaveLength(MAX_STATIC_ARROWS);
    for (const arrow of arrows) {
      expect(arrow.bearingDegrees).toBeCloseTo(180, 6);
      expect(arrow.speedKmh).toBeCloseTo(20, 6);
    }
  });

  it("spaces them evenly along the ride, inside the corridor and off the road", () => {
    const arrows = staticFlow(eastRoad(0), 4);
    const [first, second] = arrows;

    expect(arrows).toHaveLength(4);
    expect(first?.distanceMetres ?? 0).toBeCloseTo(EAST_METRES / 8, 6);
    expect((second?.distanceMetres ?? 0) - (first?.distanceMetres ?? 0)).toBeCloseTo(
      EAST_METRES / 4,
      6,
    );
    // Beside the road rather than on it, and still well inside the corridor.
    expect(first?.position[1] ?? 0).toBeLessThan(49);
    expect(first?.position[1] ?? 0).toBeGreaterThan(49 - EDGE_METRES / 111_195);
  });

  it("never draws more of them than the corridor can hold", () => {
    expect(staticFlow(eastRoad(0), 100)).toHaveLength(MAX_STATIC_ARROWS);
  });

  it("draws none at all with no reading, no route or none asked for", () => {
    expect(staticFlow({ ...eastRoad(0), samples: [] })).toEqual([]);
    expect(staticFlow({ ...eastRoad(0), totalMetres: 0 })).toEqual([]);
    expect(staticFlow(eastRoad(0), 0)).toEqual([]);
  });
});
