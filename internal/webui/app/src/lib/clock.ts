/**
 * A render forced once the wall clock crosses into the next hour, for a
 * caller that reads `Date.now()` at render time with nothing else that would
 * ever re-render it on its own — a label or a query key left open across an
 * hour boundary would otherwise keep reading the hour that was current the
 * last time something else caused a render.
 */

import { useEffect, useReducer } from "react";

export function useHourTick(on: boolean): void {
  const [, tick] = useReducer((count: number) => count + 1, 0);

  useEffect(() => {
    if (!on) {
      return;
    }
    let timer: ReturnType<typeof setTimeout>;
    const scheduleNextHour = () => {
      // A second past the boundary, not right on it: `Date.now()` and the
      // timer's own firing both carry a little slack, and the hour only
      // needs to have moved on by the time this runs, never exactly at it.
      const msUntilNextHour = 3_600_000 - (Date.now() % 3_600_000) + 1_000;
      timer = setTimeout(() => {
        tick();
        scheduleNextHour();
      }, msUntilNextHour);
    };
    scheduleNextHour();

    return () => clearTimeout(timer);
  }, [on]);
}
