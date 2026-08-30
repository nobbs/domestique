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
