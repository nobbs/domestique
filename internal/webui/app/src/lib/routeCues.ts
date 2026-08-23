/**
 * Where a stage starts, where it finishes, and which way it is ridden.
 *
 * A painted line says where a ride goes and nothing about which end it begins
 * at. For a loop it says even less: the two ends are the same point, so the
 * question "which way round?" has no answer anywhere on the map. What is
 * derived here answers both, from the same stored coordinates already being
 * drawn — nothing is fetched, and no answer depends on the basemap.
 *
 * The cues are geometry, not pictures: the terminals are the coordinates
 * themselves, and a direction cue is a two-armed chevron built from three
 * positions. That keeps every decision here testable without a canvas, which is
 * the only way this map's behaviour can be asked about at all — see
 * `RouteMap.test.tsx`.
 */

import type { Position } from "../api/types";
import { formatDistance } from "./format";
import { cumulativeMetres, haversineMetres } from "./profile";
import type { UnitSystem } from "./units";

/**
 * Web Mercator ground resolution at zoom 0 on the equator, in metres per pixel.
 *
 * Cues that keep their size and spacing on screen have to be built in metres,
 * because the geometry they are drawn from is in degrees. This is the one
 * conversion between the two.
 */
const EQUATOR_METRES_PER_PIXEL = 156_543.03392;

/** Metres in one degree of latitude, close enough for a cue a few pixels long. */
const METRES_PER_DEGREE_LATITUDE = 111_320;

/**
 * How far apart chevrons are asked to sit, in pixels on screen.
 *
 * Spacing is measured in pixels rather than in metres so the answer scales with
 * both the length of the route and the zoom: a stage framed on screen gets the
 * same handful of cues whether it is 8 km or 200 km, and zooming in spreads them
 * apart rather than multiplying them.
 */
export const CHEVRON_SPACING_PIXELS = 120;

/** How long each arm of a chevron runs, in pixels on screen. */
export const CHEVRON_ARM_PIXELS = 7;

/** How far an arm swings back from the direction of travel. */
const CHEVRON_SPREAD_DEGREES = 35;

/**
 * The most chevrons any one stage gets.
 *
 * Pixel spacing already bounds this for a stage framed on screen. The cap is for
 * the case it does not — a route far longer than the window, or a camera pulled
 * back past it — where the honest answer is still a few cues pointing the way
 * rather than a row of arrows standing in for the route.
 */
export const MAX_CHEVRONS = 20;

/**
 * How much room a chevron leaves the terminal markers, in pixels.
 *
 * A cue under a marker is a cue nobody can read, and the start marker is exactly
 * where a reader looks first.
 */
const TERMINAL_CLEARANCE_PIXELS = 26;

/**
 * How close the two ends have to be to count as the same place.
 *
 * A loop rarely closes on the exact stored coordinate, and an out-and-back
 * finishes wherever the rider stopped rolling. Both are stages whose two
 * markers would otherwise be drawn on top of each other, which is the case
 * worth detecting; whether the ground between them was a loop or a there-and-
 * back is not something two end points can answer, so nothing here claims it.
 */
export const SHARED_TERMINAL_METRES = 100;

/**
 * How far along the route a departure or arrival bearing is measured.
 *
 * Neighbouring stored points can be a metre apart, and the heading between two
 * of them is as much satellite noise as direction. Far enough to read as leaving
 * somewhere, short enough that it is still the same street.
 */
const BEARING_METRES = 40;

/** Ground covered by one pixel at a zoom and latitude, in Web Mercator. */
export function metresPerPixel(zoom: number, latitude: number): number {
  return (EQUATOR_METRES_PER_PIXEL * Math.cos((latitude * Math.PI) / 180)) / 2 ** zoom;
}

/** Compass bearing from one position to another, in degrees clockwise of north. */
export function bearingBetween(from: Position, to: Position): number {
  const [fromLongitude, fromLatitude] = from;
  const [toLongitude, toLatitude] = to;
  const radians = Math.PI / 180;
  const longitudeDelta = (toLongitude - fromLongitude) * radians;
  const y = Math.sin(longitudeDelta) * Math.cos(toLatitude * radians);
  const x =
    Math.cos(fromLatitude * radians) * Math.sin(toLatitude * radians) -
    Math.sin(fromLatitude * radians) * Math.cos(toLatitude * radians) * Math.cos(longitudeDelta);

  return ((Math.atan2(y, x) * 180) / Math.PI + 360) % 360;
}

const COMPASS_POINTS = [
  "north",
  "north-east",
  "east",
  "south-east",
  "south",
  "south-west",
  "west",
  "north-west",
] as const;

/**
 * The bearing as a word.
 *
 * Eight points, because this is read aloud rather than navigated by: "east
 * north-east" is more precision than a sentence about where a ride goes can
 * carry, and less than a rider would use a compass for.
 */
export function compassPoint(bearing: number): string {
  const normalised = ((bearing % 360) + 360) % 360;
  const point = COMPASS_POINTS[Math.round(normalised / 45) % 8];

  return point ?? "north";
}

/** Moves a position a distance along a bearing. */
function offsetPosition(position: Position, bearing: number, metres: number): Position {
  const [longitude, latitude] = position;
  const radians = (bearing * Math.PI) / 180;
  const latitudeDelta = (metres * Math.cos(radians)) / METRES_PER_DEGREE_LATITUDE;
  const longitudeScale = Math.max(Math.cos((latitude * Math.PI) / 180), 1e-6);
  const longitudeDelta =
    (metres * Math.sin(radians)) / (METRES_PER_DEGREE_LATITUDE * longitudeScale);

  return [longitude + longitudeDelta, latitude + latitudeDelta];
}

/** The two ends of a stage, which way it leaves, and which way it comes back. */
export interface RouteCues {
  start: Position;
  finish: Position;
  /** Whether the two ends are close enough that one marker would hide the other. */
  sharedTerminal: boolean;
  /** The heading the ride leaves the start on. */
  departure: number;
  /** The heading the ride is still travelling on as it reaches the finish. */
  arrival: number;
  /** How far apart the two ends lie, straight across the ground. */
  separationMetres: number;
  /** How far the ride is, along the route. */
  lengthMetres: number;
}

/**
 * Reads the terminals and headings of a stage.
 *
 * Null for anything that is not a ride: a stage of one point has an end but no
 * direction, and there is nothing honest to draw for it.
 */
export function routeCues(coordinates: Position[]): RouteCues | null {
  const start = coordinates[0];
  const finish = coordinates[coordinates.length - 1];
  if (!start || !finish || coordinates.length < 2) {
    return null;
  }
  const distances = cumulativeMetres(coordinates);
  const lengthMetres = distances[distances.length - 1] ?? 0;
  if (lengthMetres <= 0) {
    return null;
  }
  // Measured over the same ground at both ends, which is what makes "left to
  // the north-east and came back from the south-west" one comparison rather
  // than two readings at whatever resolution the source happened to store.
  const separationMetres = haversineMetres(start, finish);
  const reach = Math.min(BEARING_METRES, lengthMetres);
  const departureAt = distances.findIndex((metres) => metres >= reach);
  // The last point still that far short of the finish, rather than the first one
  // past it: the first is the finish itself on any stage whose final stored
  // points are closer together than the reach, and a bearing from the finish to
  // the finish is no bearing at all.
  const arrivalAt = distances.findLastIndex((metres) => metres <= lengthMetres - reach);

  return {
    start,
    finish,
    sharedTerminal: separationMetres <= SHARED_TERMINAL_METRES,
    departure: bearingBetween(start, coordinates[departureAt] ?? finish),
    arrival: bearingBetween(coordinates[arrivalAt] ?? start, finish),
    separationMetres,
    lengthMetres,
  };
}

/**
 * The cues in words, for a reader who is not looking at the canvas.
 *
 * A map's markers and arrows are drawn into a WebGL surface that carries no text
 * at all, so this is not a caption for the picture — it is the whole of what a
 * screen reader has. It therefore says what the cues mean rather than what they
 * look like: which way the ride leaves, and whether it comes back to where it
 * started.
 */
export function cuesDescription(cues: RouteCues, system: UnitSystem): string {
  const leaving = `leaves the start heading ${compassPoint(cues.departure)}`;
  // In the same place rather than at the same point: the two ends only have to
  // be close enough that one marker would hide the other, and a stage that
  // finishes a street away from where it started is a loop to a rider.
  if (cues.sharedTerminal) {
    return `Starts and finishes in the same place. The ride ${leaving} and returns from the ${compassPoint(cues.arrival + 180)}, ${formatDistance(cues.lengthMetres, system)} later.`;
  }

  // Apart is measured across the ground, not along the route: a stage that
  // wanders can be far longer than the gap between its ends, and saying the
  // length here would tell a reader the finish is much further away than it is.
  return `Starts and finishes ${formatDistance(cues.separationMetres, system)} apart, the finish lying to the ${compassPoint(bearingBetween(cues.start, cues.finish))}. The ride ${leaving}.`;
}

/**
 * Chevrons along the route, each pointing the way it is ridden.
 *
 * One chevron is three positions — an arm, the tip, the other arm — so the whole
 * set draws as one line layer and needs no image, sprite, or font from the
 * basemap style. They are built in metres from the caller's ground resolution,
 * which is what keeps their size and spacing steady on screen as the camera
 * moves.
 */
export function directionChevrons(
  coordinates: Position[],
  options: { metresPerPixel: number },
): Position[][] {
  const cues = routeCues(coordinates);
  if (!cues || !(options.metresPerPixel > 0)) {
    return [];
  }
  const distances = cumulativeMetres(coordinates);
  const total = cues.lengthMetres;
  const clearance = TERMINAL_CLEARANCE_PIXELS * options.metresPerPixel;
  const from = clearance;
  const to = total - clearance;
  // A stage shorter than the room its own markers take gets no chevron: the
  // markers are already touching, and a cue between them would be drawn under
  // both. Which way it was ridden is still said in words.
  if (to <= from) {
    return [];
  }
  const span = to - from;
  const spacing = Math.max(
    CHEVRON_SPACING_PIXELS * options.metresPerPixel,
    span / (MAX_CHEVRONS - 1),
  );
  const arm = CHEVRON_ARM_PIXELS * options.metresPerPixel;

  const chevrons: Position[][] = [];
  for (let travelled = from; travelled <= to + 1e-9; travelled += spacing) {
    const index = segmentAt(distances, travelled);
    const behind = coordinates[index];
    const ahead = coordinates[index + 1];
    if (!behind || !ahead) {
      continue;
    }
    const bearing = bearingBetween(behind, ahead);
    const along = distances[index] ?? 0;
    const segment = (distances[index + 1] ?? along) - along;
    const tip: Position =
      segment > 0
        ? offsetPosition(behind, bearing, travelled - along)
        : // Dropping any stored elevation, so a repeated point cannot leave one
          // chevron carrying a mix of two- and three-number positions.
          [behind[0], behind[1]];
    chevrons.push([
      offsetPosition(tip, bearing + 180 - CHEVRON_SPREAD_DEGREES, arm),
      tip,
      offsetPosition(tip, bearing + 180 + CHEVRON_SPREAD_DEGREES, arm),
    ]);
  }

  return chevrons;
}

/** The index of the segment a distance along the route falls in. */
function segmentAt(distances: number[], metres: number): number {
  for (let index = distances.length - 2; index >= 0; index--) {
    if ((distances[index] ?? 0) <= metres) {
      return index;
    }
  }

  return 0;
}
