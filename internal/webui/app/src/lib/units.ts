/**
 * Whether distance and elevation display in metric or imperial units.
 *
 * The service stores and transmits metric values throughout — this is a
 * display preference only, read by the formatters in `format.ts` and by the
 * elevation chart's own axis and readout. Remembered the same way
 * `basemap.ts` remembers a basemap choice: one namespaced `localStorage` key,
 * guarded against a browser that refuses storage outright.
 */

import { useCallback, useState } from "react";

const STORAGE_KEY = "domestique.units";

export type UnitSystem = "metric" | "imperial";

const METRES_PER_MILE = 1609.344;
const FEET_PER_METRE = 3.28084;

export function metresToFeet(metres: number): number {
  return metres * FEET_PER_METRE;
}

export function metresToMiles(metres: number): number {
  return metres / METRES_PER_MILE;
}

/** The name of this system's long-distance unit: a kilometre or a mile. */
export function distanceUnitLabel(system: UnitSystem): string {
  return system === "imperial" ? "mi" : "km";
}

/** The name of this system's height unit: a metre or a foot. */
export function elevationUnitLabel(system: UnitSystem): string {
  return system === "imperial" ? "ft" : "m";
}

/** A height or a climb, converted for display. */
export function elevationValue(metres: number, system: UnitSystem): number {
  return system === "imperial" ? metresToFeet(metres) : metres;
}

/** A distance in the display unit — kilometres or miles — as a bare number. */
export function distanceValue(metres: number, system: UnitSystem): number {
  return system === "imperial" ? metresToMiles(metres) : metres / 1000;
}

/**
 * A distance in the display unit, with just enough decimals that a step this
 * size still tells neighbouring labels apart. Whole kilometres or miles are
 * right for a whole route and useless for a four-hundred-metre window, where
 * every label would read the same number.
 *
 * `stepKilometres` is always in kilometres, whichever system is on screen —
 * the chart it labels chooses its gridlines in kilometre-space regardless, so
 * that a change of unit only ever changes the numbers a label reads, never
 * where the gridlines themselves fall.
 */
export function distanceLabel(metres: number, stepKilometres: number, system: UnitSystem): string {
  const value = distanceValue(metres, system);
  const step = distanceValue(stepKilometres * 1000, system);
  const decimals = Math.min(Math.max(Math.ceil(-Math.log10(step)), 0), 3);

  return value.toFixed(decimals);
}

/** The reader's chosen unit system, remembered across visits. Metric by default. */
export function useUnitSystem(): [UnitSystem, (system: UnitSystem) => void] {
  const [system, setSystem] = useState<UnitSystem>(readSystem);

  const choose = useCallback((next: UnitSystem) => {
    setSystem(next);
    writeSystem(next);
  }, []);

  return [system, choose];
}

function readSystem(): UnitSystem {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === "imperial" ? "imperial" : "metric";
  } catch {
    return "metric";
  }
}

function writeSystem(system: UnitSystem): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, system);
  } catch {
    // Remembering is the whole of what is lost, and the pick still stands for
    // as long as the page is open.
  }
}
