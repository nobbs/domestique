/**
 * The air's own motion over the ground, as particles adrift in the corridor.
 *
 * The wash beside the route says how hard the wind blows and the tint on the
 * route says what that does to the rider. Neither says which way the air is
 * actually moving, and a still drawing answers that badly: a colour for a
 * bearing is a legend to look up, where a streak that drifts is read without
 * one.
 *
 * Every particle is held in the corridor's own coordinates — `alongMetres` down
 * the route and a signed `offsetMetres` across it — and never as a longitude
 * and latitude. `projectToRoute` scans every segment of the route, so a field
 * kept in ground positions would pay that scan per particle per frame and spend
 * a stage of a few thousand points' whole frame budget on it. Here the corridor
 * test is `corridorWeight` of the offset, death is a comparison, and only the
 * drawing crosses to the ground — three bisections for the head, and the tail
 * from the velocity that put it there.
 *
 * The meteorological convention is the trap. `directionDegrees` is the
 * direction the wind blows *from*, so the air moves toward the opposite
 * bearing; every reading here goes through `flowBearingDegrees` for that one
 * reason, and a wind from the north drives a particle south.
 *
 * Pure: no DOM, no WebGL, no map. What a streak looks like belongs to the layer
 * that draws it.
 */

import type { Position } from "../api/types";
import type { WindSample, WindVector } from "./conditionsField";
import { corridorRadii, corridorWeight, sampleVectorAt } from "./conditionsField";
import { haversineMetres } from "./profile";
import { bearingBetween } from "./routeCues";

/**
 * One degree of latitude on the same spherical model `haversineMetres` uses, so
 * an offset in metres here lands in the units the route's distances are in.
 */
const METRES_PER_DEGREE_LATITUDE = haversineMetres([0, 0], [0, 1]);

/**
 * The most streaks one field is ever made of.
 *
 * Every particle costs a binary search, a vector interpolation and two
 * projections on every frame, and the field has to fit in what is left of a
 * sixteen-millisecond budget after MapLibre has drawn a vector basemap. A few
 * enough of them read as moving air; ten times as many read as fog and cost
 * ten times the frame.
 */
export const MAX_PARTICLES = 1400;

/** How densely the field is seeded before that cap bites: it does past ~70 km. */
export const PARTICLES_PER_KILOMETRE = 20;

/**
 * How much faster than life the field is played.
 *
 * Real air crosses a kilometre-wide corridor in minutes, which on a map reads
 * as nothing moving at all. At sixty the drawing runs a minute of the wind's
 * own travel per second, so a 20 km/h wind carries a streak a kilometre across
 * the ground in three — fast enough to be seen as motion, slow enough that the
 * eye can still follow one streak and take a bearing off it.
 */
export const DRIFT_TIME_SCALE = 60;

/** How long a streak lives before it is respawned elsewhere, in seconds. */
export const PARTICLE_LIFE_SECONDS = 3;

/**
 * How far either side of that a life is drawn, as a share of it.
 *
 * Without the jitter every particle seeded together would also die together,
 * and the whole field would blink.
 */
const LIFE_JITTER = 0.4;

/** The share of a life spent fading in, and the same again fading out. */
const FADE_SHARE = 0.25;

/**
 * How far out a fresh particle may be placed, as a share of the edge radius.
 *
 * Strictly inside it: `isSpent` counts the edge itself as spent, so a particle
 * seeded exactly on it would be respawned again the next frame, over and over,
 * without ever having been drawn.
 */
const SPAWN_EDGE_SHARE = 0.98;

/**
 * How much of the route the corridor's own direction is measured over.
 *
 * Both of the corridor's axes hang off this bearing, so a bearing that changes
 * abruptly moves every particle at once. Stored geometry stair-steps: a road
 * running north-east arrives as short alternating east and north segments, and
 * the bearing of any one of them is a fact about how the route was digitised
 * rather than about the road. Measured over a segment, the frame would flip
 * between two bearings several times a second as a particle crossed vertices,
 * and a streak 500 m off the road would jump half a kilometre sideways each
 * time.
 *
 * Wide enough to span a run of those steps, narrow enough that a real bend
 * still turns the corridor with it — the same trade `BEARING_WINDOW_METRES`
 * makes in `wind.ts`, for the same reason.
 */
export const FRAME_WINDOW_METRES = 600;

/**
 * How short the window's chord may get before it is read as saying nothing, as
 * a share of the window.
 *
 * A hairpin inside one window comes back to where it started, and the bearing
 * between two near-coincident points is noise. The local segment is the honest
 * answer there, discontinuity and all: the corridor genuinely has no single
 * direction across a switchback.
 */
const FRAME_CHORD_MINIMUM_SHARE = 0.05;

/**
 * How much of the drift a streak's tail is drawn from, in seconds of field
 * time. It makes the tail as long as the wind is fast, which is the speed
 * channel repeated — the corridor underneath is what actually carries it.
 */
export const STREAK_SECONDS = 0.8;

/** Interleaved into the vertex buffer: mercator x, mercator y, alpha. */
export const FLOATS_PER_VERTEX = 3;

/** A streak is a tapered wedge: a tail and two head corners, in two triangles. */
export const VERTICES_PER_STREAK = 6;

/** How wide a streak's head is drawn, the same for the corridor and the grid overlay. */
export const STREAK_WIDTH_PIXELS = 2;

/** How far off the road a static arrow stands, as a share of the core radius. */
const ARROW_OFFSET_CORE_SHARE = 0.6;

/** The most arrows drawn in place of the field when motion is refused. */
export const MAX_STATIC_ARROWS = 12;

/** How fast a particle is being carried, in the corridor's own two directions. */
export interface DriftRates {
  /** Down the route, so negative is back toward the start. */
  alongMetresPerSecond: number;
  /** Across it, positive to the right of the direction of travel. */
  acrossMetresPerSecond: number;
}

/** One particle, and the velocity it was last carried at. */
export interface FieldParticle extends DriftRates {
  alongMetres: number;
  /** Signed: positive lies to the right of the way the route is ridden. */
  offsetMetres: number;
  ageSeconds: number;
  lifeSeconds: number;
  /**
   * Whether there was a reading where this particle last sat, carried from the
   * sample `advanceField` already takes so `writeStreaks` need not take a
   * second one per particle per frame.
   */
  hasWind: boolean;
}

/** Everything the field needs to advect a particle and to place it on the map. */
export interface FieldGeometry {
  coordinates: Position[];
  /** `cumulativeMetres(coordinates)`, taken from the caller rather than walked again. */
  distances: number[];
  /** The wind readings along the route, in the order the forecast returns them. */
  samples: WindSample[];
  /** The forecast's grid, which is what fixes the corridor's width. */
  metresPerCell: number;
  totalMetres: number;
}

/** Where the air is going, from the direction a forecast says it comes from. */
export function flowBearingDegrees(directionDegrees: number): number {
  return (((directionDegrees + 180) % 360) + 360) % 360;
}

/**
 * A wind resolved into the corridor's two directions, sped up by
 * `DRIFT_TIME_SCALE`.
 *
 * A wind *from* the north over a road heading east comes out as pure across at
 * the rate of the wind: the air moves south, which is to the right of that
 * road. Over a road heading north the same wind is pure along and negative,
 * carrying the particle back toward the start.
 */
export function driftRates(wind: WindVector, routeBearingDegrees: number): DriftRates {
  const metresPerSecond = (wind.speedKmh / 3.6) * DRIFT_TIME_SCALE;
  const radians =
    ((flowBearingDegrees(wind.directionDegrees) - routeBearingDegrees) * Math.PI) / 180;

  return {
    alongMetresPerSecond: metresPerSecond * Math.cos(radians),
    acrossMetresPerSecond: metresPerSecond * Math.sin(radians),
  };
}

/**
 * The segment `alongMetres` falls on, by bisection over ascending distances.
 *
 * The whole point of holding a particle in corridor coordinates: this is the
 * only search the field ever does, and it is logarithmic where projecting a
 * ground position onto the route is a scan of every segment.
 */
export function segmentAt(distances: number[], alongMetres: number): number {
  let low = 0;
  let high = Math.max(distances.length - 2, 0);
  while (low < high) {
    const middle = (low + high + 1) >> 1;
    if ((distances[middle] ?? 0) <= alongMetres) {
      low = middle;
    } else {
      high = middle - 1;
    }
  }

  return low;
}

/** How far into its segment a distance sits, from 0 at one end to 1 at the other. */
function ratioWithin(distances: number[], index: number, alongMetres: number): number {
  const start = distances[index] ?? 0;
  const end = distances[index + 1] ?? start;

  return end > start ? Math.min(Math.max((alongMetres - start) / (end - start), 0), 1) : 0;
}

/** The point on the route at one distance along it, clamped to either end. */
function pointAt(
  coordinates: Position[],
  distances: number[],
  alongMetres: number,
): Position | null {
  const index = segmentAt(distances, alongMetres);
  const from = coordinates[index];
  const to = coordinates[index + 1];
  if (!from || !to) {
    return null;
  }
  const ratio = ratioWithin(distances, index, alongMetres);

  return [from[0] + ratio * (to[0] - from[0]), from[1] + ratio * (to[1] - from[1])];
}

/**
 * Which way the corridor points at one distance along the route, in degrees
 * from north.
 *
 * The chord across `FRAME_WINDOW_METRES` of route centred on the distance,
 * which is continuous in it — the ends of the chord slide along the polyline
 * rather than snapping from one segment's bearing to the next's, so the frame
 * a particle is carried in never jumps.
 */
export function routeBearingAt(
  coordinates: Position[],
  distances: number[],
  alongMetres: number,
): number {
  const half = FRAME_WINDOW_METRES / 2;
  const behind = pointAt(coordinates, distances, alongMetres - half);
  const ahead = pointAt(coordinates, distances, alongMetres + half);
  if (
    behind &&
    ahead &&
    haversineMetres(behind, ahead) > FRAME_WINDOW_METRES * FRAME_CHORD_MINIMUM_SHARE
  ) {
    return bearingBetween(behind, ahead);
  }
  const index = segmentAt(distances, alongMetres);
  const from = coordinates[index];
  const to = coordinates[index + 1];

  return from && to ? bearingBetween(from, to) : 0;
}

/**
 * A position moved so many metres east and so many north of itself.
 *
 * Flat-earth, which is good to a fraction of a percent over a corridor a few
 * kilometres wide — well inside the grid cell the reading came from.
 */
function displaced(position: Position, eastMetres: number, northMetres: number): Position {
  const [longitude, latitude] = position;
  const longitudeScale = Math.cos((latitude * Math.PI) / 180);

  return [
    longitude + eastMetres / (METRES_PER_DEGREE_LATITUDE * longitudeScale || 1),
    latitude + northMetres / METRES_PER_DEGREE_LATITUDE,
  ];
}

/**
 * A point on the route, moved out to its offset across a corridor pointing
 * `frameDegrees`.
 *
 * Takes the bearing rather than looking it up, so a caller that needs the frame
 * for something else too — `writeStreaks` needs it for the streak itself — pays
 * for it once.
 */
function placeInFrame(on: Position, frameDegrees: number, offsetMetres: number): Position {
  // A quarter turn clockwise off the way the corridor points, so a positive
  // offset lies to the right of a rider on it.
  const normal = ((frameDegrees + 90) * Math.PI) / 180;

  return displaced(on, offsetMetres * Math.sin(normal), offsetMetres * Math.cos(normal));
}

/**
 * Corridor coordinates back onto the ground: along the route to the distance,
 * then sideways by the corridor's normal. Null where there is no route to walk.
 */
export function positionAt(
  coordinates: Position[],
  distances: number[],
  alongMetres: number,
  offsetMetres: number,
): Position | null {
  if (coordinates.length < 2 || coordinates.length !== distances.length) {
    return null;
  }
  const on = pointAt(coordinates, distances, alongMetres);

  return on
    ? placeInFrame(on, routeBearingAt(coordinates, distances, alongMetres), offsetMetres)
    : null;
}

/**
 * Where a streak has come from: its head, carried back along the ground
 * velocity the corridor's own rates work out to.
 *
 * The rates are held in the corridor's axes, so turning them back into a
 * direction over the ground is the frame's bearing applied the other way round
 * — which returns exactly the direction the air is going, whatever the route
 * beneath is doing. Reading the tail out of the corridor a second time instead,
 * at the distance the particle came from, would put the polyline's own
 * stair-stepping into the one channel this layer carries: the streak would
 * swing tens of degrees either side of the wind as the ends of it crossed
 * vertices, several times a second.
 */
export function streakTail(
  head: Position,
  particle: DriftRates,
  frameDegrees: number,
  seconds: number = STREAK_SECONDS,
): Position {
  const radians = (frameDegrees * Math.PI) / 180;
  const sin = Math.sin(radians);
  const cos = Math.cos(radians);
  const { alongMetresPerSecond: along, acrossMetresPerSecond: across } = particle;

  return displaced(
    head,
    -(along * sin + across * cos) * seconds,
    -(along * cos - across * sin) * seconds,
  );
}

/**
 * A position as Web Mercator, the world square from 0 to 1 that a MapLibre
 * custom layer's matrix expects — the same numbers `MercatorCoordinate` gives,
 * computed here so a field of streaks costs no object per vertex per frame.
 */
export function mercatorXY(position: Position): [number, number] {
  const [longitude, latitude] = position;
  const clamped = Math.min(Math.max(latitude, -85.051129), 85.051129);
  const radians = (clamped * Math.PI) / 180;

  return [
    (180 + longitude) / 360,
    (180 - (180 / Math.PI) * Math.log(Math.tan(Math.PI / 4 + radians / 2))) / 360,
  ];
}

/** One screen pixel in the 0..1 world square, for a 512 px tile. */
export function mercatorPerPixelAt(zoom: number): number {
  return 1 / (512 * 2 ** zoom);
}

/** How far out the corridor still says anything, and so where a particle dies. */
export function edgeMetresOf(geometry: FieldGeometry): number {
  return corridorRadii(geometry.metresPerCell).edgeMetres;
}

/** How many streaks a route of this length is seeded with. */
export function fieldSize(totalMetres: number): number {
  if (!Number.isFinite(totalMetres) || totalMetres <= 0) {
    return 0;
  }

  return Math.min(
    MAX_PARTICLES,
    Math.max(1, Math.round((totalMetres / 1000) * PARTICLES_PER_KILOMETRE)),
  );
}

function lifeSeconds(random: () => number): number {
  return PARTICLE_LIFE_SECONDS * (1 + LIFE_JITTER * (2 * random() - 1));
}

/**
 * A particle put back into the corridor at a fresh place, rather than a new
 * one: the field's size is fixed, so respawning is what keeps its density even
 * as the wind carries streaks out of it.
 *
 * Uniform in the corridor's coordinates, which is uniform on the ground too —
 * the two axes are metres either way. Streaks that land far out are drawn
 * faint by `streakAlpha` rather than kept away from the edge.
 */
export function respawn(
  particle: FieldParticle,
  geometry: FieldGeometry,
  random: () => number,
): void {
  const edgeMetres = edgeMetresOf(geometry);
  particle.alongMetres = random() * geometry.totalMetres;
  particle.offsetMetres = (2 * random() - 1) * SPAWN_EDGE_SHARE * edgeMetres;
  particle.ageSeconds = 0;
  particle.lifeSeconds = lifeSeconds(random);
  particle.alongMetresPerSecond = 0;
  particle.acrossMetresPerSecond = 0;
  particle.hasWind = sampleVectorAt(geometry.samples, particle.alongMetres) !== null;
}

/** A field of `count` particles, each already part way through a life of its own. */
export function seedField(
  geometry: FieldGeometry,
  count: number,
  random: () => number = Math.random,
): FieldParticle[] {
  return Array.from({ length: Math.max(count, 0) }, () => {
    const particle: FieldParticle = {
      alongMetres: 0,
      offsetMetres: 0,
      ageSeconds: 0,
      lifeSeconds: PARTICLE_LIFE_SECONDS,
      alongMetresPerSecond: 0,
      acrossMetresPerSecond: 0,
      hasWind: false,
    };
    respawn(particle, geometry, random);
    // Seeded mid-life, or the whole field would fade in and out together.
    particle.ageSeconds = random() * particle.lifeSeconds;

    return particle;
  });
}

/** Whether a particle has run out of life, corridor, or route to drift along. */
export function isSpent(particle: FieldParticle, geometry: FieldGeometry): boolean {
  return (
    particle.ageSeconds >= particle.lifeSeconds ||
    Math.abs(particle.offsetMetres) >= edgeMetresOf(geometry) ||
    particle.alongMetres < 0 ||
    particle.alongMetres > geometry.totalMetres
  );
}

/** How present a particle is across its life: in, held, then out again. */
export function lifeAlpha(particle: Pick<FieldParticle, "ageSeconds" | "lifeSeconds">): number {
  if (particle.lifeSeconds <= 0) {
    return 0;
  }
  const share = Math.min(Math.max(particle.ageSeconds / particle.lifeSeconds, 0), 1);

  return Math.max(Math.min(1, share / FADE_SHARE, (1 - share) / FADE_SHARE), 0);
}

/**
 * How strongly a streak is drawn: its own life, faded again by how much the
 * corridor still speaks for the ground it is over.
 *
 * The same falloff the wash is painted with, read from the one definition of
 * it, so the field ends exactly where the corridor does.
 */
export function streakAlpha(particle: FieldParticle, metresPerCell: number): number {
  return lifeAlpha(particle) * corridorWeight(Math.abs(particle.offsetMetres), metresPerCell);
}

/**
 * The field carried forward by `seconds`, in place.
 *
 * In place because this runs on every frame: a field that allocated a new
 * particle per streak per frame would hand the collector twenty thousand
 * objects a second on a machine already drawing a map.
 */
export function advanceField(
  particles: FieldParticle[],
  geometry: FieldGeometry,
  seconds: number,
  random: () => number = Math.random,
): void {
  for (const particle of particles) {
    particle.ageSeconds += seconds;
    const wind = sampleVectorAt(geometry.samples, particle.alongMetres);
    particle.hasWind = wind !== null;
    if (wind) {
      const rates = driftRates(
        wind,
        routeBearingAt(geometry.coordinates, geometry.distances, particle.alongMetres),
      );
      particle.alongMetresPerSecond = rates.alongMetresPerSecond;
      particle.acrossMetresPerSecond = rates.acrossMetresPerSecond;
      particle.alongMetres += rates.alongMetresPerSecond * seconds;
      particle.offsetMetres += rates.acrossMetresPerSecond * seconds;
    }
    if (isSpent(particle, geometry)) {
      respawn(particle, geometry, random);
    }
  }
}

/**
 * A tapered wedge for one streak, written straight into the buffer: a tail
 * vertex at alpha 0 and two head corners at `alpha`, in two triangles sharing
 * the tail. Shared by `writeStreaks` and `writeGridStreaks` so the corridor
 * field and the weather overlay draw the identical shape.
 */
export function writeWedge(
  into: Float32Array,
  at: number,
  tailX: number,
  tailY: number,
  headX: number,
  headY: number,
  alpha: number,
  halfWidth: number,
): void {
  const length = Math.hypot(headX - tailX, headY - tailY) || 1;
  const acrossX = (-(headY - tailY) / length) * halfWidth;
  const acrossY = ((headX - tailX) / length) * halfWidth;
  const headLeftX = headX + acrossX;
  const headLeftY = headY + acrossY;
  const headRightX = headX - acrossX;
  const headRightY = headY - acrossY;
  into[at] = tailX;
  into[at + 1] = tailY;
  into[at + 2] = 0;
  into[at + 3] = headLeftX;
  into[at + 4] = headLeftY;
  into[at + 5] = alpha;
  into[at + 6] = headRightX;
  into[at + 7] = headRightY;
  into[at + 8] = alpha;
  into[at + 9] = tailX;
  into[at + 10] = tailY;
  into[at + 11] = 0;
  into[at + 12] = headRightX;
  into[at + 13] = headRightY;
  into[at + 14] = alpha;
  into[at + 15] = headLeftX;
  into[at + 16] = headLeftY;
  into[at + 17] = alpha;
}

/**
 * The field written into the vertex buffer the layer draws: a tapered wedge
 * per particle, the same shape `writeGridStreaks` draws for the weather
 * overlay.
 *
 * Returns how many vertices were written, which is what the draw call is given
 * — a particle with no reading to drift on is left out rather than drawn still.
 * The tail is computed from the velocity rather than remembered from the last
 * frame, so a respawned particle never draws a wedge across the map.
 * `mercatorPerPixel` is the world square's own size of one screen pixel, which
 * is what the wedge's width is measured in.
 */
export function writeStreaks(
  particles: FieldParticle[],
  geometry: FieldGeometry,
  into: Float32Array,
  mercatorPerPixel: number,
): number {
  const { coordinates, distances, metresPerCell } = geometry;
  const halfWidth = (STREAK_WIDTH_PIXELS / 2) * mercatorPerPixel;
  let written = 0;
  for (const particle of particles) {
    if (written + VERTICES_PER_STREAK > into.length / FLOATS_PER_VERTEX) {
      break;
    }
    if (!particle.hasWind) {
      continue;
    }
    const on = pointAt(coordinates, distances, particle.alongMetres);
    if (!on) {
      continue;
    }
    const frameDegrees = routeBearingAt(coordinates, distances, particle.alongMetres);
    const head = placeInFrame(on, frameDegrees, particle.offsetMetres);
    const tail = streakTail(head, particle, frameDegrees);
    const alpha = streakAlpha(particle, metresPerCell);
    const [tailX, tailY] = mercatorXY(tail);
    const [headX, headY] = mercatorXY(head);
    writeWedge(into, written * FLOATS_PER_VERTEX, tailX, tailY, headX, headY, alpha, halfWidth);
    written += VERTICES_PER_STREAK;
  }

  return written;
}

/** One arrow standing in the corridor, pointing the way the air is moving. */
export interface FlowArrow {
  distanceMetres: number;
  position: Position;
  /** Where the air is going, degrees clockwise from north. */
  bearingDegrees: number;
  speedKmh: number;
}

/**
 * The same directions as still arrows, for a reader who has asked for no
 * movement at all.
 *
 * Sparse and evenly spaced along the route rather than one per forecast
 * reading: the readings are moments of the ride, and a dozen arrows is as much
 * as can stand on a corridor without becoming a texture. Set off the road by a
 * share of the core radius, so an arrow is inside the corridor it belongs to
 * without sitting on the line the reader is following.
 */
export function staticFlow(geometry: FieldGeometry, count = MAX_STATIC_ARROWS): FlowArrow[] {
  const { coordinates, distances, samples, metresPerCell, totalMetres } = geometry;
  const wanted = Math.min(Math.max(count, 0), MAX_STATIC_ARROWS);
  if (wanted === 0 || totalMetres <= 0 || samples.length === 0) {
    return [];
  }
  const offsetMetres = corridorRadii(metresPerCell).coreMetres * ARROW_OFFSET_CORE_SHARE;

  return Array.from(
    { length: wanted },
    (_, index) => (totalMetres * (index + 0.5)) / wanted,
  ).flatMap((distanceMetres) => {
    const wind = sampleVectorAt(samples, distanceMetres);
    const position = positionAt(coordinates, distances, distanceMetres, offsetMetres);

    return wind && position
      ? [
          {
            distanceMetres,
            position,
            bearingDegrees: flowBearingDegrees(wind.directionDegrees),
            speedKmh: wind.speedKmh,
          },
        ]
      : [];
  });
}
