/**
 * Distance as the elevation chart's axis reads it: kilometres off a metric
 * store, at a resolution its gridlines can tell apart.
 *
 * Everything the service stores and transmits is metric and everything shown
 * is metric, so nothing here converts — `format.ts` holds the formatters that
 * put a figure in words, and this is only the pair the chart needs to label an
 * axis rather than name a value.
 */

/** A distance in kilometres, as a bare number. */
export function distanceValue(metres: number): number {
  return metres / 1000;
}

/**
 * A distance in kilometres, with just enough decimals that a step this size
 * still tells neighbouring labels apart. Whole kilometres are right for a
 * whole route and useless for a four-hundred-metre window, where every label
 * would read the same number.
 */
export function distanceLabel(metres: number, stepKilometres: number): string {
  const decimals = Math.min(Math.max(Math.ceil(-Math.log10(stepKilometres)), 0), 3);

  return distanceValue(metres).toFixed(decimals);
}
