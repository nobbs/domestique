/**
 * The Kaiserstuhl loop's own figures, so a spike can be held against the card
 * the reader is actually unhappy with rather than against a thin fixture.
 *
 * Spike-local and deliberately static: what is being tried here is how the
 * card divides, labels and weights what it holds, and none of that needs the
 * numbers to be live.
 */

import type { SurfaceKind } from "../../../api/types";
import type { MixEntry } from "../../../lib/mix";

export const FIGURES = {
  distance: "49.0 km",
  ascent: "2,730 m",
  elevation: "180–1,090 m",
  averageClimbing: "9.5%",
  steepestClimbing: "19%",
  steepestDescent: "18%",
  movingTime: "4 h 30 min",
  uncertainty: "±7% typical",
} as const;

export const GRADIENT_MIX: MixEntry[] = [
  { label: "flat", description: "under 3%", share: 0.13, metres: 6_400, band: 0 },
  { label: "3%", description: "3 to 6%", share: 0.145, metres: 7_100, band: 1 },
  { label: "6%", description: "6 to 9%", share: 0.167, metres: 8_200, band: 2 },
  { label: "9%", description: "9 to 12%", share: 0.278, metres: 13_600, band: 3 },
  { label: "12%+", description: "12% and steeper", share: 0.28, metres: 13_700, band: 4 },
].map(({ band, ...rest }) => ({
  ...rest,
  highlight: { type: "band", band } as const,
  colour: `var(--grade-${band})`,
}));

const SURFACES: Array<{
  label: string;
  description: string;
  share: number;
  metres: number;
  kind: SurfaceKind;
}> = [
  {
    label: "Asphalt",
    description: "sealed and smooth",
    share: 0.345,
    metres: 16_900,
    kind: "asphalt",
  },
  {
    label: "Paving",
    description: "sealed but rough: setts, bricks",
    share: 0.18,
    metres: 8_800,
    kind: "paving",
  },
  {
    label: "Compacted",
    description: "a firm unsealed surface",
    share: 0.208,
    metres: 10_200,
    kind: "compacted",
  },
  { label: "Gravel", description: "loose stone", share: 0.161, metres: 7_900, kind: "gravel" },
  {
    label: "Ground",
    description: "bare earth or grass",
    share: 0.106,
    metres: 5_200,
    kind: "ground",
  },
];

export const SURFACE_MIX: MixEntry[] = SURFACES.map(({ kind, ...rest }) => ({
  ...rest,
  highlight: { type: "surface", kind },
  colour: `var(--surface-${kind})`,
}));

export interface SpikeClimb {
  ordinal: number;
  length: string;
  average: string;
  steepest: string;
  ascent: string;
  starts: string;
}

export const CLIMBS: SpikeClimb[] = [
  {
    ordinal: 1,
    length: "7.7 km",
    average: "12%",
    steepest: "18%",
    ascent: "898 m",
    starts: "653 m",
  },
  {
    ordinal: 2,
    length: "9.6 km",
    average: "9.3%",
    steepest: "11%",
    ascent: "894 m",
    starts: "20.2 km",
  },
  {
    ordinal: 3,
    length: "7.8 km",
    average: "11%",
    steepest: "19%",
    ascent: "884 m",
    starts: "40.7 km",
  },
];

export const MIX_SUMMARY = "12%+ 13.7 km, asphalt 16.9 km";
export const CLIMB_SUMMARY = "3 climbs · biggest 7.7 km at 12%";
export const TITLE = "Synthetic Kaiserstuhl Loop";

/**
 * A route shaped like the Kaiserstuhl loop, for the spike's chart to draw.
 *
 * Generated rather than fetched: what the workspace spike is judging is how
 * much room a full-width dock gives the profile, and that needs a route with
 * enough shape to fill it. Three cols over forty-nine kilometres, which is
 * what the real one has.
 *
 * The card's own figures above stay as they are. They are text either way, and
 * deriving them here would buy nothing the eye can check.
 */
export const SPIKE_COORDINATES: Array<[number, number, number]> = (() => {
  const shape = Array.from({ length: 246 }, (_, index) => {
    const along = index / 245;
    // Three humps, the middle of them the tallest, on a gentle drift so the
    // route neither starts nor ends at its low point.
    return Math.sin(along * Math.PI * 3) ** 2 * (0.72 + 0.28 * Math.sin(along * Math.PI * 1.1));
  });
  // Scaled to the range the card claims, rather than to whatever the curve
  // happened to peak at — a chart whose axis disagrees with the figure above it
  // is the spike arguing with itself.
  const tallest = Math.max(...shape);

  return shape.map((height, index): [number, number, number] => {
    const along = index / 245;

    return [8.1 + along * 0.55, 48.4 + along * 0.28, 180 + (height / tallest) * 910];
  });
})();

/** Ground under that route, in the order it is ridden. */
export const SPIKE_SURFACE = {
  bands: [
    { kind: "asphalt" as const, startMetres: 0, endMetres: 11_000 },
    { kind: "compacted" as const, startMetres: 11_000, endMetres: 21_200 },
    { kind: "gravel" as const, startMetres: 21_200, endMetres: 29_100 },
    { kind: "ground" as const, startMetres: 29_100, endMetres: 34_300 },
    { kind: "paving" as const, startMetres: 34_300, endMetres: 43_100 },
    { kind: "asphalt" as const, startMetres: 43_100, endMetres: 49_000 },
  ],
  shares: [
    { kind: "asphalt" as const, metres: 16_900, share: 0.345 },
    { kind: "compacted" as const, metres: 10_200, share: 0.208 },
    { kind: "gravel" as const, metres: 7_900, share: 0.161 },
    { kind: "ground" as const, metres: 5_200, share: 0.106 },
    { kind: "paving" as const, metres: 8_800, share: 0.18 },
  ],
  totalMetres: 49_000,
};

export const SPIKE_DISTANCE_METRES = 49_000;
