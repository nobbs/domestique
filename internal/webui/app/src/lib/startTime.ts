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

/**
 * Why a start time cannot be forecast for a ride of `rideSeconds`, or null
 * when it can.
 *
 * One classifier for the two places that ask. The control asks before it
 * accepts a keystroke, and the page asks again about a value it was handed
 * from storage — and both have to name the *same* trouble, because the two
 * refusals want opposite remedies: a start left over from last week needs a
 * later one, and a ride that outruns the forecast needs an earlier one.
 * Deciding that twice is how they come to disagree.
 *
 * `now` is a parameter so a caller can read the clock at the moment it
 * matters rather than when its component last rendered.
 */
export function startTimeRefusal(
  startAt: Date,
  rideSeconds: number | undefined,
  now: Date = new Date(),
): StartTimeRefusal | null {
  if (startAt.getTime() < now.getTime() - FORECAST_PAST_ALLOWANCE_MS) {
    return "past";
  }
  const arrival = new Date(startAt.getTime() + Math.max(rideSeconds ?? 0, 0) * 1000);
  if (!isWithinForecastWindow(startAt, now) || !isWithinForecastWindow(arrival, now)) {
    return "horizon";
  }

  return null;
}

/**
 * The two ways a start time can fall outside what the forecast can answer:
 * before the endpoint's past allowance, or with a finish beyond its horizon.
 * The horizon case covers a departure past the horizon too, since a ride that
 * starts after the forecast ends certainly finishes after it.
 */
export type StartTimeRefusal = "past" | "horizon";

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
