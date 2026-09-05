/** Where a slider's track ends and how far a keypress moves it. */
export interface Domain {
  max: number;
  step: number;
}

/** At most this many steps across the track, so a keypress still moves it visibly. */
const MAX_STEPS = 60;

/**
 * The domain a library gives one measure: its largest value rounded up to a
 * whole step, with the step the coarsest of `steps` that still keeps the
 * track under `MAX_STEPS` stops. An empty library gets one step of track.
 */
export function domainOf(values: number[], steps: number[]): Domain {
  const largest = Math.max(0, ...values);
  const step = steps.find((candidate) => largest / candidate <= MAX_STEPS) ?? steps.at(-1) ?? 1;

  return { max: Math.max(step, Math.ceil(largest / step) * step), step };
}
