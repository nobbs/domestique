/**
 * How many values fall in each of `count` equal bins across `[min, max]`.
 * Values past `max` land in the last bin, so a route beyond the slider's
 * domain still shows up rather than vanishing from the picture.
 */
export function histogram(values: number[], min: number, max: number, count: number): number[] {
  if (count <= 0) {
    return [];
  }
  const bins = new Array<number>(count).fill(0);
  const width = (max - min) / count;
  if (!(width > 0)) {
    return bins;
  }
  for (const value of values) {
    const index = Math.min(count - 1, Math.max(0, Math.floor((value - min) / width)));
    bins[index] = (bins[index] ?? 0) + 1;
  }

  return bins;
}
