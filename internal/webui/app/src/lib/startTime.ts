/**
 * The reader's chosen ride start time, used to turn a stage's forecast
 * samples — each one an elapsed time since departure — into real moments
 * worth asking `GET /v1/weather` about.
 *
 * Modelled closely on `theme.ts`, with one difference: a stored start time
 * can go stale just by the calendar moving on, in a way a theme choice never
 * does. The read guard here therefore checks a stored value against the same
 * window the endpoint itself enforces, not only that it parses — see
 * `isWithinForecastWindow`. A start time left over from an earlier visit must
 * never come back as a request the endpoint is certain to answer with a
 * `400 invalid_request`.
 */

import { useCallback, useState } from "react";

const STORAGE_KEY = "domestique.start-time";

/**
 * The weather endpoint's own window, named here as the authority
 * (`weatherPastAllowance` and `weatherForecastHorizon` in
 * `internal/httpapi/routes_weather.go`). A start time outside this window is
 * one the endpoint refuses, so a control must be able to check it before
 * sending it, not only after the endpoint has said no.
 */
export const FORECAST_PAST_ALLOWANCE_MS = 24 * 60 * 60 * 1000;
export const FORECAST_HORIZON_MS = 16 * 24 * 60 * 60 * 1000;

/**
 * Whether `at` sits inside the window the weather endpoint will accept, as of
 * `now`. `now` is a parameter rather than always `new Date()` so a test can
 * hold time still instead of racing the clock the guard actually reads.
 */
export function isWithinForecastWindow(at: Date, now: Date = new Date()): boolean {
  const elapsedMs = now.getTime() - at.getTime();

  return elapsedMs <= FORECAST_PAST_ALLOWANCE_MS && elapsedMs >= -FORECAST_HORIZON_MS;
}

/**
 * The reader's chosen ride start time, remembered across visits.
 *
 * Guarded the same way `useThemeChoice` guards storage — a browser may refuse
 * it outright, in a private window or with third-party storage blocked — plus
 * the window check above. Either failure reads as `null`, which is the same
 * "nothing chosen" state offered before the reader ever picked a time.
 */
export function useStartTime(): [Date | null, (next: Date | null) => void] {
  const [startTime, setStartTimeState] = useState<Date | null>(readStartTime);

  const setStartTime = useCallback((next: Date | null) => {
    setStartTimeState(next);
    writeStartTime(next);
  }, []);

  return [startTime, setStartTime];
}

function readStartTime(): Date | null {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored === null) {
      return null;
    }
    const parsed = new Date(stored);

    return Number.isNaN(parsed.getTime()) || !isWithinForecastWindow(parsed) ? null : parsed;
  } catch {
    return null;
  }
}

function writeStartTime(next: Date | null): void {
  try {
    if (next === null) {
      window.localStorage.removeItem(STORAGE_KEY);

      return;
    }
    window.localStorage.setItem(STORAGE_KEY, next.toISOString());
  } catch {
    // Remembering is the whole of what is lost, and the pick still stands for
    // as long as the page is open.
  }
}
