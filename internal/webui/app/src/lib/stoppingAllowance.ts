/**
 * The reader's stopping allowance per moving hour and the door-to-door window
 * it draws around a predicted moving time. A rider preference, kept in this
 * browser alone: no endpoint accepts it and the served figure stays the moving
 * time. Seeds: the current-bike corpus of 242 rides, median 266 s/h, quartiles
 * 114 and 493 s/h.
 */

import { useCallback, useState } from "react";

/** The corpus median, and what a reader who has chosen nothing gets. */
export const DEFAULT_ALLOWANCE_SECONDS_PER_HOUR = 266;

/** The corpus this default and the window's spread were measured on. */
export const CORPUS_RIDES = 242;

/** Far enough for a café day; the slider's domain and the stored bound. */
export const MAX_ALLOWANCE_SECONDS_PER_HOUR = 900;

const LOWER_QUARTILE_SECONDS_PER_HOUR = 114;
const UPPER_QUARTILE_SECONDS_PER_HOUR = 493;
const SECONDS_PER_HOUR = 3600;

const STORAGE_KEY = "domestique.stopping-allowance";

/** Door to door: the same ride, stopping as little and as much as the corpus does. */
export interface ArrivalWindow {
  earliestSeconds: number;
  latestSeconds: number;
}

/**
 * The door-to-door window for `movingSeconds`: the corpus quartiles scaled by
 * the allowance's ratio to the median, so the spread moves with the middle.
 * Null without a moving time, which the caller shows as no arrival at all.
 */
export function arrivalWindow(
  movingSeconds: number | undefined,
  allowanceSecondsPerHour: number,
): ArrivalWindow | null {
  if (movingSeconds === undefined || !Number.isFinite(movingSeconds) || movingSeconds <= 0) {
    return null;
  }
  const scale = allowanceSecondsPerHour / DEFAULT_ALLOWANCE_SECONDS_PER_HOUR;

  return {
    earliestSeconds:
      movingSeconds * (1 + (LOWER_QUARTILE_SECONDS_PER_HOUR * scale) / SECONDS_PER_HOUR),
    latestSeconds:
      movingSeconds * (1 + (UPPER_QUARTILE_SECONDS_PER_HOUR * scale) / SECONDS_PER_HOUR),
  };
}

/** The allowance as minutes, the unit it is shown and chosen in. */
export function formatAllowance(secondsPerHour: number): string {
  return `${(secondsPerHour / 60).toFixed(1)} min`;
}

/** The reader's stopping allowance, remembered across visits where storage allows. */
export function useStoppingAllowance(): [number, (secondsPerHour: number) => void] {
  const [allowance, setAllowance] = useState<number>(readAllowance);

  const choose = useCallback((secondsPerHour: number) => {
    const clamped = clamp(secondsPerHour);
    setAllowance(clamped);
    writeAllowance(clamped);
  }, []);

  return [allowance, choose];
}

function clamp(secondsPerHour: number): number {
  if (!Number.isFinite(secondsPerHour)) {
    return DEFAULT_ALLOWANCE_SECONDS_PER_HOUR;
  }

  return Math.min(Math.max(secondsPerHour, 0), MAX_ALLOWANCE_SECONDS_PER_HOUR);
}

function readAllowance(): number {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    const parsed = stored === null ? Number.NaN : Number(stored);

    return Number.isFinite(parsed) ? clamp(parsed) : DEFAULT_ALLOWANCE_SECONDS_PER_HOUR;
  } catch {
    return DEFAULT_ALLOWANCE_SECONDS_PER_HOUR;
  }
}

function writeAllowance(secondsPerHour: number): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, String(secondsPerHour));
  } catch {
    // A refusal costs the choice its memory, not the page its window.
  }
}
