/**
 * The one-word readings the route panel prints beside its figures.
 *
 * A verdict is a claim in one of the page's tones, so a reader gets "hilly"
 * or "mostly unsealed" without dividing two numbers themselves.
 */

import type { SurfaceKind } from "../api/types";
import type { SurfaceSummary } from "./surface";

export type VerdictTone = "good" | "info" | "hold" | "alert";

export interface Verdict {
  label: string;
  tone: VerdictTone;
}

/**
 * Metres climbed per kilometre ridden, the usual yardstick for how hilly a
 * route is; the thresholds are the conventional ones for road riding.
 */
export function climbingVerdict(ascentMetres: number, distanceMetres: number): Verdict | null {
  if (!(distanceMetres > 0)) {
    return null;
  }
  const perKm = ascentMetres / (distanceMetres / 1000);
  if (perKm < 10) {
    return { label: "Flat", tone: "good" };
  }
  if (perKm < 20) {
    return { label: "Rolling", tone: "info" };
  }
  if (perKm < 30) {
    return { label: "Hilly", tone: "hold" };
  }
  return { label: "Mountainous", tone: "alert" };
}

const UNSEALED: ReadonlySet<SurfaceKind> = new Set(["compacted", "gravel", "ground"]);

export interface SurfaceVerdict extends Verdict {
  /** What the route is mostly made of, as the entry's title. */
  title: string;
  unsealedMetres: number;
}

/**
 * The unsealed share of the surveyed ground. Unsurveyed stretches count on
 * neither side: nobody has said what they are, so they cannot vote.
 */
export function surfaceVerdict(surface: SurfaceSummary | null): SurfaceVerdict | null {
  const surveyed = (surface?.shares ?? []).filter((entry) => entry.kind !== "unknown");
  const total = surveyed.reduce((sum, entry) => sum + entry.metres, 0);
  if (!(total > 0)) {
    return null;
  }
  const unsealedMetres = surveyed
    .filter((entry) => UNSEALED.has(entry.kind))
    .reduce((sum, entry) => sum + entry.metres, 0);
  const share = unsealedMetres / total;
  const percent = Math.round(share * 100);
  if (share < 0.05) {
    return { title: "Sealed surface", label: "Sealed", tone: "good", unsealedMetres };
  }
  if (share < 0.5) {
    return { title: "Mixed surface", label: `${percent}% unsealed`, tone: "hold", unsealedMetres };
  }
  return {
    title: "Unsealed surface",
    label: `${percent}% unsealed`,
    tone: "alert",
    unsealedMetres,
  };
}

/**
 * How hard a ride was, from its intensity factor: normalised power over the
 * rider's threshold. The bands are the usual training-zone ones.
 */
export function intensityVerdict(intensityFactor: number | undefined): Verdict | null {
  if (intensityFactor === undefined || !Number.isFinite(intensityFactor)) {
    return null;
  }
  if (intensityFactor < 0.75) {
    return { label: "Easy", tone: "good" };
  }
  if (intensityFactor < 0.85) {
    return { label: "Steady", tone: "info" };
  }
  if (intensityFactor < 0.95) {
    return { label: "Hard", tone: "hold" };
  }
  return { label: "Very hard", tone: "alert" };
}
