/**
 * How the fitness page reads the service's timeline: one scale at a time, form
 * as a share of fitness, and the bands both form and ramp rate fall into.
 *
 * The bands are docs/specs/measurement.md §Training load; change them there first.
 */

import type { FitnessDay, FitnessOutlook, FitnessScaleOutlook } from "../../api/types";

/** Which of the two load scales the page is reading. */
export type Scale = "tss" | "trimp";

/** One day on one scale. */
export interface Reading {
  date: string;
  load: number;
  fitness: number;
  fatigue: number;
  form: number;
  /** Form as a percentage of fitness, which reads the same on either scale. */
  formPercent: number;
}

/** Form as a percentage of fitness; nought while there is too little fitness to divide by. */
export function formPercent(form: number, fitness: number): number {
  return fitness > 1 ? (form / fitness) * 100 : 0;
}

export function reading(day: FitnessDay, scale: Scale): Reading {
  const [load, fitness, fatigue, form] =
    scale === "tss"
      ? [day.tssLoad, day.tssFitness, day.tssFatigue, day.tssForm]
      : [day.trimpLoad, day.trimpFitness, day.trimpFatigue, day.trimpForm];

  return { date: day.date, load, fitness, fatigue, form, formPercent: formPercent(form, fitness) };
}

export function scaleOutlook(outlook: FitnessOutlook, scale: Scale): FitnessScaleOutlook {
  return scale === "tss" ? outlook.tss : outlook.trimp;
}

export interface Band {
  name: string;
  /** Inclusive lower edge, in percent; the lowest band's is -Infinity. */
  from: number;
  colour: string;
  advice: string;
}

/** Highest first, so the first band a value clears is its own. */
export const FORM_BANDS: readonly Band[] = [
  { name: "Transition", from: 20, colour: "var(--rule)", advice: "fitness is fading" },
  { name: "Fresh", from: 5, colour: "var(--good)", advice: "ready to race or test" },
  { name: "Grey zone", from: -10, colour: "var(--ink-2)", advice: "maintaining" },
  { name: "Optimal", from: -30, colour: "var(--accent)", advice: "productive training" },
  { name: "High risk", from: -Infinity, colour: "var(--alert)", advice: "back off soon" },
];

/** Ramp as a percentage of the fitness seven days earlier. */
export const RAMP_BANDS: readonly Band[] = [
  { name: "Aggressive", from: 10, colour: "var(--alert)", advice: "hard to sustain" },
  { name: "Building", from: 3, colour: "var(--good)", advice: "fitness is rising" },
  { name: "Holding", from: -3, colour: "var(--ink-2)", advice: "fitness is steady" },
  { name: "Detraining", from: -Infinity, colour: "var(--hold)", advice: "fitness is falling" },
];

export function bandOf(bands: readonly Band[], percent: number): Band {
  return bands.find((band) => percent >= band.from) ?? (bands[bands.length - 1] as Band);
}

/** The upper edge of a band, which is the lower edge of the one above it. */
export function bandTop(bands: readonly Band[], band: Band): number {
  return bands[bands.indexOf(band) - 1]?.from ?? Infinity;
}

/** The Monday a calendar day's week began on, as a calendar day. */
export function mondayOf(date: string): string {
  const at = new Date(`${date}T00:00:00Z`);
  at.setUTCDate(at.getUTCDate() - ((at.getUTCDay() + 6) % 7));

  return at.toISOString().slice(0, 10);
}

/** Whole days from one calendar day to another; negative when `to` is earlier. */
export function daysBetween(from: string, to: string): number {
  return Math.round((Date.parse(`${to}T00:00:00Z`) - Date.parse(`${from}T00:00:00Z`)) / 86_400_000);
}

/** Each week's summed load, by its Monday, oldest first. */
export function weeklyLoads(readings: readonly Reading[]): Map<string, number> {
  const weeks = new Map<string, number>();
  for (const one of readings) {
    const monday = mondayOf(one.date);
    weeks.set(monday, (weeks.get(monday) ?? 0) + one.load);
  }

  return weeks;
}

/** `+3`, `−12`, `±0`: a change, with a real minus sign. */
export function signed(value: number, digits = 0): string {
  const rounded = Number(value.toFixed(digits));
  const sign = rounded > 0 ? "+" : rounded < 0 ? "−" : "±";

  return `${sign}${Math.abs(rounded).toFixed(digits)}`;
}
