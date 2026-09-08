/**
 * The reader's stopping allowance per moving hour and the door-to-door window
 * it draws around a predicted moving time. A rider preference, kept in this
 * browser alone: no endpoint accepts it and the served figure stays the moving
 * time. Seeds: the current-bike corpus of 242 rides, median 266 s/h, quartiles
 * 114 and 493 s/h.
 *
 * A rider whose own rides measure a habit gets that spread in place of the
 * corpus's. The measurement is served; the choice on the slider is still only
 * ever this browser's.
 */

import { useCallback, useState } from "react";

/** The corpus median, and what a reader who has chosen nothing gets. */
export const DEFAULT_ALLOWANCE_SECONDS_PER_HOUR = 266;

/** The corpus this default and the window's spread were measured on. */
export const CORPUS_RIDES = 242;

/** Far enough for a café day; the slider's domain and the stored bound. */
export const MAX_ALLOWANCE_SECONDS_PER_HOUR = 900;

const SECONDS_PER_HOUR = 3600;

/** How much a rider stops, as a middle and the spread either side of it. */
export interface StoppingSpread {
  medianSecondsPerHour: number;
  lowerQuartileSecondsPerHour: number;
  upperQuartileSecondsPerHour: number;
}

/** The seeded corpus, and what a rider with too few rides of their own gets. */
export const CORPUS_SPREAD: StoppingSpread = {
  medianSecondsPerHour: DEFAULT_ALLOWANCE_SECONDS_PER_HOUR,
  lowerQuartileSecondsPerHour: 114,
  upperQuartileSecondsPerHour: 493,
};

const STORAGE_KEY = "domestique.stopping-allowance";

/** Door to door: the same ride, stopping as little and as much as the spread it was drawn from does. */
export interface ArrivalWindow {
  earliestSeconds: number;
  latestSeconds: number;
}

/**
 * The door-to-door window for `movingSeconds`: the spread's quartiles scaled by
 * the allowance's ratio to its median, so the shape survives wherever the rider
 * puts the middle. Null without a moving time, which the caller shows as no
 * arrival at all.
 */
export function arrivalWindow(
  movingSeconds: number | undefined,
  allowanceSecondsPerHour: number,
  spread: StoppingSpread = CORPUS_SPREAD,
): ArrivalWindow | null {
  if (movingSeconds === undefined || !Number.isFinite(movingSeconds) || movingSeconds <= 0) {
    return null;
  }
  // A measured median of zero would divide by nothing; that rider never stops,
  // so the window is the moving time either side.
  const scale =
    spread.medianSecondsPerHour > 0 ? allowanceSecondsPerHour / spread.medianSecondsPerHour : 0;

  return {
    earliestSeconds:
      movingSeconds * (1 + (spread.lowerQuartileSecondsPerHour * scale) / SECONDS_PER_HOUR),
    latestSeconds:
      movingSeconds * (1 + (spread.upperQuartileSecondsPerHour * scale) / SECONDS_PER_HOUR),
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
