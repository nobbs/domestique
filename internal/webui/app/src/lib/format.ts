/**
 * Presentation helpers. The service stores, transmits and shows metric values
 * throughout, so nothing here converts — every formatter only chooses how much
 * of the figure it was handed is worth printing.
 */

import type { RouteValidation } from "../api/types";

export function formatDistance(metres: number): string {
  if (!Number.isFinite(metres) || metres <= 0) {
    return "—";
  }
  if (metres < 1000) {
    return `${Math.round(metres)} m`;
  }
  const kilometres = metres / 1000;
  return `${kilometres.toFixed(kilometres < 100 ? 1 : 0)} km`;
}

export function formatCount(value: number, singular: string, plural = `${singular}s`): string {
  return `${value.toLocaleString()} ${value === 1 ? singular : plural}`;
}

function formatElevationTotal(metres: number): string {
  if (!Number.isFinite(metres) || metres <= 0) {
    return "—";
  }

  return `${Math.round(metres).toLocaleString()} m`;
}

/**
 * Total ascent. Renders as a dash for zero or below — which reads as "no
 * usable elevation profile" for most routes, but is also the true value for
 * a route that only descends.
 */
export function formatAscent(metres: number): string {
  return formatElevationTotal(metres);
}

/** Total descent. Same dash convention as `formatAscent`, for the same reasons. */
export function formatDescent(metres: number): string {
  return formatElevationTotal(metres);
}

/**
 * The steepest sustained gradient. It is measured over a window rather than
 * between neighbouring points, so it reads as a climb rather than as satellite
 * noise; anything under a percent is not worth claiming as a gradient.
 */
export function formatGradient(percent: number): string {
  if (!Number.isFinite(percent) || percent < 1) {
    return "—";
  }

  return `${percent.toFixed(percent < 10 ? 1 : 0)}%`;
}

export function formatTimestamp(value: string | undefined): string {
  if (!value) {
    return "never";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return "unknown";
  }
  return parsed.toLocaleString(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

/**
 * When the library was last read, as a card says it.
 *
 * The clock alone for a read that happened today, which is nearly every read a
 * reader will ever see: the service reads hourly, and a full date beside it is
 * three quarters punctuation. Anything older keeps the date, because "19:38" on
 * a stale library is the one case where the short form would mislead — and the
 * page has no way to say "the sync has been down for two days" other than by
 * showing the day.
 */
export function formatReadTime(value: string | undefined, now = new Date()): string {
  if (!value) {
    return "never";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return "unknown";
  }
  if (parsed.toDateString() !== now.toDateString()) {
    return formatTimestamp(value);
  }

  return parsed.toLocaleTimeString(undefined, { timeStyle: "short" });
}

/**
 * A height above sea level.
 *
 * Not `formatAscent`: a climb of nought metres is a route with no usable
 * profile, but an altitude of nought metres is the coast, and a route that
 * drops below sea level is a real one.
 */
export function formatElevation(metres: number): string {
  if (!Number.isFinite(metres)) {
    return "—";
  }

  return `${Math.round(metres).toLocaleString()} m`;
}

/**
 * Air temperature at one forecast point.
 *
 * A decimal earns its place near freezing, where the digit after the point is
 * the difference between rain and ice; a reading already in double digits has
 * left that boundary far enough behind that the extra digit is only noise.
 */
export function formatTemperature(celsius: number): string {
  if (!Number.isFinite(celsius)) {
    return "—";
  }
  const decimals = Math.abs(celsius) < 10 ? 1 : 0;
  // A reading just below zero rounds to negative zero, and "-0°C" reads as a
  // fault rather than as a temperature.
  const rounded = Number(celsius.toFixed(decimals));
  const shown = Object.is(rounded, -0) ? 0 : rounded;

  return `${shown.toFixed(decimals)}°C`;
}

/**
 * Wind speed at one forecast point.
 *
 * The same reasoning as `formatTemperature`: a decimal separates a calm from
 * a light breeze, but is wasted once the reading is already a two-digit gale.
 */
export function formatWindSpeed(kmh: number): string {
  if (!Number.isFinite(kmh)) {
    return "—";
  }

  return `${kmh.toFixed(kmh < 10 ? 1 : 0)} km/h`;
}

/** Precipitation depth at one forecast point. */
export function formatPrecipitation(millimetres: number): string {
  if (!Number.isFinite(millimetres)) {
    return "—";
  }

  return `${millimetres.toFixed(1)} mm`;
}

/**
 * Derived from the server's own intervalSeconds, not a duplicated label.
 * Undefined means no fixed schedule.
 */
export function formatCadence(seconds: number | undefined): string {
  if (seconds === undefined || !Number.isFinite(seconds) || seconds <= 0) {
    return "No fixed schedule";
  }
  if (seconds === 3600) {
    return "Hourly";
  }
  if (seconds % 3600 === 0) {
    const hours = seconds / 3600;

    return `Every ${hours} hours`;
  }
  if (seconds < 60) {
    return seconds === 1 ? "Every second" : `Every ${seconds} seconds`;
  }
  const minutes = Math.round(seconds / 60);

  return minutes === 1 ? "Every minute" : `Every ${minutes} minutes`;
}

/**
 * Predicted moving time, rounded to the nearest five minutes — coarse enough
 * that it reads as an estimate rather than a promise. Absent, never zero,
 * when nothing has predicted this route's geometry.
 */
export function formatMovingTime(seconds: number | undefined): string {
  if (seconds === undefined || !Number.isFinite(seconds) || seconds <= 0) {
    return "—";
  }
  const roundedMinutes = Math.max(5, Math.round(seconds / 60 / 5) * 5);
  const hours = Math.floor(roundedMinutes / 60);
  const minutes = roundedMinutes % 60;
  if (hours === 0) {
    return `${minutes} min`;
  }

  return minutes === 0 ? `${hours} h` : `${hours} h ${minutes} min`;
}

/**
 * A coarse qualifier for a shown moving time, from the frozen profile's own
 * measured unseen-route error — mean absolute error, not the tighter bias,
 * since the reader wants "how far off does this usually run" rather than the
 * model's average direction of error. Undefined whenever the loaded profile
 * carries no measured benchmark result, which the caller reads as "show the
 * time unqualified" rather than as a missing value to report.
 */
export function formatMovingTimeUncertainty(
  validation: RouteValidation | undefined,
): string | undefined {
  if (!validation) {
    return undefined;
  }

  return `±${Math.round(validation.maePercent)}% typical`;
}
